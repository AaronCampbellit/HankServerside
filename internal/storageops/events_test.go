package storageops

import (
	"strings"
	"testing"
)

func TestEventLogParsingAndStatus(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()

	_, err := AppendEvent(logDir, Event{
		Operation: EventOperationChecksum,
		Status:    EventStatusSuccess,
		Severity:  EventSeverityInfo,
		Message:   "checksums on",
		Details:   map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = AppendEvent(logDir, Event{
		Operation: EventOperationAMCheck,
		Status:    EventStatusFailed,
		Severity:  EventSeverityCritical,
		Message:   "corruption found",
		Details:   map[string]any{"corruption_detected": true},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := ListEvents(logDir, EventFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events length = %d, want 2", len(events))
	}

	status, err := LoadStatus(stateDir, logDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Checksum.Enabled {
		t.Fatal("expected checksum status to show enabled")
	}
	if !status.Checksum.CorruptionDetected {
		t.Fatal("expected corruption to be highlighted")
	}
	if status.Checksum.FailureCount != 1 {
		t.Fatalf("checksum failure count = %d, want 1", status.Checksum.FailureCount)
	}
}

func TestEventRedaction(t *testing.T) {
	logDir := t.TempDir()
	_, err := AppendEvent(logDir, Event{
		Operation: EventOperationBackup,
		Status:    EventStatusFailed,
		Severity:  EventSeverityError,
		Message:   "backup failed for postgres://hank:db-secret@postgres/hankremote with repo1-cipher-pass=cipher-secret",
		Details: map[string]any{
			"output": "Authorization: Bearer session-token password=db-password token=raw-token",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := ListEvents(logDir, EventFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	encoded := events[0].Message + " " + events[0].Details["output"].(string)
	for _, leaked := range []string{"db-secret", "cipher-secret", "session-token", "db-password", "raw-token"} {
		if contains := strings.Contains(encoded, leaked); contains {
			t.Fatalf("event leaked %q: %+v", leaked, events[0])
		}
	}
}
