# Installable Agent Apps Runtime And Hermes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first installable agent-app runtime from `docs/superpowers/specs/2026-06-08-installable-agent-apps-design.md`, including `.hankapp` package validation, Settings import/install UI, and Hermes as the first package-backed app.

**Architecture:** Add a generic app protocol and agent-side app runtime. The cloud exposes admin-only app management routes and relays package validation, activation, config, and invocation to the online home agent. Hermes proves the runtime with a simple stdio request/response app; Gramaton uses the same package path for its media workflow commands.

**Tech Stack:** Go HTTP handlers, WebSocket agent commands, PostgreSQL migrations/store methods, zip package validation, stdio process execution, embedded dashboard HTML/CSS/JS, existing HankAPI client and CSRF middleware.

---

## Scope Split

This plan implements Phase 1 and Phase 2 from the design:

- generic runtime and `.hankapp` schema
- package upload/import preview in Settings
- app install, enable/disable, config, and inspect flows
- app invocation through `apps.invoke`
- Hermes package and `/Hermes` routing through the app runtime

This plan does not extract Gramaton. It adds schema and runtime foundations that Gramaton will use, but the Gramaton package needs its own plan because it requires persistent jobs, progress events, file-write policies, cancellation, media-card compatibility, and current downloader verification behavior.

## File Structure

Create focused files with these responsibilities:

- `internal/protocol/apps.go`: shared app command names and request/response payload DTOs.
- `internal/config/config.go`: agent app directory and staging directory settings.
- `internal/agent/apps/manifest.go`: `.hankapp` manifest types and manifest validation.
- `internal/agent/apps/package.go`: zip archive inspection, staging, activation, and path-safety checks.
- `internal/agent/apps/runner.go`: bounded stdio process invocation.
- `internal/agent/apps/manager.go`: app discovery, status, config, package preview/activation, and invocation orchestration.
- `internal/agent/apps/hermes_fixture_test.go`: test fixture helpers for Hermes-shaped packages.
- `internal/agent/commands.go`: dispatch `apps.*` agent commands to the app manager.
- `internal/agent/client.go`: advertise app capabilities on heartbeat.
- `cmd/hank-remote-agent/main.go`: construct the app manager from config.
- `cmd/hank-app-hermes/main.go`: first-party Hermes stdio app executable.
- `packages/hermes/app.json`: Hermes `.hankapp` manifest source.
- `packages/hermes/schemas/*.json`: Hermes config/input/output JSON schemas.
- `scripts/package-hermes-app.sh`: build a local `hermes.hankapp` archive.
- `internal/domain/models.go`: cloud-visible installed app metadata model.
- `internal/store/apps.go`: store methods for app metadata.
- `internal/migrations/sql/000010_agent_apps.up.sql`: app metadata table.
- `internal/cloud/apps.go`: admin-only app management API and package staging/download support.
- `internal/cloud/assistant_tools.go`: dynamic installed-app slash command resolution for `/Hermes`, with compiled fallback.
- `internal/cloud/ui/apps.html`: Settings Apps pane.
- `internal/cloud/ui/apps.js`: import, preview, install, enable/disable, config, and app list UI.
- `internal/cloud/ui/settings.html`, `settings.js`, `admin-nav.js`, `ui.go`, `server.go`: register Settings > Apps pane and assets.

---

### Task 1: Protocol And Agent Config Foundation

**Files:**
- Create: `internal/protocol/apps.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add this test to `internal/config/config_test.go` near the other agent config tests:

```go
func TestLoadAgentParsesAppRuntimePaths(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_CLOUD_URL", "ws://cloud.example/ws/agent")
	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "secret-token")
	t.Setenv("HANK_REMOTE_AGENT_APPS_DIR", " /srv/hank/apps ")
	t.Setenv("HANK_REMOTE_AGENT_APP_STAGING_DIR", " /srv/hank/app-staging ")

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent error: %v", err)
	}
	if cfg.AppsDir != "/srv/hank/apps" {
		t.Fatalf("AppsDir = %q", cfg.AppsDir)
	}
	if cfg.AppStagingDir != "/srv/hank/app-staging" {
		t.Fatalf("AppStagingDir = %q", cfg.AppStagingDir)
	}
}

func TestLoadAgentAppRuntimePathDefaults(t *testing.T) {
	t.Setenv("HANK_REMOTE_AGENT_CLOUD_URL", "ws://cloud.example/ws/agent")
	t.Setenv("HANK_REMOTE_AGENT_ID", "home-main")
	t.Setenv("HANK_REMOTE_AGENT_TOKEN", "secret-token")
	t.Setenv("HANK_REMOTE_AGENT_APPS_DIR", "")
	t.Setenv("HANK_REMOTE_AGENT_APP_STAGING_DIR", "")

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent error: %v", err)
	}
	if cfg.AppsDir != "/var/lib/hank/apps" {
		t.Fatalf("AppsDir = %q", cfg.AppsDir)
	}
	if cfg.AppStagingDir != "/var/lib/hank/app-staging" {
		t.Fatalf("AppStagingDir = %q", cfg.AppStagingDir)
	}
}
```

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
go test ./internal/config -run 'TestLoadAgentParsesAppRuntimePaths|TestLoadAgentAppRuntimePathDefaults' -count=1 -v
```

Expected: FAIL because `config.Agent` does not have `AppsDir` or `AppStagingDir`.

- [ ] **Step 3: Add shared app protocol DTOs**

