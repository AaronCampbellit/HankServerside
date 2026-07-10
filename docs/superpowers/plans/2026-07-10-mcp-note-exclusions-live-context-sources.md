# MCP Note Exclusions and Live Context Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add MCP-only note/notebook exclusions and GUI-managed, live, read-only project context sources routed through the home agent.

**Architecture:** Persist a boolean exclusion marker with notes and filter it only at the remote MCP boundary, including one-level notebook inheritance. Persist per-user context-source grants in PostgreSQL, manage them through authenticated profile routes and the React AI settings page, and service MCP project tools through dedicated bounded agent commands against an existing File Server source.

**Tech Stack:** Go 1.26, PostgreSQL migrations, JSON-RPC MCP over HTTP, Hank cloud/agent WebSocket protocol, React 19, TypeScript, Vitest.

## Global Constraints

- The note lock is an MCP visibility marker, not encryption or user authentication.
- Normal Hank Notes APIs and HankAI continue to see excluded records.
- Excluded notebooks hide every current child from MCP; moving an unlocked child out restores MCP visibility.
- Context sources are read-only and reference existing File Server sources without storing SMB credentials.
- Agent file access is contained beneath the configured share-relative root and denies secrets, hidden/build/vendor trees, binaries, oversized files, and symlink escapes.
- Context reads are live; no persistent project snapshot or index is introduced.
- Existing MCP OAuth grants use `docs:read` for the new tools.
- Schema changes use forward migration `000019` and the existing migration/status/drift path.

---

### Task 1: Persist MCP note exclusion and enforce effective visibility

**Files:**
- Create: `internal/migrations/sql/000019_mcp_note_exclusions_and_context_sources.up.sql`
- Modify: `internal/domain/notes.go`
- Modify: `internal/protocol/notes.go`
- Modify: `internal/store/user_notes.go`
- Modify: `internal/cloud/notes_store.go`
- Modify: `internal/cloud/mcp_server.go`
- Test: `internal/cloud/mcp_unit_test.go`
- Test: `internal/cloud/notes_api_test.go`
- Test: `internal/store/user_notes_test.go`

**Interfaces:**
- Add `MCPExcluded bool` with JSON name `mcp_excluded` to `domain.UserNote`, `protocol.NoteSummary`, `protocol.NotesFetchResponse`, and `protocol.NotesSaveRequest`.
- Add `func mcpVisibleProfileNotes(notes []domain.UserNote) []domain.UserNote` and `func mcpNoteVisible(notes []domain.UserNote, noteID string) bool`.
- Preserve omitted/false as backward-compatible `false`.

- [ ] **Step 1: Write failing tests** proving persistence/API round-trip, direct exclusion, notebook-child inheritance, move-out restoration, and `not found` for excluded MCP fetch/update/append/delete.
- [ ] **Step 2: Run red tests:** `go test ./internal/store ./internal/cloud -run 'Test.*MCPExcluded|TestMCP.*Excluded'`; expect missing field/column/filter failures.
- [ ] **Step 3: Add migration and model plumbing.** Migration SQL must include `ALTER TABLE user_notes ADD COLUMN IF NOT EXISTS mcp_excluded BOOLEAN NOT NULL DEFAULT FALSE;` and Task 3's context-source table. Update every user-note SELECT, scan, INSERT, and UPSERT column list.
- [ ] **Step 4: Add MCP-only filtering.** `list_notes`, `search_notes`, and `list_note_tags` operate on a filtered profile-note slice; ID-based tools verify effective visibility before reading or mutating. Return `store.ErrNotFound` for excluded IDs.
- [ ] **Step 5: Run green tests:** `go test ./internal/store ./internal/cloud -run 'Test.*MCPExcluded|TestMCP.*Excluded'`; expect PASS.
- [ ] **Step 6: Commit:** `git add internal/migrations/sql/000019_mcp_note_exclusions_and_context_sources.up.sql internal/domain/notes.go internal/protocol/notes.go internal/store/user_notes.go internal/cloud/notes_store.go internal/cloud/mcp_server.go internal/cloud/mcp_unit_test.go internal/cloud/notes_api_test.go internal/store/user_notes_test.go && git commit -m "feat: exclude locked notes from MCP"`.

### Task 2: Add the Notes lock control to the React dashboard

