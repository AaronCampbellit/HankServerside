package cloud

import (
	"context"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/storageops"
)

func (s *Server) forwardStorageEvents(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	seeded := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.storage == nil {
				continue
			}
			events, err := s.storage.Events(storageops.EventFilter{Limit: 50})
			if err != nil {
				continue
			}
			for _, event := range events {
				if !s.markStorageEventSeen(event.ID) {
					continue
				}
				if seeded {
					s.publishStorageEvent(ctx, event)
				}
			}
			seeded = true
		}
	}
}

func (s *Server) markStorageEventSeen(eventID string) bool {
	if strings.TrimSpace(eventID) == "" {
		return false
	}
	s.storageEventsMu.Lock()
	defer s.storageEventsMu.Unlock()
	if _, ok := s.storageEvents[eventID]; ok {
		return false
	}
	s.storageEvents[eventID] = struct{}{}
	return true
}

func (s *Server) publishStorageEvent(ctx context.Context, event storageops.Event) {
	_ = s.markStorageEventSeen(event.ID)
	s.emitStorageEvent(ctx, storageRealtimeEventName(event), storageRealtimePayload(event))
	s.notifyStorageEvent(ctx, event)
}

func storageRealtimePayload(event storageops.Event) map[string]any {
	return map[string]any{
		"event_id":     event.ID,
		"operation":    event.Operation,
		"status":       event.Status,
		"severity":     event.Severity,
		"message":      storageops.RedactSensitive(event.Message),
		"backup_label": event.BackupLabel,
	}
}

func storageRealtimeEventName(event storageops.Event) string {
	switch event.Operation {
	case storageops.EventOperationBackup:
		if storageops.IsFailureEvent(event) {
			return "storage.backup.failed"
		}
	case storageops.EventOperationChecksum, storageops.EventOperationAMCheck:
		if event.Severity == storageops.EventSeverityCritical || boolFromEventDetails(event, "corruption_detected") {
			return "storage.checksum.corruption"
		}
	case storageops.EventOperationRestoreTest, storageops.EventOperationPrimaryRestore:
		switch event.Status {
		case storageops.EventStatusStarted, storageops.EventStatusPending:
			return "storage.restore.started"
		case storageops.EventStatusSuccess:
			return "storage.restore.completed"
		case storageops.EventStatusFailed:
			return "storage.restore.failed"
		}
	}
	return "storage.health.changed"
}

func boolFromEventDetails(event storageops.Event, key string) bool {
	if event.Details == nil {
		return false
	}
	value, ok := event.Details[key]
	if !ok {
		return false
	}
	boolValue, _ := value.(bool)
	return boolValue
}
