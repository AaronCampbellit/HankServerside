package storageops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type ExecRunner struct {
	Env []string
	Dir string
}

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.Dir != "" {
		cmd.Dir = r.Dir
	}
	cmd.Env = append(os.Environ(), r.Env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

type WorkerOptions struct {
	StateDir           string
	LogDir             string
	IntentSecret       string
	RepoCipherPass     string
	DatabaseURL        string
	Stanza             string
	PGDataPath         string
	RestoreDataPath    string
	RestoreDatabaseURL string
	ComposeFile        string
	Runner             Runner
}

type Worker struct {
	service            *Service
	runner             Runner
	repoCipherPass     string
	databaseURL        string
	stanza             string
	pgDataPath         string
	restoreDataPath    string
	restoreDatabaseURL string
	composeFile        string
	lastCronRuns       map[string]string
}

func NewWorker(options WorkerOptions) *Worker {
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	stanza := strings.TrimSpace(options.Stanza)
	if stanza == "" {
		stanza = "hank"
	}
	pgDataPath := strings.TrimSpace(options.PGDataPath)
	if pgDataPath == "" {
		pgDataPath = "/var/lib/postgresql/data"
	}
	restoreDataPath := strings.TrimSpace(options.RestoreDataPath)
	if restoreDataPath == "" {
		restoreDataPath = "/var/lib/postgresql/restore"
	}
	restoreDatabaseURL := strings.TrimSpace(options.RestoreDatabaseURL)
	if restoreDatabaseURL == "" {
		restoreDatabaseURL = "postgres://hankremote:hankremote@postgres-restore:5432/hankremote?sslmode=disable"
	}
	composeFile := strings.TrimSpace(options.ComposeFile)
	if composeFile == "" {
		composeFile = "/workspace/docker-compose.yml"
	}
	return &Worker{
		service:            NewService(options.StateDir, options.LogDir, options.IntentSecret),
		runner:             runner,
		repoCipherPass:     strings.TrimSpace(options.RepoCipherPass),
		databaseURL:        strings.TrimSpace(options.DatabaseURL),
		stanza:             stanza,
		pgDataPath:         pgDataPath,
		restoreDataPath:    restoreDataPath,
		restoreDatabaseURL: restoreDatabaseURL,
		composeFile:        composeFile,
		lastCronRuns:       make(map[string]string),
	}
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if _, err := w.service.Config(); err != nil {
		if _, saveErr := w.service.SaveConfig(DefaultConfig()); saveErr != nil {
			return saveErr
		}
	}
	if err := w.processIntents(ctx); err != nil {
		return err
	}
	return nil
}

func (w *Worker) Run(ctx context.Context) error {
	if _, err := w.service.Config(); err != nil {
		if _, saveErr := w.service.SaveConfig(DefaultConfig()); saveErr != nil {
			return saveErr
		}
	}
	_, _ = AppendEvent(w.service.LogDir, NewEvent("worker", EventStatusStarted, EventSeverityInfo, "Database operations worker started."))
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	checksumTimer := time.NewTimer(0)
	defer checksumTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-checksumTimer.C:
			cfg, _ := w.service.Config()
			w.RunChecksumStatus(ctx)
			checksumTimer.Reset(time.Duration(cfg.Schedule.ChecksumIntervalSeconds) * time.Second)
		case now := <-ticker.C:
			w.runDueCron(ctx, now.UTC())
			_ = w.processIntents(ctx)
		}
	}
}

func (w *Worker) runDueCron(ctx context.Context, now time.Time) {
	cfg, err := w.service.Config()
	if err != nil {
		_, _ = AppendEvent(w.service.LogDir, NewEvent("config", EventStatusFailed, EventSeverityError, err.Error()))
		return
	}
	w.runCronJob(ctx, "backup-full", cfg.Schedule.FullBackupCron, now, func(context.Context) error {
		return w.RunBackup(ctx, "full")
	})
	w.runCronJob(ctx, "backup-diff", cfg.Schedule.DifferentialBackupCron, now, func(context.Context) error {
		return w.RunBackup(ctx, "diff")
	})
	w.runCronJob(ctx, "amcheck", cfg.Schedule.AMCheckCron, now, w.RunAMCheck)
	if cfg.Schedule.RestoreVerificationEnabled {
		w.runCronJob(ctx, "restore-verification", cfg.Schedule.RestoreVerificationCron, now, w.RunScheduledRestoreVerification)
	}
}