Create `internal/protocol/apps.go`:

```go
package protocol

import "encoding/json"

const (
	CommandAppsList           = "apps.list"
	CommandAppsPackagePreview = "apps.package_preview"
	CommandAppsPackageActivate = "apps.package_activate"
	CommandAppsConfigStatus   = "apps.config_status"
	CommandAppsConfigApply    = "apps.config_apply"
	CommandAppsInvoke         = "apps.invoke"

	AppSchemaVersion = "hank.app.v1"
	AppRuntimeStdio  = "stdio"
)

type AppSummary struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Version         string          `json:"version"`
	Publisher       string          `json:"publisher,omitempty"`
	Description     string          `json:"description,omitempty"`
	Enabled         bool            `json:"enabled"`
	Status          string          `json:"status"`
	LastError       string          `json:"last_error,omitempty"`
	Capabilities    []string        `json:"capabilities,omitempty"`
	SlashCommands   []AppSlashCommand `json:"slash_commands,omitempty"`
	Commands        []AppCommandSummary `json:"commands,omitempty"`
	PublicConfig    json.RawMessage `json:"public_config,omitempty"`
	SecretFieldsSet map[string]bool `json:"secret_fields_set,omitempty"`
}

type AppSlashCommand struct {
	Command     string `json:"command"`
	CommandID   string `json:"command_id"`
	Description string `json:"description,omitempty"`
}

type AppCommandSummary struct {
	ID             string `json:"id"`
	Mode           string `json:"mode"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	AdminOnly      bool   `json:"admin_only"`
}

type AppsListRequest struct{}

type AppsListResponse struct {
	Apps []AppSummary `json:"apps"`
}

type AppsPackagePreviewRequest struct {
	StagingID    string `json:"staging_id"`
	DownloadURL  string `json:"download_url"`
	DownloadToken string `json:"download_token"`
}

type AppsPackagePreviewResponse struct {
	StagingID string     `json:"staging_id"`
	App       AppSummary `json:"app"`
	Warnings  []string   `json:"warnings,omitempty"`
	Replacing bool       `json:"replacing"`
}

type AppsPackageActivateRequest struct {
	StagingID string `json:"staging_id"`
	Enable    bool   `json:"enable"`
}

type AppsPackageActivateResponse struct {
	App AppSummary `json:"app"`
}

type AppsConfigStatusRequest struct {
	AppID string `json:"app_id,omitempty"`
}

type AppsConfigStatusResponse struct {
	Apps []AppSummary `json:"apps"`
}

type AppsConfigApplyRequest struct {
	AppID        string          `json:"app_id"`
	PublicConfig json.RawMessage `json:"public_config,omitempty"`
	Secrets      json.RawMessage `json:"secrets,omitempty"`
	Enable       *bool           `json:"enable,omitempty"`
}

type AppsConfigApplyResponse struct {
	App AppSummary `json:"app"`
}

type AppsInvokeRequest struct {
	AppID     string          `json:"app_id"`
	CommandID string          `json:"command_id"`
	Input     json.RawMessage `json:"input,omitempty"`
	Context   json.RawMessage `json:"context,omitempty"`
}

type AppsInvokeResponse struct {
	Output json.RawMessage `json:"output,omitempty"`
	JobID  string          `json:"job_id,omitempty"`
}
```

If `gofmt` aligns the const names differently, keep the formatted output.

- [ ] **Step 4: Add agent app config fields**

Modify `internal/config/config.go`:

```go
type Agent struct {
	CloudURL      string
	AgentID       string
	Token         string
	HomeName      string
	ConfigPath    string
	AppsDir       string
	AppStagingDir string
	HA            HomeAssistant
	SMBShares     []SMB
	FilesRoot     string
	NotesRoot     string
	Media         Media
	Hermes        Hermes
}
```

In `LoadAgent`, set:

```go
AppsDir:       envOrDefault("HANK_REMOTE_AGENT_APPS_DIR", "/var/lib/hank/apps"),
AppStagingDir: envOrDefault("HANK_REMOTE_AGENT_APP_STAGING_DIR", "/var/lib/hank/app-staging"),
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
gofmt -w internal/protocol/apps.go internal/config/config.go internal/config/config_test.go
go test ./internal/config ./internal/protocol -count=1
git add internal/protocol/apps.go internal/config/config.go internal/config/config_test.go
git commit -m "Add agent app protocol and config"
```

Expected: tests PASS and commit succeeds.

---

### Task 2: Manifest And Archive Validation

**Files:**
- Create: `internal/agent/apps/manifest.go`
- Create: `internal/agent/apps/package.go`
- Create: `internal/agent/apps/manifest_test.go`

- [ ] **Step 1: Write failing manifest validation tests**

Create `internal/agent/apps/manifest_test.go`:

```go
package apps

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validHermesManifest() Manifest {
	return Manifest{
		SchemaVersion: "hank.app.v1",
		ID:            "hermes",
		Name:          "Hermes",
		Version:       "1.0.0",
		Publisher:     "Hank",
		Description:   "Route explicit /Hermes prompts to a local Hermes API server.",
		Runtime: Runtime{
			Type:    "stdio",
			Command: "bin/hermes-app",
		},
		Assistant: Assistant{
			SlashCommands: []SlashCommand{{
				Command:     "/Hermes",
				CommandID:   "chat",
				Description: "Send a prompt to Hermes.",
			}},
		},
		Commands: []Command{{
			ID:             "chat",
			Mode:           "request_response",
			InputSchema:     "schemas/chat.input.schema.json",
			OutputSchema:    "schemas/chat.output.schema.json",
			TimeoutSeconds: 120,
			AdminOnly:      true,
		}},
		Config: Config{
			Schema:       "schemas/config.schema.json",
			SecretFields: []string{"api_key"},
		},
		Permissions: Permissions{
			Network: []NetworkPermission{{
				Kind:  "configured_base_url",
				Field: "api_base_url",
			}},
		},
	}
}

