# Kanban MCP Card Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Extend existing profile Notes Kanban boards with safe MCP tools for listing, reading, creating, editing, work-logging, and moving cards, plus dashboard controls for the default board, intake column, and semantic column roles.

**Architecture:** `user_notes.board_json` remains the only Kanban source of truth and the existing Notes revisioned save path remains the only persistence path. A focused cloud Kanban service performs visibility checks, validation, card-level patches, and bounded optimistic-concurrency retries; the MCP dispatcher handles schemas, scopes, auditing, and JSON text results. Dashboard configuration writes board metadata through Notes autosave and merges the default-board ID through the existing per-user profile settings revision API.

**Tech Stack:** Go standard library, PostgreSQL-backed existing stores, MCP JSON-RPC protocol `2025-11-25`, React 19, TypeScript, Vitest, Testing Library, CSS.

## Global Constraints

- Execute Tasks 1-5 independently of the card-modal work. Execute Task 6 only after `docs/superpowers/plans/2026-07-17-kanban-card-modal-editor.md` is complete, because both plans modify `KanbanEditor.tsx` and `KanbanEditor.test.tsx`.
- Do not overwrite the current uncommitted modal tests in `web/dashboard/src/dashboard/KanbanEditor.test.tsx` or `web/dashboard/src/dashboard/KanbanCardModal.test.tsx`.
- Keep `user_notes.board_json` canonical. Add no task table, public route, migration, dependency, card deletion tool, or board-structure MCP mutation.
- Preserve legacy cards and all unrelated board/card fields, including `color`, timestamps, attachments, profile settings, and unknown JSON settings.
- Require `notes:read` for read tools and `notes:write` for all Kanban mutations. MCP-excluded notes and notebooks behave as not found.
- Mutations require stable IDs. Human-readable names are discovery output only and never select a write target.
- Retry no more than twice after the initial save conflict, and only when the targeted card and required columns are unchanged.
- Do not deploy or perform demo-server writes without separate user authorization.

## File Ownership Map

- `internal/protocol/notes.go`: additive serialized Kanban fields shared by cloud and agent.
- `internal/cloud/mcp_kanban.go`: Kanban discovery, validation, filtering, mutations, retry policy, and stable result DTOs.
- `internal/cloud/mcp_kanban_test.go`: pure and service-level Kanban behavior, including deterministic conflict tests through narrow interfaces.
- `internal/cloud/mcp_tools.go`: advertised tool names, JSON schemas, scopes, and annotations.
- `internal/cloud/mcp_server.go`: argument decoding, service dispatch, stable JSON text encoding, and write-audit calls.
- `internal/cloud/server.go`: constructs the focused Kanban service from the existing Notes/store dependencies.
- `web/dashboard/src/api/profileNotes.ts`: browser-side additive Kanban field types.
- `web/dashboard/src/api/profileSettings.ts`: focused revision-aware `/v1/me/profile` load/save client.
- `web/dashboard/src/dashboard/ProfileNotesPage.tsx`: owns default-board profile setting and passes board configuration state/callbacks.
- `web/dashboard/src/dashboard/KanbanEditor.tsx`: board-level intake/role controls only; card editing stays in `KanbanCardModal` after the modal plan lands.
- `docs/mcp.md`: tool/scope/operator contract.

---

### Task 1: Preserve Additive Kanban Metadata End to End

**Files:**
- Modify: `internal/protocol/notes.go`
- Modify: `web/dashboard/src/api/profileNotes.ts`
- Test: `internal/cloud/notes_api_test.go`
- Test: `web/dashboard/src/api/profileNotes.test.ts`

**Interfaces:**
- Produces: `protocol.KanbanBoard.IntakeColumnID string`, `protocol.KanbanColumn.Role string`, and `protocol.KanbanCard.Tags []string`.
- Produces: matching optional TypeScript properties `intake_column_id`, `role`, and `tags`.

- [x] **Step 1: Add failing Go and TypeScript round-trip tests**

In the existing Notes API Kanban save/fetch test, save this board and assert the fetched board deeply preserves every field:

```go
board := &protocol.KanbanBoard{
	IntakeColumnID: "ideas",
	Columns: []protocol.KanbanColumn{{
		ID: "ideas", Title: "Brainstorm", Role: "planning", SortOrder: 0,
		Cards: []protocol.KanbanCard{{
			ID: "offline-sync", Text: "Research offline sync\nCapture constraints",
			SortOrder: 0, Color: "cyan", DueDate: "2026-07-24",
			Tags: []string{"Hank", "Research"},
		}},
	}},
}
```