func (w *Worker) runCronJob(ctx context.Context, key string, spec string, now time.Time, run func(context.Context) error) {
	if !CronMatches(spec, now) {
		return
	}
	runKey := now.Format("2006-01-02T15:04")
	if w.lastCronRuns[key] == runKey {
		return
	}
	w.lastCronRuns[key] = runKey
	if err := run(ctx); err != nil {
		_, _ = AppendEvent(w.service.LogDir, NewEvent(key, EventStatusFailed, EventSeverityError, err.Error()))
	}
}

func (w *Worker) processIntents(ctx context.Context) error {
	intents, err := ListIntents(w.service.StateDir, w.service.IntentSecret)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		var runErr error
		switch intent.Type {
		case IntentTypeBackup:
			runErr = w.RunBackup(ctx, intent.BackupType)
		case IntentTypeRestoreTest:
			runErr = w.RunRestoreTest(ctx, intent.BackupLabel)
		case IntentTypePrimaryRestore:
			runErr = w.RunPrimaryRestore(ctx, intent.BackupLabel)
		default:
			runErr = fmt.Errorf("unsupported intent type %q", intent.Type)
		}
		if runErr != nil {
			event := NewEvent(intent.Type, EventStatusFailed, EventSeverityError, runErr.Error())
			event.Details = map[string]any{"intent_id": intent.ID}
			_, _ = AppendEvent(w.service.LogDir, event)
			if err := CompleteIntent(w.service.StateDir, intent.ID); err != nil {
				return err
			}
			continue
		}
		if err := CompleteIntent(w.service.StateDir, intent.ID); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) RunChecksumStatus(ctx context.Context) error {
	if w.databaseURL == "" {
		return errors.New("database URL is required for checksum status checks")
	}
	output, err := w.runner.Run(ctx, "psql", w.databaseURL, "-Atc", "select current_setting('data_checksums')")
	event := NewEvent(EventOperationChecksum, EventStatusSuccess, EventSeverityInfo, "PostgreSQL checksum status checked.")
	enabled := strings.TrimSpace(output) == "on"
	event.Details = map[string]any{"enabled": enabled, "data_checksums": strings.TrimSpace(output)}
	if err != nil {
		event.Status = EventStatusFailed
		event.Severity = EventSeverityError
		event.Message = "Checksum status check failed: " + err.Error()
		event.Details["output_redacted"] = strings.TrimSpace(output) != ""
	} else if !enabled {
		event.Severity = EventSeverityWarning
		event.Message = "PostgreSQL data checksums are not enabled for this cluster."
	}
	_, appendErr := AppendEvent(w.service.LogDir, event)
	if err != nil {
		return err
	}
	return appendErr
}

func (w *Worker) RunAMCheck(ctx context.Context) error {
	if w.databaseURL == "" {
		return errors.New("database URL is required for pg_amcheck")
	}
	output, err := w.runner.Run(ctx, "pg_amcheck", "--no-dependent-indexes", w.databaseURL)
	event := NewEvent(EventOperationAMCheck, EventStatusSuccess, EventSeverityInfo, "pg_amcheck completed without reported corruption.")
	event.Details = map[string]any{"output_redacted": strings.TrimSpace(output) != ""}
	if err != nil {
		event.Status = EventStatusFailed
		event.Severity = EventSeverityCritical
		event.Message = "pg_amcheck reported a database integrity problem."
		event.Details["error"] = redactAndTruncate(err.Error())
		event.Details["corruption_detected"] = true
	}
	_, appendErr := AppendEvent(w.service.LogDir, event)
	if err != nil {
		return err
	}
	return appendErr
}

