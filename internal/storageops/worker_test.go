package storageops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs map[string]string
	errors  map[string]error
	calls   []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, call)
	for key, output := range r.outputs {
		if strings.Contains(call, key) {
			return output, r.errors[key]
		}
	}
	return "", nil
}

type scriptedResponse struct {
	contains string
	output   string
	err      error
}

type scriptedRunner struct {
	responses []scriptedResponse
	calls     []string
}

func (r *scriptedRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, call)
	for _, response := range r.responses {
		if strings.Contains(call, response.contains) {
			return response.output, response.err
		}
	}
	return "", nil
}

func toJSONForTest(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWorkerBackupLogsFailure(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	runner := &fakeRunner{
		outputs: map[string]string{" backup": "backup failed"},
		errors:  map[string]error{" backup": errors.New("exit 1")},
	}
	worker := NewWorker(WorkerOptions{
		StateDir:       stateDir,
		LogDir:         logDir,
		IntentSecret:   "secret",
		RepoCipherPass: "cipher-secret",
		DatabaseURL:    "postgres://example",
		Runner:         runner,
	})

	if err := worker.RunBackup(context.Background(), "full"); err == nil {
		t.Fatal("expected backup failure")
	}
	events, err := ListEvents(logDir, EventFilter{FailuresOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Operation != EventOperationBackup {
		t.Fatalf("failure events = %+v", events)
	}
	if events[0].Details["output_excerpt"] != "backup failed" {
		t.Fatalf("failure output excerpt = %+v", events[0].Details)
	}
}

func TestWorkerRequiresRepoCipherPass(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	worker := NewWorker(WorkerOptions{
		StateDir:     stateDir,
		LogDir:       logDir,
		IntentSecret: "secret",
		DatabaseURL:  "postgres://example",
		Runner:       &fakeRunner{},
	})

	if err := worker.RunBackup(context.Background(), "full"); err == nil {
		t.Fatal("expected missing cipher pass failure")
	}
	events, err := ListEvents(logDir, EventFilter{FailuresOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Operation != EventOperationBackup {
		t.Fatalf("failure events = %+v", events)
	}
}

func TestWorkerAddsEncryptedPgBackRestTypeAndKeepsCipherPassOutOfEvents(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	runner := &fakeRunner{
		outputs: map[string]string{
			" info --output=json": `[{"backup":[{"label":"20260430-010101F","type":"full","info":{"size":42},"timestamp":{"start":1777528861,"stop":1777528871}}]}]`,
		},
	}
	worker := NewWorker(WorkerOptions{
		StateDir:       stateDir,
		LogDir:         logDir,
		IntentSecret:   "secret",
		RepoCipherPass: "cipher-secret",
		DatabaseURL:    "postgres://example",
		Runner:         runner,
	})

	if err := worker.RunBackup(context.Background(), "full"); err != nil {
		t.Fatal(err)
	}
	var encryptedCall string
	for _, call := range runner.calls {
		if strings.Contains(call, " pgbackrest ") && strings.Contains(call, " backup") {
			encryptedCall = call
			break
		}
	}
	if !strings.Contains(encryptedCall, "--repo1-cipher-type=aes-256-cbc") {
		t.Fatalf("backup call missing cipher type: %s", encryptedCall)
	}
	if strings.Contains(encryptedCall, "--repo1-cipher-pass") || strings.Contains(encryptedCall, "cipher-secret") {
		t.Fatalf("backup call leaked cipher pass: %s", encryptedCall)
	}
	events, err := ListEvents(logDir, EventFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		encoded := event.Message
		if event.Details != nil {
			encoded += " " + strings.TrimSpace(toJSONForTest(t, event.Details))
		}
		if strings.Contains(encoded, "cipher-secret") {
			t.Fatalf("event leaked cipher pass: %+v", event)
		}
	}
}

func TestDefaultRunnerPassesPgBackRestCipherPassByEnvironment(t *testing.T) {
	worker := NewWorker(WorkerOptions{RepoCipherPass: " cipher-secret "})
	runner, ok := worker.runner.(ExecRunner)
	if !ok {
		t.Fatalf("runner = %T, want ExecRunner", worker.runner)
	}
	found := false
	for _, item := range runner.Env {
		if item == "PGBACKREST_REPO1_CIPHER_PASS=cipher-secret" {
			found = true
		}
		if strings.Contains(item, " cipher-secret ") {
			t.Fatalf("cipher pass was not trimmed: %q", item)
		}
	}
	if !found {
		t.Fatalf("runner env = %+v", runner.Env)
	}
}

func TestWorkerPublishesBackupTaskStatus(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	runner := &fakeRunner{
		outputs: map[string]string{
			" info --output=json": `[{"backup":[{"label":"20260430-010101F","type":"full","info":{"size":42},"timestamp":{"start":1777528861,"stop":1777528871}}]}]`,
		},
	}
	worker := NewWorker(WorkerOptions{
		StateDir:       stateDir,
		LogDir:         logDir,
		IntentSecret:   "secret",
		RepoCipherPass: "cipher-secret",
		DatabaseURL:    "postgres://example",
		Runner:         runner,
	})

	if err := worker.RunBackup(context.Background(), "full"); err != nil {
		t.Fatal(err)
	}
	task, ok, err := LoadActiveTask(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected active task status")
	}
	if task.Status != TaskStatusSuccess || task.BackupType != "full" || task.BackupLabel != "20260430-010101F" {
		t.Fatalf("task = %+v", task)
	}
}

func TestWorkerChecksumLogsDisabledWarning(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	runner := &fakeRunner{outputs: map[string]string{"psql": "off"}}
	worker := NewWorker(WorkerOptions{
		StateDir:       stateDir,
		LogDir:         logDir,
		IntentSecret:   "secret",
		RepoCipherPass: "cipher-secret",
		DatabaseURL:    "postgres://example",
		Runner:         runner,
	})

	if err := worker.RunChecksumStatus(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := LoadStatus(stateDir, logDir, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if status.Checksum.Enabled {
		t.Fatal("expected checksum disabled status")
	}
	if len(status.Events) != 1 || status.Events[0].Severity != EventSeverityWarning {
		t.Fatalf("events = %+v", status.Events)
	}
}

func TestWorkerAMCheckSetupFailureIsNotMarkedAsCorruption(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	runner := &fakeRunner{
		outputs: map[string]string{"pg_amcheck": "ERROR: extension \"amcheck\" is not installed"},
		errors:  map[string]error{"pg_amcheck": errors.New("exit 1")},
	}
	worker := NewWorker(WorkerOptions{
		StateDir:       stateDir,
		LogDir:         logDir,
		IntentSecret:   "secret",
		RepoCipherPass: "cipher-secret",
		DatabaseURL:    "postgres://example",
		Runner:         runner,
	})

	if err := worker.RunAMCheck(context.Background()); err == nil {
		t.Fatal("expected pg_amcheck failure")
	}
	status, err := LoadStatus(stateDir, logDir, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if status.Checksum.CorruptionDetected {
		t.Fatal("setup failure should not be marked as corruption")
	}
	if len(status.Events) != 1 || status.Events[0].Severity != EventSeverityError {
		t.Fatalf("events = %+v", status.Events)
	}
	if !strings.Contains(runner.calls[0], "--install-missing=pg_catalog") {
		t.Fatalf("pg_amcheck call did not install missing extension: %s", runner.calls[0])
	}
}

func TestWorkerAMCheckCorruptionStaysCritical(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	runner := &fakeRunner{
		outputs: map[string]string{"pg_amcheck": "heap table corruption detected in public.users"},
		errors:  map[string]error{"pg_amcheck": errors.New("exit 1")},
	}
	worker := NewWorker(WorkerOptions{
		StateDir:       stateDir,
		LogDir:         logDir,
		IntentSecret:   "secret",
		RepoCipherPass: "cipher-secret",
		DatabaseURL:    "postgres://example",
		Runner:         runner,
	})

	if err := worker.RunAMCheck(context.Background()); err == nil {
		t.Fatal("expected pg_amcheck failure")
	}
	status, err := LoadStatus(stateDir, logDir, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Checksum.CorruptionDetected {
		t.Fatal("expected corruption to be marked")
	}
	if len(status.Events) != 1 || status.Events[0].Severity != EventSeverityCritical {
		t.Fatalf("events = %+v", status.Events)
	}
}

func TestRestoreValidationComparesLoginRoles(t *testing.T) {
	runner := &scriptedRunner{responses: []scriptedResponse{
		{contains: "to_regclass", output: ""},
		{contains: "postgres://main", output: "hankremote|f|t|f|f|t|f|f"},
		{contains: "postgres://restore", output: "hankremote|t|t|f|f|t|f|f"},
	}}
	worker := NewWorker(WorkerOptions{
		StateDir:           t.TempDir(),
		LogDir:             t.TempDir(),
		IntentSecret:       "secret",
		RepoCipherPass:     "cipher-secret",
		DatabaseURL:        "postgres://main",
		RestoreDataPath:    t.TempDir(),
		RestoreDatabaseURL: "postgres://restore",
		Runner:             runner,
	})

	if err := worker.validateRestoredDatabase(context.Background()); err == nil {
		t.Fatal("expected role drift failure")
	}
}

func TestScheduledRestoreVerificationUsesLatestBackup(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	if err := SaveBackupSets(stateDir, []BackupSet{{Label: "20260430-010101F", Type: "full"}}); err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{responses: []scriptedResponse{
		{contains: "select 1", output: "1"},
		{contains: "to_regclass", output: ""},
		{contains: "postgres://main", output: "hankremote|f|t|f|f|t|f|f"},
		{contains: "postgres://restore", output: "hankremote|f|t|f|f|t|f|f"},
	}}
	worker := NewWorker(WorkerOptions{
		StateDir:           stateDir,
		LogDir:             logDir,
		IntentSecret:       "secret",
		RepoCipherPass:     "cipher-secret",
		DatabaseURL:        "postgres://main",
		RestoreDataPath:    t.TempDir(),
		RestoreDatabaseURL: "postgres://restore",
		Runner:             runner,
	})

	if err := worker.RunScheduledRestoreVerification(context.Background()); err != nil {
		t.Fatal(err)
	}
	foundSet := false
	for _, call := range runner.calls {
		if strings.Contains(call, "--set=20260430-010101F") {
			foundSet = true
			break
		}
	}
	if !foundSet {
		t.Fatalf("restore did not use latest backup label; calls = %+v", runner.calls)
	}
}

func TestWorkerConsumesFailedIntent(t *testing.T) {
	logDir := t.TempDir()
	stateDir := t.TempDir()
	runner := &fakeRunner{
		outputs: map[string]string{" backup": "backup failed"},
		errors:  map[string]error{" backup": errors.New("exit 1")},
	}
	worker := NewWorker(WorkerOptions{
		StateDir:       stateDir,
		LogDir:         logDir,
		IntentSecret:   "secret",
		RepoCipherPass: "cipher-secret",
		DatabaseURL:    "postgres://example",
		Runner:         runner,
	})
	if _, err := CreateIntent(stateDir, "secret", Intent{
		Type:        IntentTypeBackup,
		HomeID:      "home_1",
		RequestedBy: "usr_1",
		BackupType:  "full",
	}); err != nil {
		t.Fatal(err)
	}

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	intents, err := ListIntents(stateDir, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 0 {
		t.Fatalf("expected failed intent to be consumed, got %+v", intents)
	}
	events, err := ListEvents(logDir, EventFilter{FailuresOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("failure events = %+v", events)
	}
}

func TestClearDirectoryContentsPreservesRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.txt"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "two.txt"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := clearDirectoryContents(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty root, got %d entries", len(entries))
	}
}