**Files:**
- Modify: `web/dashboard/src/api/profileNotes.ts`
- Modify: `web/dashboard/src/api/profileNotes.test.ts`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Add `mcp_excluded?: boolean` to `ProfileNoteSummary`, `ProfileNote`, and `SaveProfileNoteInput`.
- Add `mcpExcluded: boolean` to editor state and include it in dirty-state comparison/save payloads.
- Toolbar labels are exactly `Exclude from MCP` and `Include in MCP`.

- [ ] **Step 1: Write failing frontend tests** that click the lock, assert the PUT payload contains `mcp_excluded: true`, verify a second click sends false, and show inherited exclusion copy for a child whose notebook is excluded.
- [ ] **Step 2: Run red tests:** `npm --prefix web/dashboard test -- --run src/api/profileNotes.test.ts src/dashboard/ProfileNotesPage.test.tsx`; expect missing control/payload assertions.
- [ ] **Step 3: Implement API/editor state and toolbar control.** Use the existing save pipeline so lock changes serialize with the latest revision and cannot overwrite pending edits. Display a lock icon on excluded notebook/note rows and `Excluded because its notebook is locked` for inherited state.
- [ ] **Step 4: Add focused styles** for locked/inherited states while preserving existing toolbar responsive behavior.
- [ ] **Step 5: Run green tests:** same Vitest command; expect PASS.
- [ ] **Step 6: Commit:** `git add web/dashboard/src/api/profileNotes.ts web/dashboard/src/api/profileNotes.test.ts web/dashboard/src/dashboard/ProfileNotesPage.tsx web/dashboard/src/dashboard/ProfileNotesPage.test.tsx web/dashboard/src/styles.css && git commit -m "feat: add MCP lock controls to Notes"`.

### Task 3: Persist and manage per-user MCP context sources

**Files:**
- Modify: `internal/migrations/sql/000019_mcp_note_exclusions_and_context_sources.up.sql`
- Create: `internal/domain/mcp_context.go`
- Create: `internal/store/mcp_context.go`
- Create: `internal/store/mcp_context_test.go`
- Create: `internal/cloud/mcp_context.go`
- Modify: `internal/cloud/server.go`
- Test: `internal/cloud/server_test.go`

**Interfaces:**
- Define `domain.MCPContextSource` with `ID`, `OwnerUserID`, `HomeID`, `Name`, `FileSourceID`, `RootPath`, `Enabled`, `LastTestedAt`, `LastTestError`, `CreatedAt`, and `UpdatedAt`.
- Store methods: `CreateMCPContextSource`, `ListMCPContextSourcesByUser`, `GetMCPContextSourceForUser`, `UpdateMCPContextSourceForUser`, and `DeleteMCPContextSourceForUser`.
- Routes: `GET/POST /v1/me/mcp/context-sources`, `PUT/DELETE /v1/me/mcp/context-sources/{id}`, and `POST /v1/me/mcp/context-sources/{id}/test`.

- [ ] **Step 1: Write failing store and HTTP tests** for CRUD, unique per-user names, cross-user 404s, CSRF-protected writes, disabled sources, singleton-home association, and audit events.
- [ ] **Step 2: Run red tests:** `go test ./internal/store ./internal/cloud -run 'TestMCPContextSource'`; expect missing types/routes.
- [ ] **Step 3: Add the table to migration 000019** with foreign keys to `users` and `homes`, a unique `(owner_user_id, name)` constraint, and owner/enabled indexes.
- [ ] **Step 4: Implement domain/store/handlers.** Normalize names and share-relative roots, reject empty or absolute/traversing roots, resolve the user's singleton home, enforce ownership on every record, and use existing audit/CSRF middleware patterns.
- [ ] **Step 5: Run green tests:** same Go test command; expect PASS.
- [ ] **Step 6: Commit:** `git add internal/migrations/sql/000019_mcp_note_exclusions_and_context_sources.up.sql internal/domain/mcp_context.go internal/store/mcp_context.go internal/store/mcp_context_test.go internal/cloud/mcp_context.go internal/cloud/server.go internal/cloud/server_test.go && git commit -m "feat: manage MCP context sources"`.

### Task 4: Implement bounded live context commands and MCP tools

**Files:**
- Create: `internal/protocol/mcp_context.go`
- Create: `internal/agent/mcpcontext/service.go`
- Create: `internal/agent/mcpcontext/service_test.go`
- Modify: `internal/agent/commands.go`
- Modify: `internal/agent/client.go`
- Modify: `internal/cloud/mcp_context.go`
- Modify: `internal/cloud/mcp_tools.go`
- Modify: `internal/cloud/mcp_server.go`
- Test: `internal/cloud/mcp_unit_test.go`