func (w *Worker) RunBackup(ctx context.Context, backupType string) error {
	backupType = strings.TrimSpace(strings.ToLower(backupType))
	if backupType == "" {
		backupType = "diff"
	}
	if backupType != "full" && backupType != "diff" {
		return errors.New("backup type must be full or diff")
	}
	start := NewEvent(EventOperationBackup, EventStatusStarted, EventSeverityInfo, "pgBackRest backup started.")
	start.Details = map[string]any{"backup_type": backupType}
	_, _ = AppendEvent(w.service.LogDir, start)

	cfg, _ := w.service.Config()
	if err := w.requireRepoCipherPass(); err != nil {
		return w.recordBackupFailure("Encrypted pgBackRest repository is not configured.", "", err)
	}
	baseArgs := w.pgBackRestArgs(cfg)
	if err := w.preparePgBackRestPaths(ctx, cfg); err != nil {
		return w.recordBackupFailure("Could not prepare pgBackRest directories.", "", err)
	}
	if output, err := w.runPgBackRest(ctx, append(baseArgs, "stanza-create")...); err != nil && !strings.Contains(output, "already exists") {
		return w.recordBackupFailure("pgBackRest stanza creation failed.", output, err)
	}
	if output, err := w.runPgBackRest(ctx, append(baseArgs, "check")...); err != nil {
		return w.recordBackupFailure("pgBackRest check failed.", output, err)
	}
	backupArgs := append(append([]string{}, baseArgs...), "--type="+backupType, fmt.Sprintf("--repo1-retention-full=%d", cfg.Schedule.RetentionFull), "backup")
	output, err := w.runPgBackRest(ctx, backupArgs...)
	if err != nil {
		return w.recordBackupFailure("pgBackRest backup failed.", output, err)
	}
	backups := w.loadBackupInfo(ctx)
	_ = SaveBackupSets(w.service.StateDir, backups)
	label := latestBackupLabel(backups)
	event := NewEvent(EventOperationBackup, EventStatusSuccess, EventSeverityInfo, "pgBackRest backup completed.")
	event.BackupLabel = label
	event.Details = map[string]any{"backup_type": backupType, "output_redacted": strings.TrimSpace(output) != ""}
	_, appendErr := AppendEvent(w.service.LogDir, event)
	return appendErr
}

func (w *Worker) recordBackupFailure(message string, output string, err error) error {
	event := NewEvent(EventOperationBackup, EventStatusFailed, EventSeverityError, message)
	event.Details = map[string]any{"error": redactAndTruncate(err.Error()), "output_redacted": strings.TrimSpace(output) != ""}
	_, _ = AppendEvent(w.service.LogDir, event)
	return fmt.Errorf("%s %w", message, err)
}

func (w *Worker) loadBackupInfo(ctx context.Context) []BackupSet {
	if err := w.requireRepoCipherPass(); err != nil {
		return nil
	}
	cfg, _ := w.service.Config()
	args := append(w.pgBackRestArgs(cfg), "info", "--output=json")
	output, err := w.runPgBackRest(ctx, args...)
	if err != nil {
		return nil
	}
	var decoded []struct {
		Backup []struct {
			Label string `json:"label"`
			Type  string `json:"type"`
			Info  struct {
				Size int64 `json:"size"`
			} `json:"info"`
			Timestamp struct {
				Start int64 `json:"start"`
				Stop  int64 `json:"stop"`
			} `json:"timestamp"`
		} `json:"backup"`
	}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil || len(decoded) == 0 {
		return nil
	}
	backups := make([]BackupSet, 0, len(decoded[0].Backup))
	for _, item := range decoded[0].Backup {
		start := time.Unix(item.Timestamp.Start, 0).UTC()
		stop := time.Unix(item.Timestamp.Stop, 0).UTC()
		backups = append(backups, BackupSet{
			Label:     item.Label,
			Type:      item.Type,
			StartedAt: &start,
			StoppedAt: &stop,
			Status:    EventStatusSuccess,
			SizeBytes: item.Info.Size,
		})
	}
	return backups
}

