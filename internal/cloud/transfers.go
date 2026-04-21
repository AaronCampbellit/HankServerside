package cloud

import (
	"errors"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
)

var (
	ErrTransferNotFound      = errors.New("transfer not found")
	ErrTransferBusy          = errors.New("transfer is already active")
	ErrTransferOffsetInvalid = errors.New("transfer offset is invalid")
)

type transferRegistry struct {
	mu        sync.Mutex
	transfers map[string]*transferSession
	attempts  map[string]*transferAttempt
}

type transferSession struct {
	ID        string
	HomeID    string
	AgentID   string
	Operation string
	Path      string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time

	mu          sync.Mutex
	size        int64
	nextOffset  int64
	lastError   *protocol.ErrorPayload
	active      bool
	attemptID   string
	resumeCount int
	completedAt *time.Time
}

type transferAttempt struct {
	Session    *transferSession
	ID         string
	Offset     int64
	ReadyCh    chan transferReadyResult
	DataCh     chan transferDataFrame
	CompleteCh chan transferCompleteResult
}

type transferReadyResult struct {
	Ready protocol.FileTransferReady
	Error *protocol.ErrorPayload
}

type transferDataFrame struct {
	Offset int64
	Data   []byte
	Error  *protocol.ErrorPayload
}

type transferCompleteResult struct {
	Complete protocol.FileTransferComplete
	Error    *protocol.ErrorPayload
}

func newTransferRegistry() *transferRegistry {
	return &transferRegistry{
		transfers: make(map[string]*transferSession),
		attempts:  make(map[string]*transferAttempt),
	}
}

func (r *transferRegistry) Create(homeID string, agentID string, operation string, path string, ttl time.Duration) (*transferSession, string) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	id := newID("xfer")
	rawToken := newToken()
	session := &transferSession{
		ID:        id,
		HomeID:    homeID,
		AgentID:   agentID,
		Operation: operation,
		Path:      path,
		TokenHash: hashToken(rawToken),
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}

	r.mu.Lock()
	r.transfers[id] = session
	r.mu.Unlock()

	return session, rawToken
}

func (r *transferRegistry) Get(id string) (*transferSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.transfers[id]
	if !ok {
		return nil, false
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		delete(r.transfers, id)
		return nil, false
	}
	return session, true
}

func (r *transferRegistry) Delete(id string) {
	r.mu.Lock()
	delete(r.transfers, id)
	r.mu.Unlock()
}

func (r *transferRegistry) Authorize(id string, rawToken string, operation string) (*transferSession, error) {
	session, ok := r.Get(id)
	if !ok {
		return nil, ErrTransferNotFound
	}
	if session.Operation != operation || session.TokenHash != hashToken(rawToken) {
		return nil, ErrTransferNotFound
	}
	return session, nil
}

func (r *transferRegistry) BeginAttempt(session *transferSession, offset int64) (*transferAttempt, error) {
	attempt, err := session.BeginAttempt(offset)
	if err != nil {
		return nil, err
	}
	attempt.Session = session

	r.mu.Lock()
	r.attempts[attempt.ID] = attempt
	r.mu.Unlock()

	return attempt, nil
}

func (r *transferRegistry) GetAttempt(id string) (*transferAttempt, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt, ok := r.attempts[id]
	return attempt, ok
}

func (r *transferRegistry) EndAttempt(id string) {
	r.mu.Lock()
	attempt, ok := r.attempts[id]
	if ok {
		delete(r.attempts, id)
	}
	r.mu.Unlock()
	if ok {
		attempt.Session.EndAttempt()
	}
}

func (s *transferSession) Snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload := map[string]any{
		"transfer_id":  s.ID,
		"path":         s.Path,
		"operation":    s.Operation,
		"size":         s.size,
		"next_offset":  s.nextOffset,
		"resume_count": s.resumeCount,
		"created_at":   s.CreatedAt,
		"expires_at":   s.ExpiresAt,
		"active":       s.active,
		"completed_at": s.completedAt,
		"resumable":    true,
	}
	if s.lastError != nil {
		payload["last_error"] = s.lastError
	}
	return payload
}

func (s *transferSession) BeginAttempt(offset int64) (*transferAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Now().UTC().After(s.ExpiresAt) {
		return nil, ErrTransferNotFound
	}
	if s.active {
		return nil, ErrTransferBusy
	}
	if offset < 0 {
		return nil, ErrTransferOffsetInvalid
	}

	if s.Operation == protocol.FileTransferOperationUpload {
		if offset > s.nextOffset {
			return nil, ErrTransferOffsetInvalid
		}
	}
	if s.Operation == protocol.FileTransferOperationDownload && s.size > 0 && offset > s.size {
		return nil, ErrTransferOffsetInvalid
	}

	s.active = true
	s.attemptID = newID("xfertry")
	s.resumeCount++
	s.lastError = nil

	return &transferAttempt{
		ID:         s.attemptID,
		Offset:     offset,
		ReadyCh:    make(chan transferReadyResult, 1),
		DataCh:     make(chan transferDataFrame, 16),
		CompleteCh: make(chan transferCompleteResult, 1),
	}, nil
}

func (s *transferSession) EndAttempt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	s.attemptID = ""
}

func (s *transferSession) IsAttempt(attemptID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active && s.attemptID == attemptID
}

func (s *transferSession) MarkReady(ready protocol.FileTransferReady) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = ready.Size
	if s.Operation == protocol.FileTransferOperationUpload && ready.Offset > s.nextOffset {
		s.nextOffset = ready.Offset
	}
}

func (s *transferSession) Advance(nextOffset int64, totalSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if nextOffset > s.nextOffset {
		s.nextOffset = nextOffset
	}
	if totalSize > s.size {
		s.size = totalSize
	}
}

func (s *transferSession) Fail(err *protocol.ErrorPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err
}

func (s *transferSession) Complete(done protocol.FileTransferComplete) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = done.Size
	if done.Offset > s.nextOffset {
		s.nextOffset = done.Offset
	}
	now := time.Now().UTC()
	s.completedAt = &now
	s.lastError = nil
}

func (s *transferSession) NextOffset() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextOffset
}
