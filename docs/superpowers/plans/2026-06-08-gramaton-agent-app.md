# Gramaton Agent App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `/gramaton` as a first-party installable `.hankapp` package and route HankAI media workflow commands through it when installed and enabled.

**Architecture:** The Gramaton app is a stdio package that runs inside the home agent environment and reuses the existing `internal/agent/media` and `internal/agent/files` services, so file writes stay behind the current source-aware policy. Cloud media commands prefer `apps.gramaton.*` capabilities and fall back to compiled `media.*` commands until the package path is proven. The app runtime gains a small event extension so app commands can publish `media.downloads` status events through the existing agent event relay.

**Tech Stack:** Go stdio app executable, existing media/file agent packages, `.hankapp` manifest/schema validation, app runtime events, cloud-to-agent `apps.invoke`, embedded dashboard import UI, demo server Docker Compose deployment.

---

## Scope Boundaries

- Keep Hank auth, assistant shell, files, notes, Home Assistant, dashboard, storage, backup, and restore flows built into HankServerside.
- Do not expose SMB or raw file paths to the public internet.
- Keep app execution inside the home agent boundary.
- Preserve compiled `media.*` fallback commands until app-backed search, plan, download, status, cancel, jobs, settings, and image fetch pass demo validation.

## File Structure

- `internal/agent/apps/runner.go`: add `context` and `events` fields to the stdio request/response contract.
- `internal/agent/apps/manager.go`: emit app-provided events through a manager event sink after successful invokes.
- `internal/agent/client.go`: wire app manager events to existing `agent.event` WebSocket relay.
- `cmd/hank-app-gramaton/main.go`: stdio app command dispatcher for Gramaton/media commands.
- `cmd/hank-app-gramaton/main_test.go`: tests for search/settings/app event behavior.
- `packages/gramaton/app.json`: Gramaton package manifest.
- `packages/gramaton/schemas/*.json`: package config and command schemas.
- `scripts/package-gramaton-app.sh`: build `dist/gramaton.hankapp`.
- `internal/cloud/assistant_media.go`: route search/plan/start through Gramaton app capability when advertised.
- `internal/cloud/assistant_media_settings.go`: route settings/status/jobs/cancel/image through Gramaton app capability when advertised.
- `internal/cloud/assistant_media_test.go`: app-preference coverage for `/gramaton`.

## Tasks

### Task 1: Runtime Events

- [ ] Add failing tests that an app response can include `events`, and manager invokes publish those events through an event sink.
- [ ] Add `Context json.RawMessage` to `AppStdioRequest`.
- [ ] Add `Events []AppStdioEvent` to `AppStdioResponse`.
- [ ] Add `Manager.SetEventSink(func(context.Context, string, string, any) error)`.
- [ ] In `Manager.Invoke`, pass `AppsInvokeRequest.Context` into stdio requests and emit returned events after a successful app response.
- [ ] Wire `agent.Client` to send app events with `sendAgentEvent`.

### Task 2: Gramaton Package App

- [ ] Add `cmd/hank-app-gramaton` tests for settings validation, search request mapping, and `download_status` event output.
- [ ] Implement the Gramaton stdio dispatcher with command IDs:
  `settings_status`, `settings_apply`, `search`, `plan_download`, `download_start`, `download_status`, `download_jobs`, `download_cancel`, and `image_fetch`.
- [ ] Build its media service from app config/secrets plus inherited agent env:
  `HANK_REMOTE_AGENT_FILES_ROOT`, `HANK_REMOTE_SMB_SHARES_JSON`, and existing media env defaults.
- [ ] For `download_start`, persist job state under the app workdir and start a detached worker process so status/jobs/cancel remain available after the stdio request exits.
- [ ] Emit `media.downloads` events in app responses for start/status/cancel/jobs where a job status is returned.

### Task 3: Package Source

- [ ] Create `packages/gramaton/app.json` with runtime command `bin/hank-app-gramaton`.
- [ ] Create JSON schemas for config, settings, search, plan, start, status, jobs, cancel, image, and shared media responses.
- [ ] Create `scripts/package-gramaton-app.sh` and verify it produces `dist/gramaton.hankapp`.
- [ ] Verify the package with `internal/agent/apps.PreviewArchive`.

### Task 4: Cloud Routing

- [ ] Add a `sendMediaCommand` helper that maps compiled `media.*` commands to installed app command IDs and uses `apps.invoke` when the agent advertises `apps.gramaton.<command_id>`.
- [ ] Update assistant media search, plan, start, settings, jobs, cancel, status, and image routes to use the helper.
- [ ] Keep existing compiled media command fallback unchanged when app capability is absent.
- [ ] Add tests proving `/gramaton` prefers `apps.invoke` and existing media fallback still works.

### Task 5: Validation And Demo

- [ ] Run local checks: `gofmt -w ./cmd ./internal`, `go build ./...`, `go test ./...`, `node --check internal/cloud/ui/apps.js`, `scripts/package-hermes-app.sh`, `scripts/package-gramaton-app.sh`, `git diff --check`.
- [ ] Deploy the branch to `/home/campbellservers/HankServerside` on the demo server.
- [ ] Run demo `scripts/doctor.sh`, health/ready checks, and migration status/drift checks on the server.
- [ ] Import/install/enable Hermes and Gramaton app packages on the demo server.
- [ ] Validate app-backed `/Hermes` and `/gramaton` behavior through the demo server and capture exact commands/results.
