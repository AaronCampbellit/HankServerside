package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

const (
	auditSeverityInfo     = "info"
	auditSeverityWarning  = "warning"
	auditSeverityCritical = "critical"
)

func (s *Server) audit(ctx context.Context, eventType string, severity string, actorUserID string, actorAgentID string, homeID string, requestID string, targetType string, targetID string, metadata map[string]any) {
	if strings.TrimSpace(severity) == "" {
		severity = auditSeverityInfo
	}
	data := "{}"
	if metadata != nil {
		if encoded, err := json.Marshal(metadata); err == nil {
			data = string(encoded)
		}
	}
	event := store.AuditEvent{
		ID:           newID("audit"),
		OccurredAt:   time.Now().UTC(),
		ActorUserID:  nullableString(actorUserID),
		ActorAgentID: nullableString(actorAgentID),
		HomeID:       nullableString(homeID),
		EventType:    eventType,
		Severity:     severity,
		RequestID:    requestID,
		TargetType:   targetType,
		TargetID:     targetID,
		MetadataJSON: data,
	}
	if err := s.store.CreateAuditEvent(ctx, event); err != nil {
		s.logger.Warn("failed to record audit event", "event_type", eventType, "error", err)
	}
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stableAuditTarget(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(value))))
	return hex.EncodeToString(sum[:12])
}

func (s *Server) handleHomeAuditEvents(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership, parts []string) bool {
	if len(parts) != 1 || parts[0] != "audit-events" {
		return false
	}
	if membership.Role != domain.HomeRoleAdmin {
		http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
		return true
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.store.ListAuditEvents(
		r.Context(),
		home.ID,
		strings.TrimSpace(r.URL.Query().Get("event_type")),
		strings.TrimSpace(r.URL.Query().Get("severity")),
		strings.TrimSpace(r.URL.Query().Get("target_type")),
		limit,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	_ = auth
	writeJSON(w, http.StatusOK, map[string]any{"events": auditEventSnapshots(events)})
	return true
}

func auditEventSnapshots(events []store.AuditEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, map[string]any{
			"id":             event.ID,
			"occurred_at":    event.OccurredAt,
			"actor_user_id":  event.ActorUserID,
			"actor_agent_id": event.ActorAgentID,
			"home_id":        event.HomeID,
			"event_type":     event.EventType,
			"severity":       event.Severity,
			"request_id":     event.RequestID,
			"ip_hash":        event.IPHash,
			"target_type":    event.TargetType,
			"target_id":      event.TargetID,
			"metadata":       redactAuditMetadata(event.MetadataJSON),
		})
	}
	return out
}

func redactAuditMetadata(raw string) map[string]any {
	var metadata map[string]any
	if json.Unmarshal([]byte(raw), &metadata) != nil {
		return map[string]any{}
	}
	for key, value := range metadata {
		lowered := strings.ToLower(key)
		if strings.Contains(lowered, "token") || strings.Contains(lowered, "secret") || strings.Contains(lowered, "password") {
			metadata[key] = "[redacted]"
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			encoded, _ := json.Marshal(nested)
			metadata[key] = redactAuditMetadata(string(encoded))
		}
	}
	return metadata
}
