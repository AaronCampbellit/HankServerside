# Synced Pinned Notes and Agent Device Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Do not use subagents; repository instructions prohibit delegation unless the user explicitly requests it.

**Goal:** Add durable synced pinning for notebook child notes and replace fragmented agent details with a tabbed device workspace containing Remote Desktop.

**Architecture:** Extend the existing Notes persistence and collaboration contract with an additive `pinned` field, then expose pinning through the current authenticated save flow. Refactor the dashboard’s existing Agents and Desktop viewer components into deep-linkable device tabs without changing agent protocols, authorization, trust, or session lifecycle.

**Tech Stack:** Go, PostgreSQL migrations, React 19, TypeScript, Vitest, Testing Library, Vite.

## Global Constraints

- Work only in `HankServerside`; do not modify Hankagent or client repositories.
- Preserve unrelated dirty-worktree changes.
- Do not commit, push, deploy, or publish without explicit user authorization.
- Use migration `000027`; never mutate the schema from startup code.
- Pinned notes must be notebook children and cannot be notebooks.
- Preserve Notes owner scoping, write scopes, CSRF, and optimistic revision checks.
- Preserve Remote Desktop admin authorization, trust, readiness, audit, encryption, session lifecycle, and the existing `/dashboard/agents/{agentID}/desktop` deep link.
- Use test-first red-green cycles for every behavior change.

---

## Milestone 1: Synced Pinned Notes

### Task 1: Persist and materialize the pinned field

**Files:**
- Create: `internal/migrations/sql/000027_note_pinning.up.sql`
- Create: `internal/migrations/sql/000027_note_pinning.down.sql`
- Modify: `internal/migrations/migrations_test.go`
- Modify: `internal/domain/notes.go`
- Modify: `internal/store/user_notes.go`
- Modify: `internal/store/user_notes_test.go`
- Modify: `internal/protocol/notes.go`
- Modify: `internal/cloud/notes_store.go`
- Modify: `internal/cloud/note_collaboration.go`

**Interfaces:**
- Produces: `domain.UserNote.Pinned bool`
- Produces: `protocol.NoteSummary.Pinned bool`, `protocol.NotesFetchResponse.Pinned bool`, and `protocol.NotesSaveRequest.Pinned *bool`
- Produces: database column `user_notes.pinned BOOLEAN NOT NULL DEFAULT FALSE`

- [ ] **Step 1: Add failing migration and store tests**

Add assertions that migration `000027` defines the additive column and a check equivalent to:

```sql
CHECK (NOT pinned OR (parent_id IS NOT NULL AND page_type <> 'notebook'))
```

Extend the PostgreSQL-backed note store test to upsert and fetch a child with `Pinned: true` and confirm it round-trips.

- [ ] **Step 2: Run the red tests**

Run:

```bash
go test ./internal/migrations ./internal/store -run 'TestNotePinning|TestUserNote' -count=1
```

Expected: failure because migration `000027` and `UserNote.Pinned` do not exist.

- [ ] **Step 3: Add migration and domain/store plumbing**

The up migration must:

```sql
ALTER TABLE user_notes
  ADD COLUMN IF NOT EXISTS pinned BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE user_notes
  ADD CONSTRAINT user_notes_pinned_child_check
  CHECK (NOT pinned OR (parent_id IS NOT NULL AND page_type <> 'notebook'));

CREATE INDEX IF NOT EXISTS idx_user_notes_owner_parent_pinned_updated
  ON user_notes(owner_user_id, parent_id, pinned DESC, updated_at DESC);
```

The down migration drops the index, constraint, and column. Add `Pinned` beside notebook metadata in `domain.UserNote`; include it in both store column lists, scans, inserts, and conflict updates.

- [ ] **Step 4: Carry pinning through protocol and collaboration state**

Add the three protocol fields named above. Extend `collabState` with a boolean scalar, preserve the current value when `NotesSaveRequest.Pinned == nil`, force `false` for root notes and notebooks, and include pinning in revision and operation JSON:

```go
"pinned": state.Pinned,
```

Materialized records must assign:

```go
base.Pinned = state.Pinned
```

- [ ] **Step 5: Run persistence tests green**

Run:

```bash
gofmt -w internal/domain internal/store internal/protocol internal/cloud internal/migrations
go test ./internal/migrations ./internal/store ./internal/protocol ./internal/cloud -run 'TestNotePinning|TestUserNote|TestNotes' -count=1
```

Expected: relevant tests pass. PostgreSQL-backed tests must be reported as skipped if `HANK_REMOTE_TEST_DATABASE_URL` is unavailable.

### Task 2: Expose pinned state through the existing Notes API and client

**Files:**
- Modify: `internal/cloud/notes_api_test.go`
- Modify: `internal/cloud/notes_store.go`
- Modify: `web/dashboard/src/api/profileNotes.ts`
- Modify: `web/dashboard/src/api/profileNotes.test.ts`

