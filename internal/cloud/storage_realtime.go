package cloud

import (
	"context"
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
				if _, ok := s.storageEvents[event.ID]; ok {
					continue
				}
				s.storageEvents[event.ID] = struct{}{}
				if seeded {
					s.emitStorageEvent(ctx, storageRealtimeEventName(event), storageRealtimePayload(event))
				}
			}
			seeded = true
		}
	}
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
