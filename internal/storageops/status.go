package storageops

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StatusSnapshot struct {
	Config   Config         `json:"config"`
	Checksum ChecksumStatus `json:"checksum"`
	Backup   BackupStatus   `json:"backup"`
	Restore  RestoreStatus  `json:"restore"`
	Events   []Event        `json:"events"`
	Failures []Event        `json:"failures"`
}

type ChecksumStatus struct {
	Enabled            bool       `json:"enabled"`
	LastCheckAt        *time.Time `json:"last_check_at,omitempty"`
	LastAMCheckAt      *time.Time `json:"last_amcheck_at,omitempty"`
	CorruptionDetected bool       `json:"corruption_detected"`
	FailureCount       int        `json:"failure_count"`
	LastError          string     `json:"last_error,omitempty"`
}

type BackupStatus struct {
	Target           BackupTarget `json:"target"`
	LastSuccessfulAt *time.Time   `json:"last_successful_at,omitempty"`
	LastFailedAt     *time.Time   `json:"last_failed_at,omitempty"`
	LastBackupLabel  string       `json:"last_backup_label,omitempty"`
	FailureCount     int          `json:"failure_count"`
	Backups          []BackupSet  `json:"backups"`
}

type BackupSet struct {
	Label     string     `json:"label"`
	Type      string     `json:"type"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
	Status    string     `json:"status,omitempty"`
	SizeBytes int64      `json:"size_bytes,omitempty"`
}

type RestoreStatus struct {
	LastTestAt           *time.Time `json:"last_test_at,omitempty"`
	LastPrimaryRestoreAt *time.Time `json:"last_primary_restore_at,omitempty"`
	LastFailedAt         *time.Time `json:"last_failed_at,omitempty"`
	PendingIntents       []Intent   `json:"pending_intents,omitempty"`
}

func LoadStatus(stateDir string, logDir string, secret string) (StatusSnapshot, error) {
	cfg, err := LoadConfig(stateDir)
	if err != nil {
		cfg = DefaultConfig()
	}
	events, eventErr := ListEvents(logDir, EventFilter{Limit: 100})
	if eventErr != nil {
		return StatusSnapshot{}, eventErr
	}
	failures, failureErr := ListEvents(logDir, EventFilter{Limit: 50, FailuresOnly: true})
	if failureErr != nil {
		return StatusSnapshot{}, failureErr
	}
	backups, _ := LoadBackupSets(stateDir)
	intents, _ := ListIntents(stateDir, secret)
	for index := range intents {
		intents[index].Confirmation = ""
	}

	status := StatusSnapshot{
		Config: cfg,
		Backup: BackupStatus{
			Target:  cfg.Target,
			Backups: backups,
		},
		Events:   events,
		Failures: failures,
		Restore:  RestoreStatus{PendingIntents: intents},
	}
	for index := len(events) - 1; index >= 0; index-- {
		applyEventToStatus(&status, events[index])
	}
	return status, nil
}

func applyEventToStatus(status *StatusSnapshot, event Event) {
	occurredAt := event.Time
	switch event.Operation {
	case EventOperationChecksum:
		if event.Status == EventStatusSuccess {
			status.Checksum.LastCheckAt = &occurredAt
			status.Checksum.Enabled = boolFromDetails(event.Details, "enabled")
		}
		if IsFailureEvent(event) {
			status.Checksum.FailureCount++
			status.Checksum.LastError = event.Message
			if boolFromDetails(event.Details, "corruption_detected") || strings.Contains(strings.ToLower(event.Message), "corrupt") {
				status.Checksum.CorruptionDetected = true
			}
		}
	case EventOperationAMCheck:
		if event.Status == EventStatusSuccess {
			status.Checksum.LastAMCheckAt = &occurredAt
		}
		if IsFailureEvent(event) {
			status.Checksum.FailureCount++
			status.Checksum.LastError = event.Message
			status.Checksum.CorruptionDetected = true
		}
	case EventOperationBackup:
		if event.Status == EventStatusSuccess {
			status.Backup.LastSuccessfulAt = &occurredAt
			status.Backup.LastBackupLabel = event.BackupLabel
		}
		if IsFailureEvent(event) {
			status.Backup.LastFailedAt = &occurredAt
			status.Backup.FailureCount++
		}
	case EventOperationRestoreTest:
		if event.Status == EventStatusSuccess {
			status.Restore.LastTestAt = &occurredAt
		}
		if IsFailureEvent(event) {
			status.Restore.LastFailedAt = &occurredAt
		}
	case EventOperationPrimaryRestore:
		if event.Status == EventStatusSuccess {
			status.Restore.LastPrimaryRestoreAt = &occurredAt
		}
		if IsFailureEvent(event) {
			status.Restore.LastFailedAt = &occurredAt
		}
	}
}

func BackupInfoPath(stateDir string) string {
	return filepath.Join(dirOrDefault(stateDir, DefaultStateDir), "backups.json")
}

func SaveBackupSets(stateDir string, backups []BackupSet) error {
	if err := os.MkdirAll(dirOrDefault(stateDir, DefaultStateDir), 0o777); err != nil {
		return err
	}
	data, err := json.MarshalIndent(backups, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(BackupInfoPath(stateDir), data, 0o666)
}

func LoadBackupSets(stateDir string) ([]BackupSet, error) {
	data, err := os.ReadFile(BackupInfoPath(stateDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var backups []BackupSet
	if err := json.Unmarshal(data, &backups); err != nil {
		return nil, err
	}
	return backups, nil
}

func boolFromDetails(details map[string]any, key string) bool {
	if details == nil {
		return false
	}
	value, ok := details[key]
	if !ok {
		return false
	}
	boolValue, _ := value.(bool)
	return boolValue
}