**Interfaces:**
- Consumes: `NotesSaveRequest.Pinned *bool`
- Produces: `ProfileNoteSummary.pinned?: boolean`
- Produces: `SaveProfileNoteInput.pinned?: boolean`

- [ ] **Step 1: Add failing HTTP and TypeScript client tests**

The Go test creates a notebook and child, saves the child with `"pinned": true`, then asserts GET and list responses expose `pinned: true`. It must also assert a root note or notebook cannot remain pinned.

The client test calls:

```ts
profileNotesClient.saveNote({ ...note, pinned: true })
```

and expects the request body to contain `pinned: true`.

- [ ] **Step 2: Run the red tests**

Run:

```bash
go test ./internal/cloud -run TestProfileNotesNotebookPinning -count=1
npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts
```

Expected: assertions fail because responses and client serialization omit `pinned`.

- [ ] **Step 3: Implement the contract**

Map `domain.UserNote.Pinned` into list/fetch response builders. Add `pinned` to the TypeScript summary and save types and serialize:

```ts
pinned: Boolean(input.pinned),
```

The server remains responsible for forcing invalid root/notebook pins to `false`; the database constraint is the final integrity boundary.

- [ ] **Step 4: Run contract tests green**

Run both commands from Step 2 and expect success.

### Task 3: Add pin controls and pinned-first notebook ordering

**Files:**
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: `ProfileNoteSummary.pinned`
- Produces: row actions `Pin <title>` and `Unpin <title>`
- Produces: toolbar action `Pin note` or `Unpin note`

- [ ] **Step 1: Add a failing ordering test**

Render a selected notebook with one pinned and two unpinned children where the pinned child is last in API order. Assert the “Note cards” list exposes the pinned child first and preserves the two unpinned children’s order.

- [ ] **Step 2: Run the ordering test red**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/dashboard/ProfileNotesPage.test.tsx -t 'sorts pinned notebook notes first'
```

Expected: the pinned child remains in API order.

- [ ] **Step 3: Implement stable partitioning**

When `selectedNotebookID` is an explicit notebook, partition children without re-sorting within groups:

```ts
const pinned = children.filter((note) => note.pinned);
const unpinned = children.filter((note) => !note.pinned);
primary = [...pinned, ...unpinned];
```

- [ ] **Step 4: Add failing interaction and rollback tests**

Assert:

- Notebook child rows expose Pin/Unpin actions.
- Root notes and notebooks expose no pin action.
- Clicking Pin fetches the full note, saves it with `pinned: true`, and moves it into the pinned group.
- A rejected save leaves ordering unchanged and reports an error.
- The selected child toolbar mirrors the row action.

- [ ] **Step 5: Run interaction tests red**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/dashboard/ProfileNotesPage.test.tsx -t 'pins|unpins|pin failure'
```

Expected: pin controls are absent.

- [ ] **Step 6: Implement pin and unpin**

Add a `setNotePinned(summary, pinned)` helper that fetches the complete note, calls `saveNote` with every canonical field plus the new pin state, then updates the matching summary and selected editor revision. Reuse existing toast/error handling; do not optimistically mutate before the server succeeds.

- [ ] **Step 7: Run the Notes milestone green**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts src/dashboard/ProfileNotesPage.test.tsx
go test ./internal/migrations ./internal/store ./internal/protocol ./internal/cloud -run 'TestNotePinning|TestProfileNotesNotebookPinning|TestNotes' -count=1
```

Expected: all focused Notes tests pass.

---

## Milestone 2: Agent Device Workspace

### Task 4: Add deep-linkable device routes and accessible tabs

**Files:**
- Create: `web/dashboard/src/dashboard/AgentsPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/AgentsPage.tsx`
- Modify: `web/dashboard/src/App.tsx`
- Modify: `web/dashboard/src/App.test.tsx`
- Modify: `web/dashboard/src/router.ts`
- Modify: `web/dashboard/src/router.test.ts`

**Interfaces:**
- Produces: `AgentWorkspaceTab = "overview" | "desktop" | "terminal" | "security"`
- Produces: `AgentsPage({ initialAgentID?, initialTab? })`
- Produces: route helpers that decode the agent ID and selected tab

- [ ] **Step 1: Add failing router and page tests**

Test these mappings:

```text
/dashboard/agents/agent-1           -> overview
/dashboard/agents/agent-1/desktop   -> desktop, admin-only
/dashboard/agents/agent-1/terminal  -> terminal, admin-only
/dashboard/agents/agent-1/security  -> security, admin-only
```

Render an admin agent and assert the four native tab buttons, correct `aria-selected`, and Overview content. Assert choosing a device navigates to its encoded overview URL and Back returns to `/dashboard/agents`.

- [ ] **Step 2: Run route/workspace tests red**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/router.test.ts src/App.test.tsx src/dashboard/AgentsPage.test.tsx
```

Expected: the new route patterns and tab workspace are missing.

- [ ] **Step 3: Implement route parsing and workspace shell**

Add a single dynamic route parser shared by `routeForPath` and `pageForRoute`. Refactor `AgentDetail` to accept the selected tab and a tab-change callback. Use:

```tsx
<div role="tablist" aria-label={`${agentName} workspace`}>
  <button role="tab" aria-selected={tab === "overview"}>Overview</button>
  <button role="tab" aria-selected={tab === "desktop"}>Remote Desktop</button>
  <button role="tab" aria-selected={tab === "terminal"}>Terminal</button>
  <button role="tab" aria-selected={tab === "security"}>Security</button>
</div>
```

Navigation must use the dashboard’s internal navigation mechanism so browser history and direct loads agree.

- [ ] **Step 4: Place existing content by responsibility**

- Overview: metrics, details, lock/wake/restart.
- Terminal: `ShellConsole` or the existing capability/admin explanation.
- Security: identifiers, capabilities, `DesktopTrustSettings`, and Remove device.
- Keep `TokenSection` only on `/dashboard/agents`.

- [ ] **Step 5: Run workspace tests green**

Run the Step 2 command and expect success.

### Task 5: Embed Remote Desktop without changing its security lifecycle

**Files:**
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.test.tsx`
- Modify: `web/dashboard/src/desktop/DesktopViewerPage.tsx`
- Modify: `web/dashboard/src/dashboard/AgentsPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/AgentsPage.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Produces: `DesktopViewerPage({ agentID?, dependencies?, embedded? })`
- Consumes: existing `DesktopAgentReadiness`, `desktopReadinessComplete`, and desktop session APIs

- [ ] **Step 1: Add a failing embedded-viewer test**

Render:

```tsx
<DesktopViewerPage agentID="agent-1" embedded dependencies={dependencies} />
```

Assert it retains the connect/session controls but omits a duplicate page-level Remote Desktop heading and standalone page wrapper.

- [ ] **Step 2: Run the embedded-viewer test red**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/desktop/DesktopViewerPage.test.tsx -t embedded
```

Expected: the component has no embedded presentation.

- [ ] **Step 3: Implement embedded presentation**

Keep the existing access, support, session, reconnect, terminate, cleanup, and encrypted socket logic unchanged. Restrict the refactor to wrapper/heading selection and reusable content rendering.

- [ ] **Step 4: Add failing Agents desktop-tab tests**

Assert:

- Ready, capable agents show the embedded viewer.
- Incomplete readiness shows `DesktopReadinessCard` and a button/link to Security.
- Active-session termination remains confirmed.
- Non-admin users cannot operate the Desktop, Terminal, or Security tabs.
- Existing `/dashboard/agents/{id}/desktop` loads the Agents workspace with Remote Desktop selected.

- [ ] **Step 5: Implement the Remote Desktop tab**

Render readiness and active-session controls first. Render the embedded viewer only when the agent is online, advertises both desktop capabilities, the user is admin, and readiness is complete. Link incomplete setup to the device Security route.

- [ ] **Step 6: Add responsive workspace styling**

Use the existing Agents visual tokens. Keep one dominant tab panel, compact status/metric hierarchy, and a single-column narrow layout. Do not change global typography or unrelated dashboard pages.

- [ ] **Step 7: Run the Agent milestone green**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/router.test.ts src/App.test.tsx src/dashboard/AgentsPage.test.tsx src/desktop/DesktopViewerPage.test.tsx
```

Expected: all focused routing, workspace, and desktop lifecycle tests pass.

---

## Milestone 3: Full Verification and Board Handoff

### Task 6: Verify platform safety and finish both cards

**Files:**
- Modify only if verification exposes a scoped defect.

- [ ] **Step 1: Format and run focused checks**

```bash
gofmt -w internal/domain internal/store internal/protocol internal/cloud internal/migrations
go test ./internal/migrations ./internal/store ./internal/protocol ./internal/cloud -count=1
npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts src/dashboard/ProfileNotesPage.test.tsx src/router.test.ts src/App.test.tsx src/dashboard/AgentsPage.test.tsx src/desktop/DesktopViewerPage.test.tsx
```

- [ ] **Step 2: Run complete frontend gates**

```bash
make frontend-test
make frontend-check
make frontend-build
```

- [ ] **Step 3: Run complete Go/build gates**

```bash
go test ./...
go build ./...
git diff --check
```

- [ ] **Step 4: Run database workflow checks**

```bash
make migrate-status
make schema-drift-check
```

If either command requires `HANK_REMOTE_TEST_DATABASE_URL` or another unavailable environment dependency, preserve the exact output and report database validation as pending rather than passing.

- [ ] **Step 5: Inspect final scope**

Confirm `git status --short` and `git diff --stat` contain only the previously approved dirty work, these two board items, their tests, migration, and design/plan documents. Confirm no `.env` or secret material is present.

- [ ] **Step 6: Update Hank Build**

Append outcome and verification work logs to:

- `card-d4fa954f-36af-4336-8f99-5fb811d0f498` — synced pinned notes.
- `card-b9de5696-a6af-41ba-bcc7-568c386bd68d` — Agents device workspace.

Move completed cards to the configured Review column. Leave the deferred security card in Inbox.
