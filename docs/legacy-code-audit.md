# Legacy Code And Path Audit

Date: 2026-06-01

Scope: full repo scan for legacy code, compatibility paths, stale docs, and old product assumptions that should be removed or updated as Hank Remote moves toward the single-home cloud-and-agent production target.

## Resolution Status

Status as of 2026-06-01: resolved in one cleanup pass. The findings below are retained as historical evidence, not as open work.
For the next cleanup snapshot and the 2026-06-06 repair status, see [project-cleanup-audit-2026-06-06.md](project-cleanup-audit-2026-06-06.md).

- Schema startup mutation was replaced by embedded versioned migrations under `internal/migrations/sql`; normal read-only startup now checks migration status and `migrate up` applies pending migrations explicitly.
- Legacy cleanup migrations cover current baseline creation, `board` to `kanban`, `user_notes.body_markdown` canonicalization, `home_notes` archival/drop, browser-redirect OpenAI OAuth state removal, and pgvector column setup when the extension exists.
- `user_notes.body_markdown` is the only stored note body. API/UI compatibility keeps `content` as a response alias mapped from `body_markdown`.
- `home_notes` live storage and `UpsertHomeNote` were removed; existing rows are copied to `legacy_home_notes_archive` before the live table is dropped.
- Runtime note schema repair paths were removed from store writes and replaced with migration/status test coverage.
- Agent SMB config now uses `HANK_REMOTE_SMB_SHARES_JSON` only. `scripts/migrate-agent-smb-env.sh` converts old single-share `.env.agent` files before upgrade.
- Dashboard-generated `.env.agent` contains only required identity/config/root values. Home Assistant, SMB, and media credentials are set through Settings after the agent is online.
- Public legacy dashboard routes were removed. Current pane routes are `/dashboard/settings/people-pane`, `/dashboard/settings/connections-pane`, `/dashboard/settings/ai-pane`, `/dashboard/settings/backups-pane`, and `/dashboard/settings/join-home-pane`.
- Dashboard scripts use the shared `/assets/api-client.js` helper for credentials, CSRF, JSON headers, and error handling.
- Browser-redirect OpenAI OAuth was removed. OpenAI API-key provider support remains, and ChatGPT/Codex linking uses device code through the existing `/v1/oauth/openai/start` and `/v1/oauth/openai/status` contract.
- pgvector is the production vector mode. JSON embedding fallback remains only for local/development resilience when pgvector is unavailable.
- Phase-era docs are archived under `docs/archive/phases`; current guidance points to deployment docs, runbooks, the repair plan, and the single-home scope.

## High Priority Cleanup

### 1. Inline startup schema mutation still exists beside versioned migration commands

Evidence:

- `internal/store/store.go` still builds and runs a large `CREATE TABLE IF NOT EXISTS` / `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` statement list in `Store.migrate`.
- `cmd/hank-remote-cloud migrate up|status|baseline`, `make migrate-status`, and `scripts/schema-drift-check.sh` exist, so schema work has a formal path now.
- The repair plan requires normal cloud startup to perform read-only drift/status checks, not hidden mutations.

Why it is legacy:

Startup DDL was useful before migration discipline existed. It now conflicts with the production rule that schema changes must be explicit, versioned, checksum-checked, and drift-checked.

Recommended action:

- Move every remaining startup schema change into embedded versioned migrations.
- Keep startup to migration status/drift validation only.
- Remove opportunistic repair DDL from normal read/write store paths.
- Keep a documented `baseline` path for existing deployments.

Primary files:

- `internal/store/store.go`
- `internal/migrations/migrations.go`
- `cmd/hank-remote-cloud/main.go`
- `scripts/schema-drift-check.sh`

### 2. `home_notes` is legacy storage beside `user_notes`

Evidence:

- `home_notes` is still created in `internal/store/store.go`.
- `UpsertHomeNote` still writes to `home_notes` in `internal/store/collaboration.go`.
- Current home note list/read behavior uses `user_notes` plus `note_shares`.
- The architecture audit and repair plan both call out `home_notes` as legacy duplicate storage.

Why it is legacy:

The product has moved to user-owned profile/home notes with sharing and collaboration. Keeping `home_notes` creates duplicate storage, unclear sync semantics, and migration complexity.

Recommended action:

- Confirm whether any live agent protocol path still calls `UpsertHomeNote`.
- Add a one-time migration/archive plan for existing `home_notes` rows.
- Stop writing `home_notes`.
- Remove `home_notes` table creation, indexes, domain types, and store methods after migration.

Primary files:

- `internal/store/store.go`
- `internal/store/collaboration.go`
- `internal/domain/notes.go`
- `docs/backend-production-repair-plan.md`

### 3. `user_notes.content` and `user_notes.body_markdown` duplicate note body data

Evidence:

- `user_notes` stores both `content` and `body_markdown`.
- `UpsertUserNote` writes `noteBodyMarkdown(note)` into both columns.
- The dashboard reads `note.body_markdown || note.content`, preserving fallback behavior.

Why it is legacy:

`content` is now a compatibility copy. The current canonical note body is Markdown-oriented, and writing two columns invites divergence.

Recommended action:

- Make `body_markdown` the single canonical note body.
- Backfill `body_markdown` where needed.
- Update store reads/writes, API payloads, UI fallback logic, attachment note upserts, and tests.
- Drop or archive `content` only through an explicit migration.

Primary files:

- `internal/store/user_notes.go`
- `internal/store/attachments.go`
- `internal/store/store.go`
- `internal/cloud/ui/profile-notes.js`

### 4. Store methods still repair old `board` page-type constraints at runtime

Evidence:

- `shouldRepairUserNotesPageTypeConstraint` and `repairUserNotesPageTypeConstraint` update `board` to `kanban` and alter constraints during a note save.
- Tests intentionally recreate the legacy constraint to prove runtime repair.

Why it is legacy:

This is a hidden schema repair in a normal write path. It should be replaced by a migration/status check now that migrations exist.

Recommended action:

- Move the `board` to `kanban` repair into a versioned migration.
- Remove runtime DDL from `UpsertUserNote` and `SaveUserNoteWithOperations`.
- Replace the runtime-repair test with migration/status coverage.

Primary files:

- `internal/store/user_notes.go`
- `internal/store/user_notes_test.go`
- `internal/store/store.go`

### 5. Legacy single-share SMB env contract remains active

Evidence:

- `LoadAgent` still reads `HANK_REMOTE_SMB_HOST`, `HANK_REMOTE_SMB_SHARE`, `HANK_REMOTE_SMB_USERNAME`, `HANK_REMOTE_SMB_PASSWORD`, and `HANK_REMOTE_SMB_DOMAIN`.
- `loadSMBShares` prepends that legacy single-share config to `HANK_REMOTE_SMB_SHARES_JSON`.
- `internal/agent/files/service.go` has `appendLegacySMBConfig`.
- Dashboard-generated `.env.agent` and deployment docs still include both single-share env keys and `HANK_REMOTE_SMB_SHARES_JSON`.

Why it is legacy:

`source_id` plus `HANK_REMOTE_SMB_SHARES_JSON` is the durable multi-share contract. The single-share env fields are fallback-only compatibility.

Recommended action:

- Decide a deprecation window for existing deployments.
- Keep dashboard Settings as the preferred configuration path.
- Stop generating empty single-share env fields in new setup files once migration is safe.
- Eventually remove `cfg.SMB`, `appendLegacySMBConfig`, single-share docs, and related tests.

Primary files:

- `internal/config/config.go`
- `internal/config/config_test.go`
- `cmd/hank-remote-agent/main.go`
- `internal/agent/files/service.go`
- `internal/cloud/ui/dashboard.js`
- `docs/deployment.md`

## Medium Priority Cleanup

### 6. Legacy dashboard page redirects preserve old routes

Evidence:

- `/dashboard/home-users`, `/dashboard/service-profiles`, `/dashboard/sync-status`, `/dashboard/storage`, `/dashboard/assistant-settings`, and `/dashboard/accept-invitation` are still routed as standalone pages or redirects into the consolidated Settings/Home structure.
- Tests explicitly call these "legacy" redirects.

Why it is legacy:

The dashboard has moved toward a consolidated Home/Tools/Settings structure. Old standalone URLs remain compatibility paths and inflate route/UI test surface.

Recommended action:

- Keep pane-only routes required by Settings iframes.
- Remove public standalone redirects after bookmarked/operator links are updated.
- Update tests and docs to use canonical `/dashboard`, `/dashboard/settings#people`, `/dashboard/settings#connections`, `/dashboard/settings#ai`, and `/dashboard/settings#backups` paths.

Primary files:

- `internal/cloud/ui.go`
- `internal/cloud/server_test.go`
- `internal/cloud/ui/settings.html`
- `internal/cloud/ui/admin-nav.js`

### 7. Phase-era docs are stale as operator guidance

Evidence:

- `docs/roadmap.md` still points to Phase 1-6 docs.
- `docs/project-knowledge-index.md` indexes the old phase/task docs for HankAI.
- Phase tasklist files contain implementation prompts for already-built work.

Why it is legacy:

The repo now has `docs/backend-production-repair-plan.md`, runbooks, deployment docs, and operator setup docs. Phase docs are useful historical context, but they should not compete with current setup and repair guidance.

Recommended action:

- Mark phase docs as historical or move them under an archive docs section.
- Update `docs/project-knowledge-index.md` so HankAI weights current operator/runbook/repair docs ahead of phase docs.
- Remove implementation-prompt sections if they cause confusion.

Primary files:

- `docs/roadmap.md`
- `docs/project-knowledge-index.md`
- `docs/phase-*-*.md`