func (w *Worker) RunRestoreTest(ctx context.Context, backupLabel string) error {
	start := NewEvent(EventOperationRestoreTest, EventStatusStarted, EventSeverityInfo, "Restore verification started.")
	start.BackupLabel = backupLabel
	_, _ = AppendEvent(w.service.LogDir, start)

	if err := w.requireRepoCipherPass(); err != nil {
		return w.recordRestoreFailure(EventOperationRestoreTest, "Encrypted pgBackRest repository is not configured.", backupLabel, err)
	}
	_ = w.composeWithProfile(context.Background(), "restore", "rm", "-sf", "postgres-restore")
	if err := clearDirectoryContents(w.restoreDataPath); err != nil {
		return w.recordRestoreFailure(EventOperationRestoreTest, "Could not clear restore-test data directory.", backupLabel, err)
	}
	_, _ = w.runner.Run(ctx, "chown", "-R", "postgres:postgres", w.restoreDataPath)
	cfg, _ := w.service.Config()
	args := append(w.pgBackRestArgs(cfg), "--pg1-path="+w.restoreDataPath, "--type=immediate", "--target-action=promote")
	if strings.TrimSpace(backupLabel) != "" {
		args = append(args, "--set="+strings.TrimSpace(backupLabel))
	}
	args = append(args, "restore")
	output, err := w.runPgBackRest(ctx, args...)
	if err != nil {
		return w.recordRestoreFailure(EventOperationRestoreTest, "Restore verification failed.", backupLabel, fmt.Errorf("%w: %s", err, output))
	}
	if err := w.composeWithProfile(ctx, "restore", "up", "-d", "postgres-restore"); err != nil {
		return w.recordRestoreFailure(EventOperationRestoreTest, "Restore verification database did not start.", backupLabel, err)
	}
	defer func() {
		_ = w.composeWithProfile(context.Background(), "restore", "stop", "postgres-restore")
	}()
	if err := w.waitForRestoreDatabase(ctx); err != nil {
		return w.recordRestoreFailure(EventOperationRestoreTest, "Restore verification database was not ready.", backupLabel, err)
	}
	if err := w.validateRestoredDatabase(ctx); err != nil {
		return w.recordRestoreFailure(EventOperationRestoreTest, "Restore verification database validation failed.", backupLabel, err)
	}
	event := NewEvent(EventOperationRestoreTest, EventStatusSuccess, EventSeverityInfo, "Restore verification completed.")
	event.BackupLabel = backupLabel
	event.Details = map[string]any{"output_redacted": strings.TrimSpace(output) != "", "table_check": "hank_core_tables", "role_check": "matched"}
	_, appendErr := AppendEvent(w.service.LogDir, event)
	return appendErr
}

func (w *Worker) RunScheduledRestoreVerification(ctx context.Context) error {
	cfg, err := w.service.Config()
	if err != nil {
		return err
	}
	if !cfg.Schedule.RestoreVerificationEnabled {
		return nil
	}
	backups, _ := LoadBackupSets(w.service.StateDir)
	label := latestBackupLabel(backups)
	if label == "" {
		backups = w.loadBackupInfo(ctx)
		_ = SaveBackupSets(w.service.StateDir, backups)
		label = latestBackupLabel(backups)
	}
	if label == "" {
		event := NewEvent(EventOperationRestoreTest, EventStatusFailed, EventSeverityWarning, "Restore verification skipped because no backup is available.")
		_, appendErr := AppendEvent(w.service.LogDir, event)
		return appendErr
	}
	return w.RunRestoreTest(ctx, label)
}

