# Redacted Settings Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the admin-only Settings Recovery export/import workflow from `docs/superpowers/specs/2026-06-08-redacted-settings-recovery-design.md`.

**Architecture:** Add a focused recovery handler/model file under `internal/cloud` for bundle export, import preview, and non-secret apply. Reuse existing store and service-profile APIs; use the existing service-profile PUT path for final secret submission. Add a dedicated Settings > Recovery pane and lightweight UI for export/upload/preview/apply.

**Tech Stack:** Go HTTP handlers and store methods, JSON recovery bundles, dashboard HTML/CSS/JS, existing HankAPI client and auth/CSRF middleware.

---

### Task 1: Backend Recovery API

**Files:**
- Create: `internal/cloud/recovery.go`
- Create: `internal/cloud/recovery_test.go`
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/home_singleton.go`

- [ ] Write failing tests for admin-only export, secret redaction, import preview, and import apply.
- [ ] Run targeted tests and confirm they fail because recovery routes do not exist.
- [ ] Implement recovery bundle types, export builder, import validator, preview, and non-secret apply.
- [ ] Register `/v1/home/recovery/export`, `/v1/home/recovery/import/preview`, and `/v1/home/recovery/import/apply`.
- [ ] Run targeted tests and confirm they pass.

### Task 2: Settings Recovery UI

**Files:**
- Create: `internal/cloud/ui/recovery.html`
- Create: `internal/cloud/ui/recovery.js`
- Modify: `internal/cloud/ui.go`
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/ui/settings.html`
- Modify: `internal/cloud/ui/settings.js`
- Modify: `internal/cloud/ui/admin-nav.js`
- Modify: `internal/cloud/ui/styles.css`
- Modify: `internal/cloud/server_test.go`

- [ ] Add failing server/UI tests for the Recovery settings pane and asset registration.
- [ ] Run targeted tests and confirm they fail because the pane and assets do not exist.
- [ ] Add the Recovery pane, settings tab, dashboard route, and JS.
- [ ] Add CSS only where existing classes do not cover the recovery preview/checklist.
- [ ] Run targeted tests and JS syntax checks.

### Task 3: Validation

**Files:**
- All touched files.

- [ ] Run `gofmt -w ./cmd ./internal`.
- [ ] Run `go build ./...`.
- [ ] Run `go test ./...`.
- [ ] Run `node --check internal/cloud/ui/recovery.js`.
- [ ] Run `git diff --check`.
- [ ] Report any skipped deployment checks and dirty unrelated files.
