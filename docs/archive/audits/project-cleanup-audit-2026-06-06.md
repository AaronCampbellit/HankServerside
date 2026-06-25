# Hank Remote Cleanup Audit - 2026-06-06

## Scope

This audit looked for holes, cleanup debt, and stale source-of-truth conflicts in the current single-home Hank Remote backend. It focused on the cloud/agent protocol surface, migrations, security posture, lifecycle cleanup, file operations, and operator/HankAI documentation.

This is not a full production readiness score. Use `docs/backend-production-repair-plan.md` for the broader hardening roadmap.

This file is an archived audit snapshot. The status table below is the current interpretation of the original findings; the evidence sections are retained for traceability, not as an active task list.

## Repair Status

Status as of 2026-06-06:

| Finding | Status | Repair summary |
| --- | --- | --- |
| Service-profile enum drift | Repaired | Added migration `000008_home_service_profiles_hermes.up.sql` and a migration contract test for supported service types. |
| Empty secret-encryption key permits plaintext storage | Repaired | Cloud config now requires `HANK_REMOTE_SECRET_ENCRYPTION_KEY` unless `HANK_REMOTE_ALLOW_PLAINTEXT_SECRETS=true` is explicitly set for local development; `/readyz` reports secret-storage status; `hank-remote-cloud secrets status --strict` and `hank-remote-cloud secrets reencrypt` cover known legacy plaintext rows. |
| `rollback_required` move jobs have no rollback path | Repaired | Added `files.move_rollback`, cloud rollback endpoint, dashboard rollback action, policy check, runbook coverage, and tests. |
| Lifecycle cleanup is partial | Repaired | Expanded pruning for operational rows, old transfer rows, login backoff, assistant attachment metadata, deleted note attachment rows, and safe stale note attachment files, with summary logging and configurable interval/retention env vars. |
| README route inventory is stale | Repaired | Updated README route and agent-command inventory, including quick links, audit/query telemetry, file jobs, assistant models/media image, attachment discard, and transfer status. |
| Security hardening note read as active risk | Repaired | Reworded the hardening note as historical rationale plus current-risk lines, with the active remaining risk focused on legacy plaintext-row detection/re-encryption. |
| Architecture docs understate current surfaces | Repaired | Updated `docs/architecture.md` with current cloud, agent, protocol, operator, assistant, media, file-job, and maintenance surfaces. |
| Historical audits need current-status framing | Repaired | Added a 2026-06-06 status table to `docs/backend-architecture-audit.md` and linked the legacy audit to this repaired cleanup snapshot. |
| Migration tests need constraint-contract assertions | Partially repaired | Added the service-profile constraint-contract assertion. Broader enum coverage for file-job, assistant, note, and collaboration states remains follow-up hardening. |

The evidence sections below are the original audit findings retained for traceability.

## High Priority

### 1. Service-profile enum sources are out of sync

Evidence:

- `internal/domain/models.go` defines `ServiceTypeHermes = "hermes"`.
- `internal/cloud/collaboration_handlers.go` accepts `PUT /v1/home/service-profiles/hermes` and routes it to the agent through `config.apply`.
- `internal/migrations/sql/000001_current_schema.up.sql` still adds `home_service_profiles_service_type_check` with only `('homeassistant', 'smb')`.
- `docs/backend-production-repair-plan.md` repeats the same two-value service-profile enum.
- The targeted Hermes cloud test is Postgres-backed and is skipped unless `HANK_REMOTE_TEST_DATABASE_URL` is set, so the local default test run does not prove the database constraint accepts the current API contract.

Risk:

Hermes service-profile saves can diverge between code, docs, tests, and a real migrated database. At best this is confusing schema drift; at worst dashboard saves fail only on deployed Postgres.

Cleanup:

- Add a migration that reconciles `home_service_profiles.service_type` with the actual supported service types.
- Add a focused migration/store test that inspects or exercises the Postgres check constraint for every supported service type.
- Update the repair-plan enum list when the migration lands.

### 2. Empty secret-encryption key still permits plaintext storage

Evidence:

- `internal/config/config.go` loads `HANK_REMOTE_SECRET_ENCRYPTION_KEY` but does not require it.
- `cmd/hank-remote-cloud/main.go` calls `ConfigureSecretEncryption`, but an empty key is accepted.
- `internal/store/secrets.go` returns no secret box for an empty key, and `encryptSecret` returns the original value when no box is configured.
- `docs/deployment.md` tells operators to keep the key stable, while `docs/security-hardening-todo.md` still calls stored secret encryption only partially implemented.

Risk:

Manual or misconfigured installs can persist OpenAI/ChatGPT OAuth tokens, APNs device tokens, and profile-vault JSON in plaintext even though the deployment docs imply the key is part of the normal production setup.

Cleanup:

- Make the key required for production-like cloud startup, or require an explicit development opt-out that is visible in `/readyz` and logs.
- Add tests proving new encrypted secret-bearing rows are not stored as plaintext when the key is configured.
- Add an operator path for detecting and re-encrypting existing plaintext values.

### 3. Cross-source file moves can require rollback, but rollback is not implemented

Evidence:

- `internal/agent/files/move.go` implements cross-source moves as copy, verify, then delete-source.
- Failures after verification map to `rollback_required`.
- `internal/cloud/file_jobs.go` stores and broadcasts `rollback_required`, and the UI can retry those jobs.
- There is no rollback endpoint or agent command that deletes copied destination files, restores source state, or marks a job `rolled_back`.
- `docs/runbooks/file-transfer-failures.md` covers transfer retry/failure handling, not managed file-job rollback.

Risk:

If delete-source fails after a verified copy, or the cloud/agent is interrupted at that point, operators can be left with duplicated data and no first-class cleanup action. Retrying the move may be unsafe or misleading because the destination already exists.

Cleanup:

- Add an explicit rollback command/path for `rollback_required` jobs.
- Track enough destination paths in the job metadata to make rollback idempotent.
- Extend the runbook and tests for delete-source failure, cancellation after verify, cloud restart during active move, retry, and rollback.

## Medium Priority

### 4. Scheduled lifecycle cleanup is still partial

Evidence:

- `internal/store/production_state.go` prunes expired sessions, agent tokens, invitations, app websocket tickets, rate-limit events, relay requests, and disconnected app/agent rows.
- It only marks expired `file_transfers`; it does not remove old completed/failed transfer rows.
- It does not prune old `audit_events`, `login_backoff` rows, assistant attachment metadata, assistant trace data, soft-deleted note attachment rows, or orphaned attachment files.
- `cmd/hank-remote-cloud/main.go` starts maintenance with a hard-coded 30-day retention.

Risk:

Long-running installs can accumulate operational rows and attachment files without an operator-visible retention policy. The repair plan also calls for retention summaries, but the current job has no summary/audit output.

Cleanup:

- Add configurable retention windows for audit events, transfer/job history, login backoff, assistant artifacts, and note attachments.
- Reconcile database attachment rows with the filesystem and purge safe orphan/deleted files.
- Emit cleanup counts to logs, metrics, or audit events so operators can tell maintenance is actually doing work.

### 5. README route inventory is stale

Evidence:

The root `README.md` route list omits current live routes used by the dashboard and code, including:

- `GET /v1/home/setup-status`
- `GET|POST|PUT|DELETE /v1/home/quick-links...`
- `GET /v1/home/audit-events`
- `GET /v1/home/query-telemetry`
- `GET /v1/home/file-jobs`, `GET /v1/home/file-jobs/{jobID}`, `POST /v1/home/file-jobs/{jobID}/cancel`, and `POST /v1/home/file-jobs/{jobID}/retry`
- `GET /v1/home/assistant/models`
- `GET /v1/home/assistant/media-image`
- `DELETE /v1/home/assistant/sessions/{sessionID}/attachments/{attachmentID}/discard`
- `GET /v1/file-transfers/{transferID}/status`

Risk:

The README is indexed as a HankAI project document and is likely to become stale answer material for route-level questions.

Cleanup:

- Replace the hand-maintained route list with either a generated route inventory or a smaller route-family summary that points to current handler groups.
- Add a docs check if the project keeps route inventories in markdown.

### 6. Security-hardening note is partly historical but still written as active risk

Evidence:

- `docs/security-hardening-todo.md` marks multiple sections implemented, but some "Current risk" paragraphs still describe pre-fix risks.
- The secret-encryption section correctly says partial implementation, but then still lists service-profile secrets as ordinary database values even though current cloud code relays those secrets to the agent instead of persisting them.

