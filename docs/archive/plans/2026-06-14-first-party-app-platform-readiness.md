# First-Party App Platform Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make installed first-party `.hankapp` packages usable from Hank chat without HankServerside rebuilds, with app-level member access controls.

**Architecture:** Store app access mode with installed app metadata, expose it through the app API, and enforce it in both HankAI slash routing and direct `apps.invoke` relay. HankAI should resolve installed package slash commands before built-in tools and invoke them through a generic request/response contract, while existing app-specific rich paths remain as compatibility where needed.

**Tech Stack:** Go, PostgreSQL migrations, stdlib JSON, existing Hank cloud/agent protocol, existing dashboard JavaScript.

---

### Task 1: Add App-Level Access Metadata

**Files:**
- Modify: `internal/protocol/apps.go`
- Modify: `internal/domain/models.go`
- Modify: `internal/store/apps.go`
- Modify: `internal/store/apps_test.go`
- Create: `internal/migrations/sql/000015_agent_app_user_access.up.sql`

- [x] **Step 1: Write failing store/protocol tests**

Add assertions that `domain.HomeAgentApp.UserAccess` round-trips through `UpsertHomeApp`, `GetHomeApp`, and `ListHomeApps`, and that the default access value is `admins_only` when omitted.

- [x] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/store -run TestAppMetadata -count=1`

Expected: FAIL because `UserAccess` does not exist.

- [x] **Step 3: Add metadata field and migration**

Add constants for `admins_only` and `home_members`, add `UserAccess` to protocol/domain structs, extend store columns/scans/upsert, and add migration `000015_agent_app_user_access.up.sql`.

- [x] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/store -run TestAppMetadata -count=1`

Expected: PASS.

### Task 2: Expose And Update App Access Through Cloud API

**Files:**
- Modify: `internal/cloud/apps.go`
- Modify: `internal/cloud/apps_test.go`
- Modify: `internal/cloud/ui/apps.js`

- [x] **Step 1: Write failing cloud API tests**

Add tests that new app activation persists `admins_only`, admin config update can set `user_access`, and member requests only receive slash commands for enabled `home_members` apps.

- [x] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/cloud -run 'TestApps' -count=1`

Expected: FAIL because user access is not parsed, persisted, updated, or filtered.

- [x] **Step 3: Implement API support**

Update `persistAgentApp`, `persistedAppSummaries`, and app config handling to preserve or update `user_access`. Add access filtering for app list responses based on current membership while preserving admin visibility.

- [x] **Step 4: Update Settings UI**

Add an app-level access select/toggle to `apps.js` config rendering and include it in config payloads.

- [x] **Step 5: Run focused tests and verify GREEN**

Run: `go test ./internal/cloud -run 'TestApps' -count=1`

Expected: PASS.

### Task 3: Enforce App Access For Direct WebSocket Invocation

**Files:**
- Modify: `internal/cloud/server.go`
- Modify: `internal/cloud/permissions.go`
- Modify: `internal/cloud/production_validation_test.go` or `internal/cloud/apps_test.go`

- [x] **Step 1: Write failing relay authorization tests**

Add tests proving a member cannot send direct `apps.invoke` for an `admins_only` app, can send it for a `home_members` app, and admins can send it either way.

- [x] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/cloud -run 'AppsInvoke|AppAccess|Production' -count=1`

Expected: FAIL because direct `apps.invoke` is not access-checked.

- [x] **Step 3: Implement shared authorization**

Decode `protocol.AppsInvokeRequest` from `RoutedCommand.Body`, load persisted app metadata, require enabled app, enforce `user_access`, and reject unauthorized invocations before relay.

- [x] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/cloud -run 'AppsInvoke|AppAccess|Production' -count=1`

Expected: PASS.

### Task 4: Add Generic HankAI Installed-App Slash Routing

**Files:**
- Modify: `internal/cloud/assistant.go`
- Modify: `internal/cloud/assistant_tools.go`
- Modify: `internal/cloud/assistant_workflow_test.go`

- [x] **Step 1: Write failing assistant tests**

Add tests proving an installed `home_members` app slash command routes to `apps.invoke` for a member, an `admins_only` app is hidden/rejected for a member, an admin can invoke an `admins_only` app, and generic output text renders.

- [x] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/cloud -run 'TestAssistant.*InstalledApp|TestAssistant.*GenericApp' -count=1`

Expected: FAIL because backend resolver does not dynamically invoke installed app slash commands.

- [x] **Step 3: Implement generic app resolver/executor**

Before built-in tool execution, resolve explicit slash prompts against persisted installed apps available to the current user. Build an `apps.invoke` payload with `raw_text`, `slash_command`, and safe context. Render generic output text/cards and attach diagnostics.

- [x] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/cloud -run 'TestAssistant.*InstalledApp|TestAssistant.*GenericApp' -count=1`

Expected: PASS.

### Task 5: Harden Manifest Validation For Generic App Contracts

**Files:**
- Modify: `internal/agent/apps/manifest.go`
- Modify: `internal/agent/apps/manifest_test.go`

- [x] **Step 1: Write failing manifest tests**

Add tests for reserved built-in slash command rejection and invalid settings defaults/options.

- [x] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/agent/apps -run TestValidateManifest -count=1`

Expected: FAIL for missing validation.

- [x] **Step 3: Implement validation**

Reject package slash command collisions with built-in Hank commands and validate settings defaults/options match supported field types where feasible.

- [x] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/agent/apps -run TestValidateManifest -count=1`

Expected: PASS.

### Task 6: Documentation And Full Verification

**Files:**
- Modify: `docs/deployment.md`
- Modify: `packages/hermes/README.md`
- Modify: `packages/gramaton/README.md`
- Modify: `packages/ydownload/README.md`

- [x] **Step 1: Update docs**

Document app-level access, first-party trust assumptions, generic slash command behavior, and when HankServerside changes are still needed.

- [x] **Step 2: Format**

Run: `gofmt -w ./cmd ./internal`

Expected: no output.

- [x] **Step 3: Build**

Run: `go build ./...`

Expected: exit 0.

- [x] **Step 4: Test**

Run: `go test ./...`

Expected: exit 0.

- [x] **Step 5: Migration checks**

Run if database environment is configured: `make migrate-status` and `make schema-drift-check`.

Expected: exit 0, or report skipped with the missing environment.

## Self-Review

- Spec coverage: app-level access, generic slash routing, direct invocation authorization, package validation, UI, docs, and verification are covered.
- No placeholders remain.
- Type names are consistent with existing `AppSummary`, `HomeAgentApp`, and `AppsInvokeRequest` contracts.
- Scope stays within first-party installable app readiness and does not add a third-party marketplace or sandbox.