### 8. `docs/backend-architecture-audit.md` contains resolved findings as if current

Evidence:

- It still says file transfer tokens are returned in URL query strings.
- It still says the agent WebSocket accepts query-token fallback.
- Current code and docs indicate bearer transfer tokens and header-only agent auth.
- It also warns that `/v1/home` singleton routes will not scale to SaaS, which conflicts with the corrected single-home product scope if read without context.

Why it is legacy:

The file is a dated audit artifact, but it is indexed as project knowledge and may mislead future work.

Recommended action:

- Add a top-of-file status note that it is a point-in-time audit.
- Mark resolved findings with dates or link to the repair plan.
- Reword SaaS/multi-home warnings as "out of scope unless product scope changes."

Primary files:

- `docs/backend-architecture-audit.md`
- `docs/project-knowledge-index.md`

### 9. Multiple dashboard-local API helpers remain instead of one shared browser client

Evidence:

- Many UI files implement local `api()` wrappers.
- `admin-nav.js` has separate `apiJSON()`/fetch behavior.
- Memory from prior dashboard cleanup identifies this as recurring tax for CSRF/header changes.

Why it is legacy:

This is not old product behavior, but it is legacy frontend structure. Cross-cutting auth/CSRF/error handling requires repeated edits across page scripts.

Recommended action:

- Add one shared embedded browser API helper asset.
- Move CSRF/header/error handling into it.
- Migrate page scripts incrementally, starting with Settings, Dashboard, File Server, and Hank.

Primary files:

- `internal/cloud/ui/*.js`
- `internal/cloud/ui.go`

## Low Priority Or Needs Decision

### 10. Assistant embedding JSON fallback duplicates pgvector columns

Evidence:

- `assistant_chunks` and `assistant_file_index` keep `embedding_json`.
- When pgvector is available, `embedding VECTOR(768)` is also added.
- UI exposes `json_fallback` vector mode.

Why it may be legacy:

If pgvector is mandatory for production, JSON embeddings are compatibility storage. If pgvector remains optional, this is intentional fallback behavior.

Recommended action:

- Decide whether pgvector is required for production.
- If required, migrate and remove JSON fallback after proof.
- If optional, document retention and cleanup policy for duplicate embeddings.

Primary files:

- `internal/store/store.go`
- `internal/store/assistant_index.go`
- `internal/cloud/assistant.go`
- `internal/cloud/ui/assistant-settings.js`

### 11. OpenAI browser redirect OAuth remains alongside ChatGPT/Codex device-code flow

Evidence:

- `openai_oauth.go` implements browser redirect OAuth.
- `chatgpt_oauth.go` implements the experimental device-code ChatGPT/Codex flow.
- Deployment docs describe both OpenAI API-key and ChatGPT/Codex OAuth-style flows.

Why it may be legacy:

This depends on product direction. If ChatGPT/Codex device-code becomes the only subscription-style flow and OpenAI API key remains the supported production provider, browser redirect OAuth may be unnecessary.

Recommended action:

- Decide whether browser redirect OpenAI account linking is still a supported operator/user feature.
- If not, remove routes/UI/settings and migrate stored `openai_accounts` records safely.
- If yes, document the distinction more clearly and keep both.

Primary files:

- `internal/cloud/openai_oauth.go`
- `internal/cloud/chatgpt_oauth.go`
- `internal/store/openai_oauth.go`
- `internal/cloud/ui/assistant-settings.js`
- `docs/deployment.md`

### 12. Setup file generation still includes blank optional connection secrets

Evidence:

- Dashboard-generated `.env.agent` includes blank Home Assistant and SMB fields.
- Docs tell users to leave Home Assistant and SMB blank and fill them later from Settings.

Why it may be legacy:

Now that Settings persists service profiles into `.env.agent`, new setup files could be smaller and less secret-field oriented.

Recommended action:

- Keep required agent identity fields in setup output.
- Consider moving optional local service credentials entirely into dashboard Settings after first boot.
- Avoid showing blank legacy SMB single-share keys once multi-share JSON/settings is the primary path.

Primary files:

- `internal/cloud/ui/dashboard.js`
- `docs/deployment.md`
- `scripts/bootstrap-first-run.sh`

## Not Legacy For Current Scope

- `/v1/home` and `/v1/home/...` are current canonical app-facing routes for the single-home deployment model. Do not replace them with user-facing `/v1/homes/{home_id}` unless product scope changes.
- `source_id` is current and should be preserved for file routing and transfers.
- Bearer `transfer_token` on `/v1/file-transfers/{id}` is current and replaced the old query-token pattern.
- Local SMB usage inside the home agent is current. What should remain removed is raw public SMB exposure or app-side remote SMB networking.
- `HANK_REMOTE_SMB_SHARES_JSON` is current. The legacy part is the old single-share env fallback.