func (w *Worker) RunPrimaryRestore(ctx context.Context, backupLabel string) error {
	start := NewEvent(EventOperationPrimaryRestore, EventStatusStarted, EventSeverityWarning, "Primary database restore started.")
	start.BackupLabel = backupLabel
	_, _ = AppendEvent(w.service.LogDir, start)

	if err := w.requireRepoCipherPass(); err != nil {
		return w.recordRestoreFailure(EventOperationPrimaryRestore, "Encrypted pgBackRest repository is not configured.", backupLabel, err)
	}
	if err := w.compose(ctx, "stop", "cloud", "postgres"); err != nil {
		return w.recordRestoreFailure(EventOperationPrimaryRestore, "Could not stop cloud and postgres before restore.", backupLabel, err)
	}
	if err := w.writeSafetyMarker(backupLabel); err != nil {
		warning := NewEvent(EventOperationPrimaryRestore, EventStatusStarted, EventSeverityWarning, "Primary restore safety marker could not be written.")
		warning.Details = map[string]any{"error": err.Error()}
		_, _ = AppendEvent(w.service.LogDir, warning)
	}
	cfg, _ := w.service.Config()
	args := append(w.pgBackRestArgs(cfg), "--pg1-path="+w.pgDataPath, "--delta", "--type=immediate", "--target-action=promote")
	if strings.TrimSpace(backupLabel) != "" {
		args = append(args, "--set="+strings.TrimSpace(backupLabel))
	}
	args = append(args, "restore")
	_, _ = w.runner.Run(ctx, "chown", "-R", "postgres:postgres", w.pgDataPath)
	output, err := w.runPgBackRest(ctx, args...)
	if err != nil {
		_ = w.compose(ctx, "up", "-d", "postgres", "cloud")
		return w.recordRestoreFailure(EventOperationPrimaryRestore, "Primary database restore failed.", backupLabel, fmt.Errorf("%w: %s", err, output))
	}
	if err := w.compose(ctx, "up", "-d", "postgres"); err != nil {
		return w.recordRestoreFailure(EventOperationPrimaryRestore, "Postgres did not restart after restore.", backupLabel, err)
	}
	time.Sleep(5 * time.Second)
	if err := w.compose(ctx, "up", "-d", "cloud"); err != nil {
		return w.recordRestoreFailure(EventOperationPrimaryRestore, "Cloud did not restart after restore.", backupLabel, err)
	}
	event := NewEvent(EventOperationPrimaryRestore, EventStatusSuccess, EventSeverityInfo, "Primary database restore completed.")
	event.BackupLabel = backupLabel
	event.Details = map[string]any{"output_redacted": strings.TrimSpace(output) != ""}
	_, appendErr := AppendEvent(w.service.LogDir, event)
	return appendErr
}

func (w *Worker) recordRestoreFailure(operation string, message string, backupLabel string, err error) error {
	event := NewEvent(operation, EventStatusFailed, EventSeverityCritical, message)
	event.BackupLabel = backupLabel
	event.Details = map[string]any{"error": redactAndTruncate(err.Error())}
	_, _ = AppendEvent(w.service.LogDir, event)
	return fmt.Errorf("%s %w", message, err)
}