func TestValidateManifestAcceptsHermesShape(t *testing.T) {
	t.Parallel()
	if err := ValidateManifest(validHermesManifest()); err != nil {
		t.Fatalf("ValidateManifest error: %v", err)
	}
}

func TestValidateManifestRejectsUnsafeIDsAndPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"bad app id", func(m *Manifest) { m.ID = "../bad" }, "app id"},
		{"bad command path", func(m *Manifest) { m.Runtime.Command = "../bad" }, "runtime command"},
		{"bad schema path", func(m *Manifest) { m.Commands[0].InputSchema = "/tmp/schema.json" }, "schema path"},
		{"unknown permission", func(m *Manifest) { m.Permissions.Network[0].Kind = "internet" }, "permission"},
		{"missing command", func(m *Manifest) { m.Commands = nil }, "command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validHermesManifest()
			tt.mutate(&manifest)
			err := ValidateManifest(manifest)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateManifest error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPreviewArchiveRejectsTraversal(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "bad.hankapp")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	writer, err := zw.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = PreviewArchive(archivePath)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("PreviewArchive error = %v", err)
	}
}

func TestPreviewArchiveAcceptsHermesPackage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifest := validHermesManifest()
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(dir, "hermes.hankapp")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, body := range map[string]string{
		"app.json":                         string(rawManifest),
		"bin/hermes-app":                   "#!/bin/sh\n",
		"schemas/config.schema.json":       `{"type":"object"}`,
		"schemas/chat.input.schema.json":   `{"type":"object"}`,
		"schemas/chat.output.schema.json":  `{"type":"object"}`,
	} {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewArchive(archivePath)
	if err != nil {
		t.Fatalf("PreviewArchive error: %v", err)
	}
	if preview.Manifest.ID != "hermes" || preview.Manifest.Commands[0].ID != "chat" {
		t.Fatalf("preview = %#v", preview)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/agent/apps -run 'TestValidateManifest|TestPreviewArchive' -count=1 -v
```

Expected: FAIL because package `internal/agent/apps` does not exist.

- [ ] **Step 3: Implement manifest types and validation**

Create `internal/agent/apps/manifest.go` with exported `Manifest`, `Runtime`, `Assistant`, `SlashCommand`, `Command`, `Config`, `Permissions`, `NetworkPermission`, and `ValidateManifest`. Use strict regexes:

```go
var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var slashCommandPattern = regexp.MustCompile(`^/[A-Za-z][A-Za-z0-9_-]*$`)
```

Validation requirements:

- `schema_version` equals `protocol.AppSchemaVersion`.
- `id` and command ids match `identifierPattern`.
- runtime type equals `protocol.AppRuntimeStdio`.
- runtime command and schema paths are relative clean paths under the app directory.
- at least one command exists.
- slash commands refer to declared command ids.
- network permission kind is `configured_base_url`.
- files/events permissions may be empty in this task.

- [ ] **Step 4: Implement archive preview**

Create `internal/agent/apps/package.go` with:

```go
type PackagePreview struct {
	Manifest Manifest
	Warnings []string
}

func PreviewArchive(path string) (PackagePreview, error)
```

Implementation requirements:

- open zip archive
- reject absolute paths, `..`, duplicate paths, directories with unsafe names, and symlink entries
- require `app.json`
- decode `app.json`
- call `ValidateManifest`
- verify runtime command exists in archive
- verify every referenced schema exists in archive

- [ ] **Step 5: Run tests and commit**

Run:

```bash
gofmt -w internal/agent/apps
go test ./internal/agent/apps -run 'TestValidateManifest|TestPreviewArchive' -count=1 -v
git add internal/agent/apps
git commit -m "Add agent app package validation"
```

Expected: tests PASS and commit succeeds.

---

### Task 3: Bounded Stdio Runtime

**Files:**
- Create: `internal/agent/apps/runner.go`
- Create: `internal/agent/apps/runner_test.go`

- [ ] **Step 1: Write failing runner tests**

Create `internal/agent/apps/runner_test.go`:

```go
package apps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeExecutable(t *testing.T, dir string, body string) string {
	t.Helper()
	path := filepath.Join(dir, "app.sh")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRunnerInvokeReturnsOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := writeExecutable(t, dir, "#!/bin/sh\nread line\nprintf '%s\n' '{\"request_id\":\"req_1\",\"ok\":true,\"output\":{\"text\":\"ok\"}}'\n")
	runner := Runner{MaxOutputBytes: 4096, MaxStderrBytes: 1024}
	response, err := runner.Invoke(context.Background(), InvokeSpec{
		Executable: exe,
		WorkDir:    dir,
		Timeout:    time.Second,
		Request: AppStdioRequest{
			ProtocolVersion: "hank.app.stdio.v1",
			RequestID:       "req_1",
			AppID:           "hermes",
			CommandID:       "chat",
			Input:           json.RawMessage(`{"prompt":"hello"}`),
		},
	})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if !response.OK || string(response.Output) != `{"text":"ok"}` {
		t.Fatalf("response = %#v", response)
	}
}

func TestRunnerInvokeRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := writeExecutable(t, dir, "#!/bin/sh\nprintf '%s\n' 'not json'\n")
	runner := Runner{MaxOutputBytes: 4096, MaxStderrBytes: 1024}
	_, err := runner.Invoke(context.Background(), InvokeSpec{
		Executable: exe,
		WorkDir:    dir,
		Timeout:    time.Second,
		Request:    AppStdioRequest{RequestID: "req_1"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid app response") {
		t.Fatalf("Invoke error = %v", err)
	}
}

func TestRunnerInvokeTimesOut(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := writeExecutable(t, dir, "#!/bin/sh\nsleep 2\n")
	runner := Runner{MaxOutputBytes: 4096, MaxStderrBytes: 1024}
	_, err := runner.Invoke(context.Background(), InvokeSpec{
		Executable: exe,
		WorkDir:    dir,
		Timeout:    20 * time.Millisecond,
		Request:    AppStdioRequest{RequestID: "req_1"},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Invoke error = %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/agent/apps -run 'TestRunnerInvoke' -count=1 -v
```

Expected: FAIL because `Runner`, `InvokeSpec`, and stdio request/response types do not exist.

- [ ] **Step 3: Implement runner**

Create `internal/agent/apps/runner.go` with:

```go
type AppStdioRequest struct {
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	AppID           string          `json:"app_id"`
	CommandID       string          `json:"command_id"`
	Config          json.RawMessage `json:"config,omitempty"`
	Secrets         json.RawMessage `json:"secrets,omitempty"`
	Input           json.RawMessage `json:"input,omitempty"`
}

type AppStdioResponse struct {
	RequestID string          `json:"request_id"`
	OK        bool            `json:"ok"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     *AppError       `json:"error,omitempty"`
}

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InvokeSpec struct {
	Executable string
	Args       []string
	WorkDir    string
	Timeout    time.Duration
	Request    AppStdioRequest
}

type Runner struct {
	MaxOutputBytes int64
	MaxStderrBytes int64
}
```

Use `exec.CommandContext`, write one JSON line to stdin, read stdout/stderr with limits, kill on timeout, unmarshal one response, and return `invalid app response` for bad JSON.

- [ ] **Step 4: Run tests and commit**

Run:

```bash
gofmt -w internal/agent/apps
go test ./internal/agent/apps -run 'TestRunnerInvoke' -count=1 -v
git add internal/agent/apps/runner.go internal/agent/apps/runner_test.go
git commit -m "Add stdio app runner"
```

Expected: tests PASS and commit succeeds.

---

### Task 4: Agent App Manager And Dispatch

**Files:**
- Create: `internal/agent/apps/manager.go`
- Create: `internal/agent/apps/manager_test.go`
- Modify: `internal/agent/commands.go`
- Modify: `internal/agent/client.go`
- Modify: `cmd/hank-remote-agent/main.go`

- [ ] **Step 1: Write failing manager tests**

Create `internal/agent/apps/manager_test.go` with tests for:

- empty app directory returns no apps
- preview stages a valid Hermes package
- activate installs the staged package disabled by default
- config apply sets public config, secrets-set metadata, and enabled state
- invoke refuses disabled app
- invoke runs enabled Hermes-shaped app and returns output

Use these test names:

```go
func TestManagerListEmptyDirectory(t *testing.T)
func TestManagerPreviewAndActivateHermesPackage(t *testing.T)
func TestManagerConfigApplyEnablesAppAndTracksSecrets(t *testing.T)
func TestManagerInvokeRefusesDisabledApp(t *testing.T)
func TestManagerInvokeRunsEnabledApp(t *testing.T)
```

- [ ] **Step 2: Run manager tests and verify failure**

Run:

```bash
go test ./internal/agent/apps -run 'TestManager' -count=1 -v
```

Expected: FAIL because `Manager` does not exist.

- [ ] **Step 3: Implement manager**

Create `internal/agent/apps/manager.go` with:

```go
type Manager struct {
	appsDir    string
	stagingDir string
	runner     Runner
	mu         sync.RWMutex
	apps       map[string]*InstalledApp
	staged     map[string]PackagePreview
}

type InstalledApp struct {
	Manifest     Manifest
	Path         string
	Enabled      bool
	PublicConfig json.RawMessage
	Secrets      json.RawMessage
	SecretSet    map[string]bool
	Status       string
	LastError    string
}
```

Required methods:

```go
func NewManager(appsDir string, stagingDir string, runner Runner) *Manager
func (m *Manager) Load(ctx context.Context) error
func (m *Manager) List(ctx context.Context) protocol.AppsListResponse
func (m *Manager) PreviewPackage(ctx context.Context, request protocol.AppsPackagePreviewRequest) (protocol.AppsPackagePreviewResponse, error)
func (m *Manager) ActivatePackage(ctx context.Context, request protocol.AppsPackageActivateRequest) (protocol.AppsPackageActivateResponse, error)
func (m *Manager) ConfigStatus(ctx context.Context, request protocol.AppsConfigStatusRequest) (protocol.AppsConfigStatusResponse, error)
func (m *Manager) ConfigApply(ctx context.Context, request protocol.AppsConfigApplyRequest) (protocol.AppsConfigApplyResponse, error)
func (m *Manager) Invoke(ctx context.Context, request protocol.AppsInvokeRequest) (protocol.AppsInvokeResponse, error)
func (m *Manager) Capabilities() []string
```

For this task, `PreviewPackage` may support local `DownloadURL` values with `file://` in tests and HTTP(S) downloads in production. It must write package bytes to `stagingDir`, call `PreviewArchive`, and retain the preview by `StagingID`.

For `Invoke`, convert command output into `protocol.AppsInvokeResponse{Output: response.Output}` and reject disabled or unknown apps.

- [ ] **Step 4: Wire manager into agent dispatch**

Modify `internal/agent/commands.go`:

- Add `apps *agentapps.Manager` to `commandDispatcher`.
- Add switch cases for `protocol.CommandAppsList`, `CommandAppsPackagePreview`, `CommandAppsPackageActivate`, `CommandAppsConfigStatus`, `CommandAppsConfigApply`, and `CommandAppsInvoke`.
- Decode each protocol request and call the manager.
- Map JSON decode failures to `badRequest("invalid_app_request", err)`. Map unknown app, disabled app, missing staging package, package validation failure, and permission refusal through `mapError(err)` so callers get structured agent errors without losing the original message.

Modify `internal/agent/client.go`:

- Add an `apps *agentapps.Manager` parameter to `NewClient`.
- If nil, construct an empty manager with blank dirs for tests.
- In `capabilities`, append generic app management commands when the manager is non-nil.
- Append `apps.<app_id>.<command_id>` capabilities from `manager.Capabilities()`.

Modify `cmd/hank-remote-agent/main.go`:

```go
appManager := agentapps.NewManager(cfg.AppsDir, cfg.AppStagingDir, agentapps.Runner{
	MaxOutputBytes: 1 << 20,
	MaxStderrBytes: 16 << 10,
})
if err := appManager.Load(context.Background()); err != nil {
	logger.Warn("failed to load agent apps", "error", err)
}
client := agent.NewClient(cfg.CloudURL, cfg.AgentID, cfg.Token, cfg.HomeName, cfg.ConfigPath, ha, files, media, notes, hermes, appManager, logger)
```

- [ ] **Step 5: Update compile errors and tests**

Update all `agent.NewClient` call sites in tests by passing `nil` for the new app manager parameter before `logger`.

Run:

```bash
gofmt -w internal/agent cmd/hank-remote-agent/main.go
go test ./internal/agent ./internal/agent/apps -count=1
git add internal/agent cmd/hank-remote-agent/main.go
git commit -m "Wire app runtime into home agent"
```

Expected: tests PASS and commit succeeds.

---

### Task 5: Cloud App Store, Routes, And Package Staging

**Files:**
- Create: `internal/migrations/sql/000010_agent_apps.up.sql`
- Modify: `internal/domain/models.go`
- Create: `internal/store/apps.go`
- Create: `internal/store/apps_test.go`
- Create: `internal/cloud/apps.go`
- Create: `internal/cloud/apps_test.go`
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/home_singleton.go`

- [ ] **Step 1: Write store tests**

Create `internal/store/apps_test.go` with tests:

```go
func TestAppMetadataStoreRoundTrip(t *testing.T)
func TestAppMetadataListOrdersByName(t *testing.T)
```

Use `testutil` patterns already used by store tests. Assert that `UpsertHomeApp`, `GetHomeApp`, and `ListHomeApps` preserve `home_id`, `app_id`, `version`, `enabled`, `public_config_json`, `secret_fields_set_json`, `status`, `last_error`, `updated_by`, and timestamps.

- [ ] **Step 2: Add migration and store model**

Create `internal/migrations/sql/000010_agent_apps.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS home_agent_apps (
	home_id TEXT NOT NULL REFERENCES homes(id) ON DELETE CASCADE,
	app_id TEXT NOT NULL,
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	enabled BOOLEAN NOT NULL DEFAULT FALSE,
	public_config_json TEXT NOT NULL DEFAULT '{}',
	secret_fields_set_json TEXT NOT NULL DEFAULT '{}',
	status TEXT NOT NULL DEFAULT 'pending',
	last_error TEXT NOT NULL DEFAULT '',
	updated_at TIMESTAMP NOT NULL,
	updated_by TEXT NOT NULL,
	PRIMARY KEY (home_id, app_id)
);
```

Modify `internal/domain/models.go`:

```go
type HomeAgentApp struct {
	HomeID              string    `json:"home_id"`
	AppID               string    `json:"app_id"`
	Name                string    `json:"name"`
	Version             string    `json:"version"`
	Enabled             bool      `json:"enabled"`
	PublicConfigJSON    string    `json:"public_config_json,omitempty"`
	SecretFieldsSetJSON string    `json:"secret_fields_set_json,omitempty"`
	Status              string    `json:"status"`
	LastError           string    `json:"last_error,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
	UpdatedBy           string    `json:"updated_by"`
}
```

Create `internal/store/apps.go` with:

```go
func (s *Store) UpsertHomeApp(ctx context.Context, app domain.HomeAgentApp) error
func (s *Store) GetHomeApp(ctx context.Context, homeID string, appID string) (domain.HomeAgentApp, error)
func (s *Store) ListHomeApps(ctx context.Context, homeID string) ([]domain.HomeAgentApp, error)
```

- [ ] **Step 3: Write cloud API tests**

Create `internal/cloud/apps_test.go` with tests:

```go
func TestAppsListRequiresHomeMembership(t *testing.T)
func TestAppsImportPreviewRequiresAdminAndOnlineAgent(t *testing.T)
func TestAppsImportPreviewRoutesPackageToAgent(t *testing.T)
func TestAppsActivatePersistsReturnedAppMetadata(t *testing.T)
func TestAppsConfigApplyRoutesToAgent(t *testing.T)
```

Use existing `setupServerAndAgent` helpers from cloud tests. Verify:

- member can list apps but cannot import/activate/configure
- admin can import preview only when agent is online
- preview sends `protocol.CommandAppsPackagePreview`
- activate sends `protocol.CommandAppsPackageActivate`
- config apply sends `protocol.CommandAppsConfigApply`
- returned app metadata is persisted with redacted secret-set metadata only

- [ ] **Step 4: Implement cloud package staging**

Create `internal/cloud/apps.go` with:

- in-memory package staging registry on `Server`
- max package size constant of `32 << 20`
- admin-only handlers under `/v1/home/apps`
- package download endpoint for the online agent

Add to `Server`:

```go
appPackages *appPackageStagingRegistry
```

Initialize in `NewServer`:

```go
appPackages: newAppPackageStagingRegistry(),
```

Routes handled by `handleHomeApps`:

- `GET /v1/home/apps`
- `POST /v1/home/apps/import/preview`
- `POST /v1/home/apps/import/activate`
- `GET /v1/home/apps/packages/{stagingID}`
- `PUT /v1/home/apps/{appID}/config`

The package download route must require:

- authenticated agent bearer token using `X-Hank-Agent-ID` and `Authorization: Bearer`
- matching staging download token from `X-Hank-App-Package-Token`
- staging record for the same home

Implement a small helper in `internal/cloud/apps.go`:

```go
func (s *Server) authenticateAgentPackageDownload(r *http.Request, homeID string) (domain.Agent, bool)
```

Use the same token hash path as agent websocket auth. Do not log raw package tokens.

- [ ] **Step 5: Register cloud handler**

Modify `internal/cloud/home_singleton.go`:

```go
if s.handleHomeApps(w, r, home, auth, membership, parts) {
	return
}
```

Place it near service profiles and recovery.

- [ ] **Step 6: Run tests and commit**

Run:

```bash
gofmt -w internal/domain/models.go internal/store/apps.go internal/store/apps_test.go internal/cloud/apps.go internal/cloud/apps_test.go internal/cloud/server.go internal/cloud/home_singleton.go
go test ./internal/store ./internal/cloud -run 'TestAppMetadata|TestApps' -count=1 -v
go test ./internal/migrations -run Test -count=1
git add internal/domain/models.go internal/store/apps.go internal/store/apps_test.go internal/cloud/apps.go internal/cloud/apps_test.go internal/cloud/server.go internal/cloud/home_singleton.go internal/migrations/sql/000010_agent_apps.up.sql
git commit -m "Add cloud app management API"
```

Expected: tests PASS and commit succeeds. If DB-backed store tests require `HANK_REMOTE_TEST_DATABASE_URL`, record the skip explicitly in the final validation note.

---

### Task 6: Settings Apps Pane

**Files:**
- Create: `internal/cloud/ui/apps.html`
- Create: `internal/cloud/ui/apps.js`
- Modify: `internal/cloud/ui.go`
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/ui/settings.html`
- Modify: `internal/cloud/ui/settings.js`
- Modify: `internal/cloud/ui/admin-nav.js`
- Modify: `internal/cloud/ui/styles.css`
- Modify: `internal/cloud/server_test.go`

- [ ] **Step 1: Add failing UI route and asset tests**

Modify `internal/cloud/server_test.go`:

- Add `/dashboard/settings/apps-pane` to the authenticated dashboard route list.
- Add a member-forbidden check for `/dashboard/settings/apps-pane`.
- Add an asset check for `/assets/apps.js`.
- Add `settings.html` assertions for:

```html
data-settings-page-tab="apps" data-admin-only="true" hidden
data-settings-page-panel="apps" data-admin-only="true" hidden
data-src="/dashboard/settings/apps-pane?embedded=1"
```

- Add `admin-nav.js` assertion for:

```js
href: "/dashboard/settings#apps"
adminOnly: true
```

- [ ] **Step 2: Run UI tests and verify failure**

Run:

```bash
go test ./internal/cloud -run 'TestDashboard|TestSettings|TestUIAsset|TestAdminNav' -count=1 -v
```

Expected: FAIL because Apps pane route and asset do not exist.

- [ ] **Step 3: Add Apps pane route and asset registration**

Modify `internal/cloud/ui.go`:

```go
func (s *Server) handleSettingsAppsPane(w http.ResponseWriter, r *http.Request) {
	s.serveAdminUIPage(w, r, "/dashboard/settings/apps-pane", "apps.html")
}
```

Add `apps.js` to `serveUIAsset`.

Modify `internal/cloud/server.go`:

```go
mux.HandleFunc("/dashboard/settings/apps-pane", server.handleSettingsAppsPane)
```

- [ ] **Step 4: Add Settings tab and nav entry**

Modify `internal/cloud/ui/settings.html`:

```html
<button type="button" class="settings-page-tab" role="tab" aria-selected="false" data-settings-page-tab="apps" data-admin-only="true" hidden>Apps</button>
```

Add panel:

```html
<section class="settings-frame-panel" data-settings-page-panel="apps" data-admin-only="true" hidden>
  <iframe class="settings-pane-frame" title="App settings" data-src="/dashboard/settings/apps-pane?embedded=1"></iframe>
</section>
```

Modify `internal/cloud/ui/settings.js`:

```js
apps: "apps",
app: "apps",
packages: "apps",
```

Modify `internal/cloud/ui/admin-nav.js` with an admin-only Settings Apps search item.

- [ ] **Step 5: Add Apps pane HTML and JS**

Create `internal/cloud/ui/apps.html` using existing panel/card classes. It must include:

- installed apps section with `id="apps-list"`
- import form with `id="app-import-form"` and file input accepting `.hankapp`
- preview section with `id="app-preview"`
- install button `id="app-install-button"`
- config form container `id="app-config-panel"`

Create `internal/cloud/ui/apps.js` with:

- `loadApps()` calls `GET /v1/home/apps`
- import preview posts `FormData` to `/v1/home/apps/import/preview`
- install posts preview `staging_id` to `/v1/home/apps/import/activate`
- enable/disable/config save calls `/v1/home/apps/{appID}/config`
- secret inputs are cleared after save
- package preview renders name, version, publisher, slash commands, permissions, and config fields

Use `window.HankAPI.request` for JSON routes. For `FormData`, call `fetch` with `credentials: "same-origin"` and include the existing CSRF header by reading `window.HankAPI.csrfToken()` if available; if the helper does not expose it, add a small helper to `api-client.js` in this task and update existing calls only where necessary.

- [ ] **Step 6: Add minimal CSS**

Modify `internal/cloud/ui/styles.css` only if existing `.card-list`, `.settings-grid`, `.status-chip`, `.actions`, and `.panel` classes cannot render the preview cleanly. Add app-specific selectors under a short block:

```css
.app-permission-list,
.app-command-list {
  display: grid;
  gap: 8px;
}
```

- [ ] **Step 7: Run UI checks and commit**

Run:

```bash
gofmt -w internal/cloud
node --check internal/cloud/ui/apps.js
node --check internal/cloud/ui/settings.js
node --check internal/cloud/ui/admin-nav.js
go test ./internal/cloud -run 'TestDashboard|TestSettings|TestUIAsset|TestAdminNav' -count=1 -v
git add internal/cloud/ui.go internal/cloud/server.go internal/cloud/server_test.go internal/cloud/ui/apps.html internal/cloud/ui/apps.js internal/cloud/ui/settings.html internal/cloud/ui/settings.js internal/cloud/ui/admin-nav.js internal/cloud/ui/styles.css internal/cloud/ui/api-client.js
git commit -m "Add Settings Apps import UI"
```

Expected: checks PASS and commit succeeds. If `api-client.js` is unchanged, omit it from `git add`.

---

### Task 7: Hermes App Package And Assistant Routing

**Files:**
- Create: `cmd/hank-app-hermes/main.go`
- Create: `cmd/hank-app-hermes/main_test.go`
- Create: `packages/hermes/app.json`
- Create: `packages/hermes/schemas/config.schema.json`
- Create: `packages/hermes/schemas/chat.input.schema.json`
- Create: `packages/hermes/schemas/chat.output.schema.json`
- Create: `scripts/package-hermes-app.sh`
- Modify: `internal/cloud/assistant_tools.go`
- Modify: `internal/cloud/assistant_workflow_test.go`

- [ ] **Step 1: Write Hermes app executable tests**

Create `cmd/hank-app-hermes/main_test.go` with a test that:

- starts an `httptest.Server`
- sends one stdio request to the app command handler function
- verifies `/v1/responses` request body, bearer header, and output JSON

Use a testable function:

```go
func run(ctx context.Context, input io.Reader, output io.Writer, stderr io.Writer, client *http.Client) int
```

Test names:

```go
func TestHermesAppRunChatSuccess(t *testing.T)
func TestHermesAppRunRejectsEmptyPrompt(t *testing.T)
func TestHermesAppRunReturnsUpstreamError(t *testing.T)
```

- [ ] **Step 2: Implement Hermes stdio app**

Create `cmd/hank-app-hermes/main.go`:

- decode one `apps.AppStdioRequest` from stdin
- require `command_id == "chat"`
- read config fields `api_base_url`, `model`, `timeout_seconds`
- read secret field `api_key`
- read input fields `prompt`, `conversation_id`, `session_key`
- call Hermes `POST /v1/responses` with bearer auth
- write one `apps.AppStdioResponse` JSON line to stdout
- write only sanitized errors to stderr

The output shape must match:

```json
{
  "request_id": "req_123",
  "ok": true,
  "output": {
    "text": "Hermes answer",
    "model": "hermes-agent",
    "response_id": "resp_123",
    "conversation_id": "conv_123"
  }
}
```

- [ ] **Step 3: Add Hermes package source**

Create `packages/hermes/app.json` using the manifest from the design, with runtime command:

```json
"command": "bin/hank-app-hermes"
```

Create config schema requiring:

```json
{
  "type": "object",
  "required": ["api_base_url", "model", "timeout_seconds"],
  "properties": {
    "api_base_url": { "type": "string", "format": "uri" },
    "model": { "type": "string", "default": "hermes-agent" },
    "timeout_seconds": { "type": "integer", "minimum": 1, "maximum": 300, "default": 120 },
    "api_key": { "type": "string", "writeOnly": true }
  }
}
```

Create input/output schemas for the chat command with the fields from the design.

- [ ] **Step 4: Add package script**

Create `scripts/package-hermes-app.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/dist/hermes.hankapp"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

mkdir -p "${TMP}/bin" "${TMP}/schemas" "${ROOT}/dist"
go build -o "${TMP}/bin/hank-app-hermes" "${ROOT}/cmd/hank-app-hermes"
cp "${ROOT}/packages/hermes/app.json" "${TMP}/app.json"
cp "${ROOT}/packages/hermes/schemas/"*.json "${TMP}/schemas/"
cp "${ROOT}/packages/hermes/README.md" "${TMP}/README.md" 2>/dev/null || true
(cd "${TMP}" && zip -qr "${OUT}" .)
echo "${OUT}"
```

Set executable bit in the implementation task:

```bash
chmod +x scripts/package-hermes-app.sh
```

- [ ] **Step 5: Add app-backed assistant routing**

Modify `internal/cloud/assistant_tools.go`:

- In `executeAssistantHermesChatTool`, first attempt installed app invocation when the online agent advertises `apps.hermes.chat`.
- Send `protocol.CommandAppsInvoke` with `app_id: "hermes"`, `command_id: "chat"`, and the prompt/conversation/session fields expected by the Hermes app package.
- Decode output into `protocol.HermesChatResponse`.
- If app capability is missing, report that Hermes is not configured on the home agent.

Add helper:

```go
func (s *Server) agentHasCapability(homeID string, capability string) bool {
	if agent, ok := s.router.GetAgent(homeID); ok {
		return slices.Contains(agent.capabilities, capability)
	}
	return false
}
```

Use an existing helper if one already exists by the time this task is executed.

- [ ] **Step 6: Add assistant workflow test**

Modify `internal/cloud/assistant_workflow_test.go` with a new test:

```go
func TestAssistantHermesCommandPrefersInstalledApp(t *testing.T)
```

Set up agent capabilities containing `apps.hermes.chat`, serve an expected `apps.invoke` command, return output matching `protocol.HermesChatResponse`, and assert:

- assistant text is the app output
- request command is `protocol.CommandAppsInvoke`
- request body has `app_id == "hermes"` and `command_id == "chat"`
- compiled `hermes.chat` command is not sent

Keep the existing `TestAssistantHermesCommandRoutesThroughAgent` as fallback coverage.

- [ ] **Step 7: Run tests and commit**

Run:

```bash
gofmt -w cmd/hank-app-hermes internal/cloud
go test ./cmd/hank-app-hermes ./internal/cloud -run 'TestHermesAppRun|TestAssistantHermesCommand' -count=1 -v
scripts/package-hermes-app.sh
go test ./internal/agent/apps -run TestPreviewArchiveAcceptsHermesPackage -count=1 -v
git add cmd/hank-app-hermes packages/hermes scripts/package-hermes-app.sh internal/cloud/assistant_tools.go internal/cloud/assistant_workflow_test.go
git commit -m "Add Hermes installable app package"
```

Expected: tests PASS, `dist/hermes.hankapp` is produced locally but should not be committed unless the repo already tracks generated archives.

---

### Task 8: Full Validation And Gramaton Handoff

**Files:**
- Modify: `docs/superpowers/specs/2026-06-08-installable-agent-apps-design.md`
- Create: `docs/superpowers/plans/2026-06-08-gramaton-agent-app.md`

- [ ] **Step 1: Run repository validation**

Run:

```bash
gofmt -w ./cmd ./internal
go build ./...
go test ./...
git diff --check
```

Expected: all commands PASS. If DB-backed tests skip because `HANK_REMOTE_TEST_DATABASE_URL` is unset, record that explicitly.

- [ ] **Step 2: Run package-level checks**

Run:

```bash
node --check internal/cloud/ui/apps.js
scripts/package-hermes-app.sh
go test ./internal/agent/apps ./cmd/hank-app-hermes -count=1
```

Expected: all commands PASS and the Hermes package script prints `dist/hermes.hankapp`.

- [ ] **Step 3: Update design status**

Modify `docs/superpowers/specs/2026-06-08-installable-agent-apps-design.md` status line:

```markdown
Status: Runtime and Hermes implementation complete; Gramaton extraction pending follow-up implementation plan.
```

- [ ] **Step 4: Create Gramaton follow-up plan stub with real scope**

Create `docs/superpowers/plans/2026-06-08-gramaton-agent-app.md` with a short approved-handoff plan header and scope boundaries. Include the exact preserved requirements:

- source-aware destination selection
- login and destination validation before saving settings
- ranged download verification with fallback to single-stream download
- `media.downloads` event publishing
- download status, cancel, and jobs commands
- no writes outside selected SMB source/destination policy

Do not implement Gramaton in this task.

- [ ] **Step 5: Commit final status docs**

Run:

```bash
git add docs/superpowers/specs/2026-06-08-installable-agent-apps-design.md docs/superpowers/plans/2026-06-08-gramaton-agent-app.md
git commit -m "Document Gramaton app follow-up"
```

Expected: commit succeeds.

---

## Final Verification Checklist

Run before reporting implementation complete:

```bash
gofmt -w ./cmd ./internal
go build ./...
go test ./...
node --check internal/cloud/ui/apps.js
scripts/package-hermes-app.sh
git diff --check
```

Also run database checks when a database is available:

```bash
make migrate-status
make schema-drift-check
```

Report:

- Security impact: admin-only app routes, package validation, agent-side secrets, and no direct SMB exposure.
- Database impact: migration `000010_agent_apps.up.sql` and whether migration checks ran.
- Validation: exact commands run and any skips.
