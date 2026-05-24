package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

const (
	defaultAssistantTraceLimit = 250
	maxAssistantTraceLimit     = 1000
)

type assistantTraceContextKey struct{}

type assistantTraceContext struct {
	HomeID    string
	UserID    string
	SessionID string
	RunID     string
	MessageID string
	RequestID string
}

type assistantTraceEvent struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"created_at"`
	Level     string            `json:"level"`
	Scope     string            `json:"scope"`
	Event     string            `json:"event"`
	Summary   string            `json:"summary"`
	HomeID    string            `json:"home_id,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	RunID     string            `json:"run_id,omitempty"`
	MessageID string            `json:"message_id,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

type assistantTraceLog struct {
	mu     sync.Mutex
	events []assistantTraceEvent
	limit  int
}

type assistantTraceResponse struct {
	Events []assistantTraceEvent `json:"events"`
	Total  int                   `json:"total"`
}

func newAssistantTraceLog(limit int) *assistantTraceLog {
	if limit <= 0 {
		limit = maxAssistantTraceLimit
	}
	return &assistantTraceLog{limit: limit}
}

func withAssistantTraceContext(ctx context.Context, trace assistantTraceContext) context.Context {
	return context.WithValue(ctx, assistantTraceContextKey{}, trace)
}

func assistantTraceContextFrom(ctx context.Context) assistantTraceContext {
	if ctx == nil {
		return assistantTraceContext{}
	}
	trace, _ := ctx.Value(assistantTraceContextKey{}).(assistantTraceContext)
	return trace
}

func (s *Server) handleAssistantLogs(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership) {
	if membership.Role != domain.HomeRoleAdmin {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit := defaultAssistantTraceLimit
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = min(parsed, maxAssistantTraceLimit)
			}
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
		events, total := s.assistantTraceSnapshot(home.ID, sessionID, runID, limit)
		writeJSON(w, http.StatusOK, assistantTraceResponse{Events: events, Total: total})
	case http.MethodDelete:
		cleared := s.clearAssistantTrace(home.ID)
		s.recordAssistantTrace(r.Context(), assistantTraceEvent{
			Level:   "info",
			Scope:   "assistant",
			Event:   "assistant.trace.cleared",
			Summary: "Cleared HankAI workflow trace entries.",
			HomeID:  home.ID,
			Details: map[string]string{
				"cleared": strconv.Itoa(cleared),
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{"cleared": cleared})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) recordAssistantTrace(ctx context.Context, event assistantTraceEvent) {
	if s == nil || s.assistantTrace == nil {
		return
	}
	trace := assistantTraceContextFrom(ctx)
	if event.HomeID == "" {
		event.HomeID = trace.HomeID
	}
	if event.UserID == "" {
		event.UserID = trace.UserID
	}
	if event.SessionID == "" {
		event.SessionID = trace.SessionID
	}
	if event.RunID == "" {
		event.RunID = trace.RunID
	}
	if event.MessageID == "" {
		event.MessageID = trace.MessageID
	}
	if event.RequestID == "" {
		event.RequestID = trace.RequestID
	}
	event.ID = newID("atrace")
	event.CreatedAt = time.Now().UTC()
	event.Level = firstNonBlank(event.Level, "info")
	event.Scope = firstNonBlank(event.Scope, "assistant")
	event.Summary = truncateTraceValue(event.Summary)
	event.Details = sanitizeTraceDetails(event.Details)
	s.assistantTrace.append(event)
}

func (s *Server) assistantTraceSnapshot(homeID string, sessionID string, runID string, limit int) ([]assistantTraceEvent, int) {
	if s == nil || s.assistantTrace == nil {
		return nil, 0
	}
	return s.assistantTrace.snapshot(homeID, sessionID, runID, limit)
}

func (s *Server) clearAssistantTrace(homeID string) int {
	if s == nil || s.assistantTrace == nil {
		return 0
	}
	return s.assistantTrace.clear(homeID)
}

func (l *assistantTraceLog) append(event assistantTraceEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
	if l.limit > 0 && len(l.events) > l.limit {
		l.events = append([]assistantTraceEvent(nil), l.events[len(l.events)-l.limit:]...)
	}
}

func (l *assistantTraceLog) snapshot(homeID string, sessionID string, runID string, limit int) ([]assistantTraceEvent, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit <= 0 {
		limit = defaultAssistantTraceLimit
	}
	limit = min(limit, maxAssistantTraceLimit)
	filtered := make([]assistantTraceEvent, 0, len(l.events))
	for _, event := range l.events {
		if homeID != "" && event.HomeID != "" && event.HomeID != homeID {
			continue
		}
		if sessionID != "" && event.SessionID != sessionID {
			continue
		}
		if runID != "" && event.RunID != runID {
			continue
		}
		filtered = append(filtered, event)
	}
	total := len(filtered)
	if total > limit {
		filtered = filtered[total-limit:]
	}
	return append([]assistantTraceEvent(nil), filtered...), total
}

func (l *assistantTraceLog) clear(homeID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if homeID == "" {
		cleared := len(l.events)
		l.events = nil
		return cleared
	}
	kept := l.events[:0]
	cleared := 0
	for _, event := range l.events {
		if event.HomeID == "" || event.HomeID == homeID {
			cleared++
			continue
		}
		kept = append(kept, event)
	}
	l.events = kept
	return cleared
}

func sanitizeTraceDetails(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(details))
	for key, value := range details {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if traceKeyLooksSensitive(key) {
			sanitized[key] = "[redacted]"
			continue
		}
		sanitized[key] = truncateTraceValue(value)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func traceKeyLooksSensitive(key string) bool {
	lowered := strings.ToLower(key)
	for _, marker := range []string{"password", "token", "secret", "credential", "cookie", "authorization", "api_key", "apikey"} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func truncateTraceValue(value string) string {
	value = strings.TrimSpace(value)
	const max = 600
	if len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}

func traceDetails(values map[string]any) map[string]string {
	if len(values) == 0 {
		return nil
	}
	details := make(map[string]string, len(values))
	for key, value := range values {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			details[key] = typed
		case int:
			details[key] = strconv.Itoa(typed)
		case int64:
			details[key] = strconv.FormatInt(typed, 10)
		case bool:
			details[key] = strconv.FormatBool(typed)
		case time.Duration:
			details[key] = typed.String()
		case time.Time:
			details[key] = typed.Format(time.RFC3339)
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				details[key] = "unprintable"
			} else {
				details[key] = string(encoded)
			}
		}
	}
	return details
}

func traceEventDetailsFromJSON(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return map[string]string{"payload_error": err.Error()}
	}
	keys := []string{"job_id", "title", "status", "completed_count", "total_count", "failed_count", "skipped_count", "current_file", "error_message"}
	details := make(map[string]string)
	for _, key := range keys {
		value, ok := body[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			details[key] = typed
		case float64:
			details[key] = strconv.Itoa(int(typed))
		case bool:
			details[key] = strconv.FormatBool(typed)
		default:
			encoded, _ := json.Marshal(typed)
			details[key] = string(encoded)
		}
	}
	return details
}
