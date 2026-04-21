package cloud

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
)

type agentRequestRegistry struct {
	mu      sync.Mutex
	pending map[string]chan protocol.Envelope
}

func newAgentRequestRegistry() *agentRequestRegistry {
	return &agentRequestRegistry{pending: make(map[string]chan protocol.Envelope)}
}

func (r *agentRequestRegistry) Register(requestID string) (<-chan protocol.Envelope, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[requestID]; ok {
		return nil, errors.New("duplicate agent request")
	}
	ch := make(chan protocol.Envelope, 1)
	r.pending[requestID] = ch
	return ch, nil
}

func (r *agentRequestRegistry) Resolve(envelope protocol.Envelope) bool {
	r.mu.Lock()
	ch, ok := r.pending[envelope.RequestID]
	if ok {
		delete(r.pending, envelope.RequestID)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	ch <- envelope
	return true
}

func (r *agentRequestRegistry) Cancel(requestID string) {
	r.mu.Lock()
	delete(r.pending, requestID)
	r.mu.Unlock()
}

func (s *Server) sendAgentCommand(ctx context.Context, homeID string, command string, body any) (protocol.Envelope, error) {
	agentConn, ok := s.router.GetAgent(homeID)
	if !ok {
		return protocol.Envelope{}, errors.New("agent offline")
	}

	requestID := newID("sync")
	responseCh, err := s.agentRequests.Register(requestID)
	if err != nil {
		return protocol.Envelope{}, err
	}
	defer s.agentRequests.Cancel(requestID)

	commandBody, err := protocol.EncodeBody(body)
	if err != nil {
		return protocol.Envelope{}, err
	}
	envelope, err := protocol.NewEnvelope(protocol.TypeCloudCommand, requestID, agentConn.agent.ID, homeID, protocol.RoutedCommand{
		Command: command,
		Body:    commandBody,
	})
	if err != nil {
		return protocol.Envelope{}, err
	}
	if err := agentConn.peer.Write(ctx, envelope); err != nil {
		return protocol.Envelope{}, err
	}

	timeout := s.requestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-waitCtx.Done():
		return protocol.Envelope{}, waitCtx.Err()
	case response := <-responseCh:
		return response, nil
	}
}