func (w *Worker) compose(ctx context.Context, args ...string) error {
	composeArgs := []string{"compose", "-f", w.composeFile}
	composeArgs = append(composeArgs, args...)
	output, err := w.runner.Run(ctx, "docker", composeArgs...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

func (w *Worker) composeWithProfile(ctx context.Context, profile string, args ...string) error {
	composeArgs := []string{"compose", "-f", w.composeFile}
	if strings.TrimSpace(profile) != "" {
		composeArgs = append(composeArgs, "--profile", strings.TrimSpace(profile))
	}
	composeArgs = append(composeArgs, args...)
	output, err := w.runner.Run(ctx, "docker", composeArgs...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

func (w *Worker) waitForRestoreDatabase(ctx context.Context) error {
	deadline := time.Now().Add(2 * time.Minute)
	var lastOutput string
	var lastErr error
	for time.Now().Before(deadline) {
		output, err := w.runner.Run(ctx, "psql", w.restoreDatabaseURL, "-Atc", "select 1")
		if err == nil && strings.TrimSpace(output) == "1" {
			return nil
		}
		lastOutput = output
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("%w: %s", lastErr, lastOutput)
	}
	return fmt.Errorf("restore database did not answer readiness query: %s", lastOutput)
}

func (w *Worker) validateRestoredDatabase(ctx context.Context) error {
	missingTables, err := w.missingExpectedTables(ctx)
	if err != nil {
		return err
	}
	if missingTables != "" {
		return fmt.Errorf("restored database is missing expected tables: %s", missingTables)
	}
	mainRoles, err := w.loginRoles(ctx, w.databaseURL)
	if err != nil {
		return fmt.Errorf("main database role check: %w", err)
	}
	restoreRoles, err := w.loginRoles(ctx, w.restoreDatabaseURL)
	if err != nil {
		return fmt.Errorf("restore database role check: %w", err)
	}
	if mainRoles != restoreRoles {
		return fmt.Errorf("restored database login roles differ from main database")
	}
	return nil
}

func (w *Worker) missingExpectedTables(ctx context.Context) (string, error) {
	const query = `WITH expected(name) AS (VALUES ('users'), ('homes'), ('home_memberships'), ('agents'), ('agent_tokens'), ('app_sessions'), ('user_notes'), ('home_permissions')) SELECT coalesce(string_agg(name, ',' ORDER BY name), '') FROM expected WHERE to_regclass('public.' || name) IS NULL`
	output, err := w.runner.Run(ctx, "psql", w.restoreDatabaseURL, "-Atc", query)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, redactAndTruncate(output))
	}
	return strings.TrimSpace(output), nil
}

func (w *Worker) loginRoles(ctx context.Context, databaseURL string) (string, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return "", errors.New("database URL is required for role validation")
	}
	const query = `SELECT rolname || '|' || rolsuper || '|' || rolinherit || '|' || rolcreaterole || '|' || rolcreatedb || '|' || rolcanlogin || '|' || rolreplication || '|' || rolbypassrls FROM pg_roles WHERE rolcanlogin AND rolname !~ '^pg_' AND rolname <> 'postgres' ORDER BY rolname`
	output, err := w.runner.Run(ctx, "psql", databaseURL, "-Atc", query)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, redactAndTruncate(output))
	}
	return strings.TrimSpace(output), nil
}

func (w *Worker) runPgBackRest(ctx context.Context, args ...string) (string, error) {
	return w.runner.Run(ctx, "gosu", append([]string{"postgres", "pgbackrest"}, args...)...)
}

func (w *Worker) requireRepoCipherPass() error {
	if strings.TrimSpace(w.repoCipherPass) == "" {
		return errors.New("HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS is required for encrypted pgBackRest backups")
	}
	return nil
}

func (w *Worker) preparePgBackRestPaths(ctx context.Context, cfg Config) error {
	cfg = cfg.Normalized()
	if cfg.Target.Type == TargetTypePosix && strings.TrimSpace(cfg.Target.Path) != "" {
		if _, err := w.runner.Run(ctx, "chown", "-R", "postgres:postgres", cfg.Target.Path); err != nil {
			return err
		}
	}
	_, _ = w.runner.Run(ctx, "chown", "-R", "postgres:postgres", "/var/log/pgbackrest")
	return nil
}

func (w *Worker) writeSafetyMarker(backupLabel string) error {
	path := filepath.Join(w.service.StateDir, "primary-restore-safety-"+time.Now().UTC().Format("20060102T150405Z")+".json")
	data, err := json.MarshalIndent(map[string]any{
		"backup_label": backupLabel,
		"pgdata_path":  w.pgDataPath,
		"created_at":   time.Now().UTC(),
		"note":         "Primary restore requested. pgBackRest repository is the durable restore source.",
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o666)
}

func clearDirectoryContents(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func latestBackupLabel(backups []BackupSet) string {
	if len(backups) == 0 {
		return ""
	}
	return backups[len(backups)-1].Label
}

func truncateOutput(value string) string {
	const maxOutputBytes = 12000
	if len(value) <= maxOutputBytes {
		return value
	}
	return value[:maxOutputBytes] + "\n... truncated ..."
}

func redactAndTruncate(value string) string {
	return truncateOutput(RedactSensitive(value))
}

func (w *Worker) pgBackRestArgs(cfg Config) []string {
	cfg = cfg.Normalized()
	args := []string{"--stanza=" + w.stanza, "--repo1-cipher-type=aes-256-cbc", "--repo1-cipher-pass=" + w.repoCipherPass}
	if cfg.Target.Type == TargetTypePosix && strings.TrimSpace(cfg.Target.Path) != "" {
		args = append(args, "--repo1-path="+cfg.Target.Path)
	}
	return args
}