**Interfaces:**
- Protocol commands: `mcp.context.list`, `mcp.context.search`, `mcp.context.read`, `mcp.context.test`.
- Requests carry `source_id`, `root_path`, and relative `path` or `query`; responses carry source-relative entries/content plus `truncated` where applicable.
- MCP tools: `list_context_sources`, `list_context_files`, `search_context`, `read_context_file`, all requiring `docs:read`.

- [ ] **Step 1: Write failing agent tests** for contained list/read/search, `.env` and hidden/vendor/build exclusions, unsupported/binary/oversized rejection, `..` and absolute traversal rejection, symlink escape rejection, and bounded truncation.
- [ ] **Step 2: Write failing cloud tests** for tool schemas, per-user source lookup, disabled/other-user 404s, correct home routing, agent-offline error, and decoded tool output.
- [ ] **Step 3: Run red tests:** `go test ./internal/agent/... ./internal/cloud -run 'TestMCPContext'`; expect missing command/service/tool failures.
- [ ] **Step 4: Implement the dedicated agent service** over the existing File Service abstraction. Fixed ceilings: 400 KB read, 50 returned matches, 10,000 visited files, 20 MB inspected text, and cloud command context timeout of 20 seconds. Return source-relative paths only.
- [ ] **Step 5: Wire dispatcher capabilities and cloud tools.** Test updates `LastTestedAt/LastTestError`; MCP calls audit source/operation without query results or file contents.
- [ ] **Step 6: Run green tests:** same Go command; expect PASS.
- [ ] **Step 7: Commit:** `git add internal/protocol/mcp_context.go internal/agent/mcpcontext internal/agent/commands.go internal/agent/client.go internal/cloud/mcp_context.go internal/cloud/mcp_tools.go internal/cloud/mcp_server.go internal/cloud/mcp_unit_test.go && git commit -m "feat: read live project context through MCP"`.

### Task 5: Build the MCP Context Sources GUI and finish documentation

**Files:**
- Modify: `web/dashboard/src/api/assistant.ts`
- Modify: `web/dashboard/src/api/assistant.test.ts`
- Modify: `web/dashboard/src/settings/AssistantSettings.tsx`
- Modify: `web/dashboard/src/settings/AssistantSettings.test.tsx`
- Modify: `web/dashboard/src/styles.css`
- Modify: `docs/mcp.md`
- Modify: `docs/notes-api.md`
- Modify: `README.md`

**Interfaces:**
- `AssistantClient` methods: `listMCPContextSources`, `createMCPContextSource`, `updateMCPContextSource`, `testMCPContextSource`, and `deleteMCPContextSource`.
- GUI supports list/add/edit/test/toggle/remove and identifies saved state separately from live test state.

- [ ] **Step 1: Write failing API and component tests** for normalized source lists, add/edit/test/toggle/remove requests, no-share state, offline/test failure, duplicate-name error, enabled state, and successful test timestamp.
- [ ] **Step 2: Run red tests:** `npm --prefix web/dashboard test -- --run src/api/assistant.test.ts src/settings/AssistantSettings.test.tsx`; expect missing API/UI behavior.
- [ ] **Step 3: Implement the GUI** in the existing MCP Connector panel using the File Server share options already returned by assistant/home settings. Disable mutation controls for view-only users. Use accessible dialogs/labels and plain-language read-only/live-agent copy.
- [ ] **Step 4: Update docs** with note-lock semantics, live-agent availability, GUI setup, context tools, allowed/blocked content, and privacy implications.
- [ ] **Step 5: Run frontend verification:** targeted Vitest command and `npm --prefix web/dashboard run build`; expect PASS.
- [ ] **Step 6: Run repository verification:** `gofmt -w ./cmd ./internal && go build ./... && go test ./... && make migrate-status && make schema-drift-check && git diff --check`; all must pass or be reported with exact blockers.
- [ ] **Step 7: Commit:** `git add web/dashboard/src/api/assistant.ts web/dashboard/src/api/assistant.test.ts web/dashboard/src/settings/AssistantSettings.tsx web/dashboard/src/settings/AssistantSettings.test.tsx web/dashboard/src/styles.css docs/mcp.md docs/notes-api.md README.md && git commit -m "feat: add MCP context sources GUI"`.
