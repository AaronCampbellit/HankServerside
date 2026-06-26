# Authenticated User Action Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Settings > Logs surface for authenticated audit events, including failed uploads, downloads, installs, and sortable log views.

**Architecture:** Reuse the existing `audit_events` table, admin-only `/v1/home/audit-events` API, and lifecycle pruning. Add bounded server-side sorting, targeted audit writes in file/app handlers, and a dedicated dashboard settings page backed by the same API.

**Tech Stack:** Go `net/http`, existing store layer, embedded static UI assets, vanilla dashboard JavaScript, existing Go tests.

---

### Task 1: Store And API Sorting

**Files:**
- Modify: `internal/store/production_state.go`
- Modify: `internal/cloud/audit.go`
- Test: `internal/store/store_test.go`
- Test: `internal/cloud/production_validation_test.go`

- [x] Add tests showing `ListAuditEvents` accepts sort field/order and rejects unsupported values by falling back to newest first.
- [x] Add API tests for `sort=event_type&order=asc` on `/v1/home/audit-events`.
- [x] Update store listing to allow only `occurred_at`, `event_type`, `severity`, and `target_type`, with `asc` or `desc`.
- [x] Update `handleHomeAuditEvents` to pass query sort parameters.

### Task 2: Failed Action Audit Coverage

**Files:**
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/apps.go`
- Test: `internal/cloud/server_test.go`
- Test: `internal/cloud/apps_test.go`

- [x] Add failing tests for failed file upload setup and failed app package preview.
- [x] Record compact audit events with hashed paths and non-secret metadata.
- [x] Preserve best-effort audit behavior so user actions are not blocked by audit failures.

### Task 3: Logs Settings Tab

**Files:**
- Create: `internal/cloud/ui/settings-logs.html`
- Create: `internal/cloud/ui/settings-logs.js`
- Modify: `internal/cloud/ui.go`
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/ui/settings-nav.js`
- Test: `internal/cloud/server_test.go`

- [x] Add an admin-only `/dashboard/settings/logs` page.
- [x] Add `Logs` to settings navigation and admin-route redirects.
- [x] Render filters and sort controls for event type, severity, target type, sort field, sort order, and limit.
- [x] Fetch `/v1/home/audit-events` with those parameters and render redacted metadata.

### Task 4: Validation

**Files:**
- No new files.

- [ ] Run `gofmt -w ./cmd ./internal`.
- [ ] Run targeted Go tests for store/cloud audit, file transfer, apps, and UI static checks.
- [ ] Run `go test ./...` if the environment supports it.
- [ ] Run `go build ./...` if the environment supports it.
- [x] Run `git diff --check` as fallback static validation.