In `profileNotes.test.ts`, pass the same shape to `saveNote` and assert the request body includes it unchanged:

```ts
expect(request).toHaveBeenCalledWith("/v1/me/notes/board-1", expect.objectContaining({
  method: "PUT",
  body: expect.objectContaining({
    board: {
      intake_column_id: "ideas",
      columns: [{
        id: "ideas", title: "Brainstorm", role: "planning", sort_order: 0,
        cards: [{ id: "offline-sync", text: "Research offline sync\nCapture constraints", sort_order: 0, color: "cyan", due_date: "2026-07-24", tags: ["Hank", "Research"] }],
      }],
    },
  }),
}));
```

- [x] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/cloud -run 'Kanban|ProfileNotes' -count=1
npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts
```

Expected: FAIL because the three additive fields are absent from the Go and TypeScript contracts.

- [x] **Step 3: Add the fields without changing existing JSON names**

Use these exact structures:

```go
type KanbanCard struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	SortOrder int       `json:"sort_order"`
	Color     string    `json:"color,omitempty"`
	DueDate   string    `json:"due_date,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type KanbanColumn struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Role      string       `json:"role,omitempty"`
	SortOrder int          `json:"sort_order"`
	Cards     []KanbanCard `json:"cards"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type KanbanBoard struct {
	IntakeColumnID string         `json:"intake_column_id,omitempty"`
	Columns        []KanbanColumn `json:"columns"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}
```

Add matching optional fields to the current TypeScript types:

```ts
export type KanbanCard = {
  id?: string;
  text?: string;
  title?: string;
  sort_order?: number;
  color?: string;
  due_date?: string;
  tags?: string[];
};

export type KanbanColumn = {
  id?: string;
  title?: string;
  role?: "planning" | "active" | "rework" | "human" | "review" | "complete" | string;
  sort_order?: number;
  cards?: KanbanCard[];
};

export type KanbanBoard = {
  intake_column_id?: string;
  columns?: KanbanColumn[];
};
```

- [x] **Step 4: Run focused tests and verify GREEN**

Run the two commands from Step 2. Expected: PASS.

- [x] **Step 5: Commit the contract change**

```bash
git add internal/protocol/notes.go internal/cloud/notes_api_test.go web/dashboard/src/api/profileNotes.ts web/dashboard/src/api/profileNotes.test.ts
git commit -m "feat: preserve Kanban workflow metadata"
```

---

### Task 2: Build the Kanban Read Service

**Files:**
- Create: `internal/cloud/mcp_kanban.go`
- Create: `internal/cloud/mcp_kanban_test.go`

**Interfaces:**
- Consumes: `protocol.KanbanBoard`, the existing Notes visibility rules, `store.GetUserProfileSettings`, and `cloudNotesService.FetchProfile`.
- Produces: `newMCPKanbanService(store mcpKanbanStore, notes mcpKanbanNotes, now func() time.Time) *mcpKanbanService`.
- Produces: `ListBoards`, `ListCards`, and `GetCard` methods returning stable DTOs rather than raw board JSON.

- [x] **Step 1: Define narrow test fakes and failing discovery tests**

Define the production interfaces in `mcp_kanban.go`:

```go
type mcpKanbanStore interface {
	ListProfileNotes(context.Context, string, bool) ([]domain.UserNote, error)
	GetUserProfileSettings(context.Context, string) (domain.UserProfileSettings, error)
}

type mcpKanbanNotes interface {
	FetchProfile(context.Context, string, string) (protocol.NotesFetchResponse, error)
	SaveProfile(context.Context, string, string, protocol.NotesSaveRequest) (protocol.NotesSaveResponse, error)
}

type mcpKanbanService struct {
	store mcpKanbanStore
	notes mcpKanbanNotes
	now   func() time.Time
}

func newMCPKanbanService(store mcpKanbanStore, notes mcpKanbanNotes, now func() time.Time) *mcpKanbanService {
	if now == nil { now = time.Now }
	return &mcpKanbanService{store: store, notes: notes, now: now}
}
```

Create table-driven tests proving that `ListBoards`:

```go
func TestMCPKanbanListBoardsHonorsVisibilityAndDefault(t *testing.T) {
	// Visible Kanban board is returned; text notes, excluded boards, and boards
	// under an excluded notebook are absent. settings_json contains
	// {"kanban_default_board_id":"board-1"}, so board-1 is marked default.
}
```

Also add failing tests for stale defaults, an empty board, duplicate/nonempty IDs, and duplicate/unsupported roles.

- [x] **Step 2: Run service tests and verify RED**

Run: `go test ./internal/cloud -run '^TestMCPKanban' -count=1`

Expected: FAIL because `mcpKanbanService` and its DTOs do not exist.

- [x] **Step 3: Implement normalized read DTOs and helpers**

Use these public JSON shapes inside the package:

```go
type mcpKanbanColumnSummary struct {
	ID string `json:"column_id"`; Title string `json:"title"`; Role string `json:"role,omitempty"`; CardCount int `json:"card_count"`
}
type mcpKanbanBoardSummary struct {
	BoardID string `json:"board_id"`; Title string `json:"title"`; Default bool `json:"default"`; IntakeColumnID string `json:"intake_column_id,omitempty"`; Revision string `json:"revision"`
	Columns []mcpKanbanColumnSummary `json:"columns"`; TotalCardCount int `json:"total_card_count"`; ActiveCardCount int `json:"active_card_count"`
}
type mcpKanbanCardResult struct {
	BoardID string `json:"board_id"`; BoardTitle string `json:"board_title"`; BoardRevision string `json:"board_revision"`
	ColumnID string `json:"column_id"`; ColumnTitle string `json:"column_title"`; ColumnRole string `json:"column_role,omitempty"`; CardID string `json:"card_id"`
	Title string `json:"title"`; DetailsMarkdown string `json:"details_markdown"`; DueDate string `json:"due_date,omitempty"`; Tags []string `json:"tags"`
	CreatedAt time.Time `json:"created_at,omitempty"`; UpdatedAt time.Time `json:"updated_at,omitempty"`; Columns []mcpKanbanColumnSummary `json:"columns,omitempty"`
}
```

Implement these exact method signatures:

```go
func (s *mcpKanbanService) ListBoards(ctx context.Context, userID string) ([]mcpKanbanBoardSummary, error)
func (s *mcpKanbanService) ListCards(ctx context.Context, userID string, args mcpKanbanListCardsArgs) ([]mcpKanbanCardResult, error)
func (s *mcpKanbanService) GetCard(ctx context.Context, userID string, boardID string, cardID string) (mcpKanbanCardResult, error)
```

`mcpKanbanListCardsArgs` has `BoardID`, `ColumnID`, `Role`, `Query`, `Tags`, `DueFrom`, `DueThrough`, `IncludeComplete`, and `Limit`. Default `Limit` to 50, reject values above 100, use AND semantics for normalized tags, make due bounds inclusive, and let explicit `role=complete` imply `IncludeComplete`.

For `ListCards` and `GetCard`, resolve an empty `BoardID` through the usable configured default. A stale/missing default returns the typed no-default error with visible board candidates; it never silently chooses the first board.

Use the first nonblank line as title and all remaining lines as Markdown details:

```go
func splitKanbanCardText(text string) (string, string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line), strings.TrimSpace(strings.Join(lines[index+1:], "\n"))
		}
	}
	return "", ""
}
```

Reuse the same ancestor-notebook MCP exclusion logic as existing Notes tools. Treat an excluded/foreign/non-Kanban note as not found. Parse `kanban_default_board_id` from profile `settings_json`, and return visible board candidates in the typed no-default error.

- [x] **Step 4: Add filtering and compatibility tests**

Add table cases for query matching title/details case-insensitively, tag AND matching, inclusive dates, default complete exclusion, `role=complete`, stable board/column/card order, legacy cards with no metadata, and preservation of all defined compatibility fields during read normalization.

- [x] **Step 5: Run service tests and verify GREEN**

Run: `go test ./internal/cloud -run '^TestMCPKanban' -count=1`

Expected: PASS.

- [x] **Step 6: Commit the read service**

```bash
git add internal/cloud/mcp_kanban.go internal/cloud/mcp_kanban_test.go
git commit -m "feat: add Kanban MCP read service"
```

---

### Task 3: Add Card Mutations and Safe Conflict Retries

**Files:**
- Modify: `internal/cloud/mcp_kanban.go`
- Modify: `internal/cloud/mcp_kanban_test.go`

**Interfaces:**
- Consumes: the Task 2 service and DTOs.
- Produces: `CreateCard`, `UpdateCard`, `AppendWorklog`, and `MoveCard` with exact-ID mutation and at most three total save attempts.

- [x] **Step 1: Add failing mutation and validation tests**

Add tests covering create destination precedence, intake fallback, first-column fallback, no-default candidates, empty boards, partial patch semantics, work-log append, cross-column move, same-column reorder, valid end index, invalid index, normalized tags, strict calendar dates, empty titles, and preservation of `color`, timestamps, other cards, and board metadata.

Use deterministic time for work logs:

```go
fixedNow := func() time.Time { return time.Date(2026, 7, 17, 14, 30, 0, 0, time.UTC) }
want := "## Work log\n\n### 2026-07-17T14:30:00Z — Verification\n\n`go test ./...` passed."
```

Build the fake Notes service so its first save can return `noteConflictError` with either an unrelated-card change or a targeted-card change. Assert unrelated changes retry and survive; targeted changes return the latest card state without a second save.

- [x] **Step 2: Run mutation tests and verify RED**

Run: `go test ./internal/cloud -run '^TestMCPKanban(Create|Update|Append|Move|Conflict|Validation)' -count=1`

Expected: FAIL because mutation methods are undefined.

- [x] **Step 3: Implement exact mutation argument types**

```go
type mcpKanbanCreateArgs struct { BoardID, ColumnID, Title, DetailsMarkdown, DueDate string; Tags []string }
type mcpKanbanUpdateArgs struct { BoardID, CardID string; Title, DetailsMarkdown, DueDate *string; Tags *[]string }
type mcpKanbanWorklogArgs struct { BoardID, CardID, EntryMarkdown, Kind string }
type mcpKanbanMoveArgs struct { BoardID, CardID, TargetColumnID string; TargetIndex *int }

func (s *mcpKanbanService) CreateCard(ctx context.Context, userID string, args mcpKanbanCreateArgs) (mcpKanbanCardResult, error)
func (s *mcpKanbanService) UpdateCard(ctx context.Context, userID string, args mcpKanbanUpdateArgs) (mcpKanbanCardResult, error)
func (s *mcpKanbanService) AppendWorklog(ctx context.Context, userID string, args mcpKanbanWorklogArgs) (mcpKanbanCardResult, error)
func (s *mcpKanbanService) MoveCard(ctx context.Context, userID string, args mcpKanbanMoveArgs) (mcpKanbanCardResult, error)
```

Use pointer fields for patch inputs so omission differs from clearing. Join canonical content without destroying Markdown:

```go
func joinKanbanCardText(title, details string) string {
	title = strings.TrimSpace(title)
	details = strings.TrimSpace(details)
	if details == "" { return title }
	return title + "\n" + details
}
```

Normalize tags by trimming, case-insensitive deduplication, preserving the first spelling/order, limiting 20 tags, and limiting each tag to 64 Unicode code points. Validate due dates by parsing with `time.Parse("2006-01-02", value)` and requiring formatting back to the identical string.

- [x] **Step 4: Implement bounded target-aware retry**

For each attempt, fetch a fresh board, capture a mutation fingerprint, apply one patch, and call `SaveProfile` with the fetched revision. The fingerprint contains the full target card for update/worklog, the source card plus source/destination column IDs for move, and the destination column identity for create. On `noteConflictError`, reload and compare only those targets. Retry attempts 2 and 3 only when they match; otherwise return:

```go
type mcpKanbanConflictError struct {
	Message string
	Latest  *mcpKanbanCardResult
}

func (e *mcpKanbanConflictError) Error() string { return e.Message }
```

Never replace raw `board_json`; send the normalized, minimally patched `protocol.KanbanBoard` through `SaveProfile`. Generate new card IDs with `newID("kanban_card")`, append new cards at the selected destination end, and recalculate only affected card sort orders.

- [x] **Step 5: Run focused and broader cloud tests**

Run:

```bash
go test ./internal/cloud -run '^TestMCPKanban' -count=1
go test ./internal/cloud -count=1
```

Expected: PASS.

- [x] **Step 6: Commit the mutation service**

```bash
git add internal/cloud/mcp_kanban.go internal/cloud/mcp_kanban_test.go
git commit -m "feat: add safe Kanban card mutations"
```

---

### Task 4: Advertise and Dispatch the Seven MCP Tools

**Files:**
- Modify: `internal/cloud/mcp_tools.go`
- Modify: `internal/cloud/mcp_server.go`
- Modify: `internal/cloud/server.go`
- Test: `internal/cloud/mcp_unit_test.go`
- Test: `internal/cloud/mcp_flow_test.go`

**Interfaces:**
- Consumes: all Task 2-3 service methods.
- Produces: MCP tools `list_kanban_boards`, `list_kanban_cards`, `get_kanban_card`, `create_kanban_card`, `update_kanban_card`, `append_kanban_worklog`, and `move_kanban_card`.

- [x] **Step 1: Add failing schema, annotation, scope, result, and audit tests**

Assert `tools/list` returns all seven tools with `additionalProperties: false`. Read tools must contain `annotations: {"readOnlyHint":true}`; writes must contain `annotations: {"readOnlyHint":false,"destructiveHint":false}`. Assert reads fail without `notes:read`, writes fail without `notes:write`, results are JSON text without `board_json`, and write audit metadata contains IDs but no title/details/tags/work-log content.

Extend the OAuth flow test to create a Kanban note through the existing Notes service, then call list/get/create/update/worklog/move by exact IDs.

- [x] **Step 2: Run MCP tests and verify RED**

Run: `go test ./internal/cloud -run 'MCP' -count=1`

Expected: FAIL because the tools are not advertised or dispatched.

- [x] **Step 3: Add annotations and complete JSON schemas**

Extend the definition and list serializer:

```go
type mcpToolDef struct {
	Name string; Description string; InputSchema map[string]any; Scopes []string; Annotations map[string]any
}

if len(d.Annotations) > 0 { item["annotations"] = d.Annotations }
```

Add `mcpBool`, `mcpStringArray`, and enum helpers beside `mcpStr`/`mcpInt`. Schemas must encode every approved argument and required field. `create_kanban_card` description must state: “Call only after the user explicitly asks to capture a task.” `kind` enum is `progress|verification|blocker|outcome`; role enum is `planning|active|rework|human|review|complete`; list limit maximum is 100; `target_index` minimum is 0.

- [x] **Step 4: Construct and dispatch the service**

Add `kanban *mcpKanbanService` to `Server`. After the existing Notes service is constructed, initialize it with:

```go
server.kanban = newMCPKanbanService(server.store, server.notes, func() time.Time { return time.Now().UTC() })
```

Add seven small switch cases in `executeMCPTool`: decode with `decodeMCPArgs`, call the matching service method, encode the DTO with the existing JSON helper, and never expose raw board JSON. For mutations call a new audit helper accepting a metadata map:

```go
func (s *Server) auditMCPKanbanWrite(ctx context.Context, auth mcpAuthContext, tool string, result mcpKanbanCardResult, extra map[string]string)
```

Audit `tool`, MCP client ID, `board_id`, `card_id`, and relevant source/destination column IDs only.

- [x] **Step 5: Return friendly typed errors**

Map validation, no-default, not-found, and conflict errors to the existing MCP tool error response convention. Include visible board candidates for no-default and `Latest` for targeted conflict. Do not include a stack, raw board JSON, or card content beyond the conflict DTO approved by the design.

- [x] **Step 6: Run MCP and cloud tests**

Run:

```bash
go test ./internal/cloud -run 'MCP' -count=1
go test ./internal/cloud -count=1
```

Expected: PASS.

- [x] **Step 7: Commit the MCP boundary**

```bash
git add internal/cloud/mcp_tools.go internal/cloud/mcp_server.go internal/cloud/server.go internal/cloud/mcp_unit_test.go internal/cloud/mcp_flow_test.go
git commit -m "feat: expose Kanban card MCP tools"
```

---

### Task 5: Add the Revision-Aware Profile Settings Client

**Files:**
- Create: `web/dashboard/src/api/profileSettings.ts`
- Create: `web/dashboard/src/api/profileSettings.test.ts`

**Interfaces:**
- Produces: `profileSettingsClient.load()` and `profileSettingsClient.save(expectedRevision, settings)`.
- Produces: `mergeDefaultKanbanBoard(settings, boardID)` that preserves every unrelated setting and deletes only `kanban_default_board_id` when `boardID` is empty.

- [x] **Step 1: Write failing client and merge tests**

```ts
it("merges and clears only the default Kanban board", () => {
  const current = { dashboard: { density: "compact" }, assistant: { model: "gpt" } };
  expect(mergeDefaultKanbanBoard(current, "board-1")).toEqual({
    ...current, kanban_default_board_id: "board-1",
  });
  expect(mergeDefaultKanbanBoard({ ...current, kanban_default_board_id: "board-1" }, "")).toEqual(current);
});

it("saves with the loaded revision", async () => {
  await client.save(7, { dashboard: { density: "compact" }, kanban_default_board_id: "board-1" });
  expect(request).toHaveBeenCalledWith("/v1/me/profile", {
    method: "PUT",
    body: { expected_revision: 7, settings: { dashboard: { density: "compact" }, kanban_default_board_id: "board-1" } },
  });
});
```

- [x] **Step 2: Run the focused test and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/api/profileSettings.test.ts`

Expected: FAIL because the module does not exist.

- [x] **Step 3: Implement the focused client**

```ts
import { apiClient, type ApiTransport } from "./client";

export type ProfileSettings = Record<string, unknown>;
export type ProfileSettingsResponse = { revision: number; settings: ProfileSettings };

export class ProfileSettingsClient {
  constructor(private readonly api: ApiTransport = apiClient) {}
  load() { return this.api.request<ProfileSettingsResponse>("/v1/me/profile"); }
  save(expectedRevision: number, settings: ProfileSettings) {
    return this.api.request<ProfileSettingsResponse>("/v1/me/profile", {
      method: "PUT", body: { expected_revision: expectedRevision, settings },
    });
  }
}

export function mergeDefaultKanbanBoard(settings: ProfileSettings, boardID: string): ProfileSettings {
  const next = { ...settings };
  if (boardID) next.kanban_default_board_id = boardID;
  else delete next.kanban_default_board_id;
  return next;
}

export const profileSettingsClient = new ProfileSettingsClient();
```

- [x] **Step 4: Run the client tests and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/api/profileSettings.test.ts`

Expected: PASS.

- [x] **Step 5: Commit the settings client**

```bash
git add web/dashboard/src/api/profileSettings.ts web/dashboard/src/api/profileSettings.test.ts
git commit -m "feat: add profile settings client"
```

---

### Task 6: Add Human-Managed Kanban Workflow Controls

**Prerequisite:** Complete and commit `docs/superpowers/plans/2026-07-17-kanban-card-modal-editor.md`, then re-read the resulting `KanbanEditor` props before applying this task. Do not move workflow controls into `KanbanCardModal`; they belong to the board workbar and column menus.

**Files:**
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`
- Modify: `web/dashboard/src/dashboard/styles.css`

**Interfaces:**
- Consumes: Task 1 metadata types, Task 5 client, and the completed modal editor.
- Produces: `KanbanEditor` props `isDefaultBoard`, `onSetDefaultBoard`, and board-metadata updates through existing `onChange`.

- [x] **Step 1: Add failing board and page tests**

In `KanbanEditor.test.tsx`, assert the board workbar exposes `Set as default board` or `Clear default board`; each column menu exposes `Set as intake` and a labeled role selector; badges expose `Intake`, `Planning`, `Active Work`, `Rework`, `Needs Human`, `Review`, and `Complete` text accessibly.

Exercise this state transition exactly:

```ts
fireEvent.click(screen.getByRole("button", { name: "Column options for Brainstorm" }));
fireEvent.click(screen.getByRole("button", { name: "Set Brainstorm as intake" }));
fireEvent.change(screen.getByLabelText("Role for Brainstorm"), { target: { value: "planning" } });
fireEvent.change(screen.getByLabelText("Role for Backlog"), { target: { value: "planning" } });
```

Assert intake becomes `Brainstorm`, and moving `planning` to `Backlog` clears it from `Brainstorm`. Delete the intake/role column and assert `intake_column_id` is empty and no deleted role remains.

In `ProfileNotesPage.test.tsx`, mock `/v1/me/profile` with revision 7 and unrelated settings, set/clear the default, and assert PUT merges only `kanban_default_board_id`.

- [x] **Step 2: Run focused tests and verify RED**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/dashboard/KanbanEditor.test.tsx src/dashboard/ProfileNotesPage.test.tsx
```

Expected: FAIL because workflow controls and settings ownership are absent.

- [x] **Step 3: Own default-board state in the Notes page**

Load profile settings alongside note summaries, retain `revision`, `settings`, and `defaultBoardID`, then pass these props:

```tsx
<KanbanEditor
  board={board}
  onChange={setBoard}
  isDefaultBoard={defaultBoardID === selectedNoteID}
  onSetDefaultBoard={(enabled) => void saveDefaultBoard(enabled ? selectedNoteID : "")}
  {...existingModalPlanProps}
/>
```

`saveDefaultBoard` merges through `mergeDefaultKanbanBoard`, saves with the loaded revision, and adopts the returned revision/settings. On HTTP 409, reload once, re-merge the intended board ID into the newest settings, retry once, and surface the existing page error if the second write conflicts.

- [x] **Step 4: Implement intake and unique role updates in the board editor**

Use one role constant for values and labels:

```ts
const KANBAN_ROLES = [
  ["", "None"], ["planning", "Planning"], ["active", "Active Work"],
  ["rework", "Rework"], ["human", "Needs Human"],
  ["review", "Review"], ["complete", "Complete"],
] as const;
```

Setting a nonempty role maps every column, assigning it to the selected column and clearing it from any other column. Setting intake writes `intake_column_id`. Column deletion clears intake automatically when IDs match; deleting the column naturally removes its role. Commit through the existing `onChange`/serialized Notes autosave and preserve all card fields.

- [x] **Step 5: Add accessible badges and responsive styles**

Render small text badges beside column titles and a default badge in the board workbar. Keep buttons/selects reachable by keyboard and preserve the modal plan's near-full-screen narrow layout. Add only scoped `.kanban-*` selectors to `styles.css`.

- [x] **Step 6: Run dashboard tests and build**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/dashboard/KanbanEditor.test.tsx src/dashboard/KanbanCardModal.test.tsx src/dashboard/ProfileNotesPage.test.tsx src/api/profileSettings.test.ts src/api/profileNotes.test.ts
npm --prefix web/dashboard run test:run
npm --prefix web/dashboard run build
```

Expected: PASS, and the production dashboard build completes without TypeScript errors.

- [x] **Step 7: Commit the workflow UI**

```bash
git add web/dashboard/src/dashboard/ProfileNotesPage.tsx web/dashboard/src/dashboard/ProfileNotesPage.test.tsx web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx web/dashboard/src/dashboard/styles.css
git commit -m "feat: configure Kanban workflow roles"
```

---

### Task 7: Document and Verify the Complete Local Contract

**Files:**
- Modify: `docs/mcp.md`
- Modify: `internal/cloud/mcp_flow_test.go`

**Interfaces:**
- Consumes: all prior tasks.
- Produces: documented scopes/tool behavior and a repeatable local JSON-RPC smoke path.

- [x] **Step 1: Document the seven tools and behavior boundaries**

Add rows mapping read tools to `notes:read` and write tools to `notes:write`. Document default-board/intake fallback, role meanings, exact-ID writes, no card deletion/board mutation, three-attempt conflict policy, explicit capture intent, and the current ChatGPT Voice limitation. State that profile Notes content sent through an assistant follows the existing Notes MCP privacy model.

- [x] **Step 2: Add a direct JSON-RPC smoke assertion**

Extend the existing in-process MCP flow test to call `initialize`, `tools/list`, then the seven tools in this order: list boards, list cards, get card, create, update, append worklog, move. Assert every response has protocol `2025-11-25`, JSON-decodable text content, exact board/card/column IDs, and no `board_json` field.

- [x] **Step 3: Run formatting and full repository gates**

Run:

```bash
gofmt -w ./cmd ./internal
go test ./...
go build ./...
npm --prefix web/dashboard run test:run
npm --prefix web/dashboard run build
git diff --check
```

Expected: every command succeeds. No migration/status command is required because this design adds JSON fields only and no database schema change.

- [x] **Step 4: Review security and persistence invariants**

Confirm from tests and diff that no new route exists; read/write scopes are enforced; ownership and MCP exclusions are reused; audits omit content; there is no card deletion; Settings and board JSON updates preserve unrelated keys/fields; and all writes use expected revisions.

- [x] **Step 5: Commit documentation and final verification coverage**

```bash
git add docs/mcp.md internal/cloud/mcp_flow_test.go
git commit -m "docs: describe Kanban MCP tools"
```

Do not deploy. After deployment is separately authorized, verify the advertised tools through Codex and typed ChatGPT. Voice verification remains deferred until ChatGPT apps support Voice.