Risk:

HankAI and future agents may reopen resolved work or miss the narrower current issue: empty encryption key fallback and migration/re-encryption.

Cleanup:

- Convert the file into a current hardening status matrix or archive it with a top status note.
- Keep active risks separate from fixed historical rationale.

### 7. Architecture docs understate current assistant, media, and operator surfaces

Evidence:

- `docs/architecture.md` still reads like a compact early architecture overview.
- Current code includes assistant model settings, media workflow settings, quick links, audit events, query telemetry, file jobs, backup/restore-test UI, realtime events, and note attachments.

Risk:

The architecture doc does not help a new maintainer understand the current operational system, and HankAI may retrieve it ahead of more specific docs.

Cleanup:

- Add a short current-system map that points to the main handler groups and operator surfaces.
- Keep long feature details in feature docs/runbooks, but make `architecture.md` reflect the live shape.

## Lower Priority Cleanup

### 8. Historical audit docs need stronger current-status framing

Evidence:

- `docs/backend-architecture-audit.md` and `docs/legacy-code-audit.md` contain useful point-in-time evidence, but both include many now-fixed findings.
- `docs/project-knowledge-index.md` warns about this, but the bodies can still be retrieved by HankAI.

Risk:

Automated answers may cite old unresolved-sounding findings unless the status note is very prominent and machine-readable.

Cleanup:

- Add concise "Current Status" tables near the top of historical audits.
- Prefer linking to current runbooks and the repair plan for operator instructions.

### 9. Migration tests should include constraint-contract assertions

Evidence:

- The project now has versioned migrations and checksum checks.
- The service-profile enum drift shows that checksum/status validation alone does not prove enum constraints match current domain constants.

Risk:

Future enum additions can land in Go code and UI without a matching migration.

Cleanup:

- Add a small migration contract test for check-constrained enum tables.
- Cover service profiles, file job statuses, assistant attachment statuses, assistant run states, note page types, and collaboration roles.

## Already Resolved Items Observed

These older cleanup themes appear resolved in current code and should not be reopened without new evidence:

- Agent websocket auth uses bearer token plus `X-Hank-Agent-ID`; query-token support is not present in the current route.
- File transfers are bearer-token based and expose a status endpoint.
- Runtime state for cloud, app websocket tickets, relay requests, app/agent connections, audit events, file transfers, and file jobs is durable.
- Legacy note body storage and older SMB fallback environment paths have been moved into the migration/cleanup path.
- Dashboard JavaScript is using shared API helpers rather than many separate ad hoc auth paths.

## Validation Notes

Commands run during the original audit:

- `go test ./internal/cloud -run TestHermesServiceProfileApplyRoutesToAgent -count=1 -v`
- `go test ./internal/store -run TestMigration -count=1`
- `go test ./...`

Commands run after the repair pass:

- `make fmt`
- `go test ./internal/config ./internal/migrations ./internal/agent/files ./internal/cloud ./internal/store ./internal/repo_checks -run 'TestLoadCloudDefaults|TestLoadCloudRejectsInvalidMaintenanceRetention|TestLoadCloudRequiresSecretEncryptionKeyUnlessPlaintextOptOut|TestHomeServiceProfileConstraintIncludesSupportedServiceTypes|TestRollbackMoveDestinationDeletesCopiedDestination|TestFileServerUIOffersManagedJobRollback|TestRollbackRequiredFileOperationJobCanBeRolledBack|TestPruneLifecycleRemovesExpiredOperationalRows|TestReencryptPlaintextSecrets|TestActiveDocsDoNotReferenceRemovedLegacyPaths' -count=1 -v`
- `go test ./...`
- `make build`
- `git diff --check`

The focused rollback and lifecycle tests that require PostgreSQL compiled but skipped because `HANK_REMOTE_TEST_DATABASE_URL` is not set in this shell. `make migrate-status` now requires `HANK_REMOTE_SECRET_ENCRYPTION_KEY`; with a temporary local-dev key it reached database connection and failed because the local Compose hostname `postgres` is not resolvable. `scripts/schema-drift-check.sh` with temporary local validation values also reached database connection and failed because PostgreSQL is not listening on `127.0.0.1:5432` in this shell.
