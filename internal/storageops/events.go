package storageops

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	EventSeverityInfo     = "info"
	EventSeverityWarning  = "warning"
	EventSeverityError    = "error"
	EventSeverityCritical = "critical"

	EventStatusStarted = "started"
	EventStatusSuccess = "success"
	EventStatusFailed  = "failed"
	EventStatusPending = "pending"

	EventOperationChecksum       = "checksum"
	EventOperationAMCheck        = "amcheck"
	EventOperationBackup         = "backup"
	EventOperationRestoreTest    = "restore_test"
	EventOperationPrimaryRestore = "primary_restore"
	EventOperationConfig         = "config"
)

type Event struct {
	ID          string         `json:"id"`
	Time        time.Time      `json:"time"`
	Severity    string         `json:"severity"`
	Operation   string         `json:"operation"`
	Status      string         `json:"status"`
	Message     string         `json:"message"`
	BackupLabel string         `json:"backup_label,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
}

type EventFilter struct {
	Limit        int
	Severity     string
	Operation    string
	FailuresOnly bool
}

func EventLogPath(logDir string) string {
	return filepath.Join(dirOrDefault(logDir, DefaultLogDir), "storage-events.jsonl")
}

func NewEvent(operation string, status string, severity string, message string) Event {
	return Event{
		ID:        newEventID(),
		Time:      time.Now().UTC(),
		Severity:  normalizeSeverity(severity),
		Operation: strings.TrimSpace(operation),
		Status:    strings.TrimSpace(status),
		Message:   strings.TrimSpace(message),
	}
}

func AppendEvent(logDir string, event Event) (Event, error) {
	if event.ID == "" {
		event.ID = newEventID()
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	event.Severity = normalizeSeverity(event.Severity)
	event = RedactEvent(event)
	if err := os.MkdirAll(dirOrDefault(logDir, DefaultLogDir), 0o777); err != nil {
		return Event{}, err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return Event{}, err
	}
	file, err := os.OpenFile(EventLogPath(logDir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return Event{}, err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return Event{}, err
	}
	return event, nil
}

func ListEvents(logDir string, filter EventFilter) ([]Event, error) {
	path := EventLogPath(logDir)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var events []Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			events = append(events, NewEvent("event_log", EventStatusFailed, EventSeverityWarning, fmt.Sprintf("Skipped unreadable storage log line: %v", err)))
			continue
		}
		event = RedactEvent(event)
		if !eventMatchesFilter(event, filter) {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Time.After(events[j].Time)
	})
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if len(events) > filter.Limit {
		events = events[:filter.Limit]
	}
	return events, nil
}

func eventMatchesFilter(event Event, filter EventFilter) bool {
	if filter.Severity != "" && normalizeSeverity(event.Severity) != normalizeSeverity(filter.Severity) {
		return false
	}
	if filter.Operation != "" && event.Operation != filter.Operation {
		return false
	}
	if filter.FailuresOnly && event.Status != EventStatusFailed && event.Severity != EventSeverityError && event.Severity != EventSeverityCritical {
		return false
	}
	return true
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case EventSeverityCritical:
		return EventSeverityCritical
	case EventSeverityError:
		return EventSeverityError
	case EventSeverityWarning, "warn":
		return EventSeverityWarning
	default:
		return EventSeverityInfo
	}
}

func IsFailureEvent(event Event) bool {
	return event.Status == EventStatusFailed || event.Severity == EventSeverityError || event.Severity == EventSeverityCritical
}

func newEventID() string {
	return fmt.Sprintf("sto_%d", time.Now().UTC().UnixNano())
}
