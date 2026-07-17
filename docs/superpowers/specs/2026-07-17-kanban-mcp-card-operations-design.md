# Kanban MCP Card Operations Design

## Goal

Extend the existing remote MCP endpoint with safe, card-level tools for the existing profile Notes Kanban boards. Typed ChatGPT and Codex must be able to list boards and cards, read and search cards, create cards, edit card fields, append dated work results, and move cards through a configurable workflow without reading or replacing raw board JSON.

The intended workflow is:

1. The user brainstorms with an assistant.
2. The assistant creates a card only after an explicit instruction such as "add that as a task."
3. A card with no named destination goes to the configured default board and intake column.
4. Codex can later select that card, work it, append results, and autonomously move it through the board's configured stages.

ChatGPT Voice does not currently support apps, so Voice invocation is not part of this version. The MCP contract should remain suitable for Voice when OpenAI enables app tools there. See [Apps in ChatGPT](https://help.openai.com/en/articles/11487775-connectors-in) for the current product limitation.

## Scope

This version manages cards only. MCP can inspect board and column structure but cannot create, rename, reorder, or delete boards or columns. It also cannot delete cards. Board structure, the default board, the intake column, and semantic column roles remain human-managed in the Kanban UI.

The existing profile Notes Kanban board is the only source of truth. This design does not introduce a task table, a second task API, or a separate task service.

## Data Model

### Existing board storage

`user_notes.board_json` remains canonical. Additive fields extend the existing JSON contract:

- `KanbanBoard.intake_column_id` optionally identifies the default capture column.
- `KanbanColumn.role` optionally contains one of `planning`, `active`, `rework`, `human`, `review`, or `complete`.
- `KanbanCard.tags` is an optional normalized list of tags.
- `KanbanCard.due_date` remains an optional `YYYY-MM-DD` date.

Column roles are unique within a board. Assigning a role to one column clears that role from the previous column. Roles guide automation but do not constrain visible column names or order. A board may omit some or all roles.

The existing card `text` field remains backward compatible and canonical for human content. Its first nonblank line is the card title; the remaining content is Markdown details. Existing fields such as card color are preserved by MCP mutations even though MCP does not manage them.

### Default board

The selected default board note ID is stored as `kanban_default_board_id` in the existing per-user profile `settings_json`. UI updates must merge this key without replacing unrelated profile settings. If the referenced note is deleted, converted from Kanban, excluded from MCP, or no longer owned by the user, MCP treats the setting as stale and reports that no usable default exists.

These are additive JSON changes. No database migration is expected.

## Ownership and Components

The MCP boundary continues to expose only the authenticated user's own profile notes. Existing MCP exclusion rules apply: an excluded Kanban note, or one under an excluded notebook, behaves as not found.

Kanban mutation logic belongs in a focused cloud service rather than in the MCP dispatcher. The service owns:

- discovering visible Kanban notes;
- resolving the configured default board and intake column;
- normalizing and validating board, column, and card data;
- filtering cards;
- applying one card-level patch or move;
- retrying safe optimistic-concurrency conflicts; and
- returning stable result objects.

The MCP dispatcher owns tool argument decoding, scope enforcement, audit calls, and result encoding. The dashboard owns the human configuration controls. The store and existing Notes save path remain responsible for persistence and revision checks.

## MCP Tool Contract

All tools return stable JSON-encoded MCP text content, matching the endpoint's existing result convention, containing the relevant board ID, column ID, card ID, and board revision. They never return or accept raw `board_json`.

### Read tools

#### `list_kanban_boards`

Returns every MCP-visible profile Kanban board with:

- board ID and title;
- whether it is the usable default;
- intake column ID;
- revision;
- ordered columns with IDs, titles, roles, and card counts; and
- total and active card counts.

Requires `notes:read` and is annotated read-only.

#### `list_kanban_cards`

Lists concise card results. Optional filters are:

- `board_id`, falling back to the configured default;
- `column_id` or semantic `role`;
- case-insensitive text query across title and details;
- tags;
- due-date bounds;
- `include_complete`, which defaults to `false`; and
- bounded result limit.

Multiple requested tags use AND semantics. Due-date bounds are inclusive. An explicit `role=complete` filter implies completed cards should be included even when `include_complete` is omitted. The default limit is 50 and the maximum is 100.

Each result contains enough board, column, card, due-date, tag, and revision context for a later exact write. Requires `notes:read` and is annotated read-only.

#### `get_kanban_card`

Requires `card_id`; `board_id` is optional and falls back to the configured default. Returns the full title, Markdown details, due date, tags, current column and role, the board's complete ordered column summaries, timestamps, and board revision. Requires `notes:read` and is annotated read-only.

### Write tools

#### `create_kanban_card`

Requires `title`. Optional inputs are `details_markdown`, `due_date`, `tags`, `board_id`, and `column_id`.

Destination resolution is deterministic:

1. Use an explicit board and column when both are supplied.
2. Otherwise use the explicit board or configured default board.
3. Within that board, use the explicit column, configured intake column, or first ordered column.
4. If no usable default exists, return the available boards and require a selection.
5. If the selected board has no columns, return an actionable error and do not create a card.

The tool description instructs assistants to call it only after an explicit user request to capture a task. The server cannot infer conversational intent, so this is a client-behavior rule rather than an authorization rule. Requires `notes:write` and is annotated as a non-destructive write.

#### `update_kanban_card`

Requires `card_id`; `board_id` is optional and falls back to the configured default. It patches only supplied fields: `title`, `details_markdown`, `due_date`, and `tags`. Omission preserves a field, an empty due date clears it, and an empty tag list clears tags. An empty title is invalid.

Requires `notes:write` and is annotated as a non-destructive write.

#### `append_kanban_worklog`

Requires `card_id` and `entry_markdown`. It also accepts a `kind` of `progress`, `verification`, `blocker`, or `outcome`, plus an optional `board_id`.

The server preserves the original title and Markdown details, ensures a `## Work log` section exists, and appends an entry headed by the server's RFC 3339 UTC timestamp and the human-readable kind. It never rewrites earlier work-log entries.

Requires `notes:write` and is annotated as a non-destructive write.

#### `move_kanban_card`

Requires `card_id` and `target_column_id`; `board_id` is optional and falls back to the configured default. An optional zero-based `target_index` controls position and otherwise defaults to the end of the target column. Valid positions range from zero through the target column's resulting card count, inclusive at the end. The source and destination may be the same column to support reordering.

Requires `notes:write` and is annotated as a non-destructive write.

### Name resolution and autonomous transitions

Read results expose human names and stable IDs. Mutations require IDs and never choose among duplicate titles or similar column names. An assistant first lists or searches, asks the user when multiple cards remain plausible, and then writes by exact ID.

Semantic roles let Codex infer appropriate transitions after it has been asked to work a card. Codex may move freely among configured stages, including Planning, Active Work, Rework, Needs Human, Review, and Complete. Every transition is returned to and reported by the client. The server does not impose a fixed workflow order.

## Kanban UI Configuration

The board workbar provides a `Set as default board` action, or `Clear default board` when already selected. The selected default is visible on the board and is saved through the existing profile-settings revision flow.

Each column menu provides:

- `Set as intake`; and
- a role selector for Planning, Active Work, Rework, Needs Human, Review, Complete, or None.

Small labels show the intake and role assignments without changing column titles. Assigning a unique role transfers it from the previous column. Deleting the intake column clears `intake_column_id`; MCP then falls back to the first ordered column. Deleting a role-bearing column removes that role with the column.

The dashboard must preserve all unrelated board and profile settings during these updates.

## Mutation and Concurrency Flow

For each write:

1. Authenticate the MCP bearer token and enforce the tool's Notes scope.
2. Load the user-owned, MCP-visible Kanban note.
3. Locate the exact target card and column IDs.
4. Capture the target card's original value when the operation modifies an existing card.
5. Apply only the requested mutation to a normalized board copy.
6. Save through the existing Notes service using the loaded revision.
7. If the save conflicts, reload and retry only when the targeted card and required columns are unchanged.
8. Allow at most two conflict retries after the first attempt.
9. Stop with a conflict containing the latest card state when the target changed, disappeared, or moved in a way that invalidates the requested operation.
10. Audit the successful write and return the updated card, column, and revision.

This makes unrelated dashboard autosaves retryable while preventing MCP from silently overwriting a human edit to the same card.

## Validation and Failure Behavior

- Board, column, and card IDs must be nonempty and unique within their owning board.
- Due dates must be empty or valid `YYYY-MM-DD` calendar dates.
- Tags are trimmed, deduplicated case-insensitively, and returned in stable order. A card may have at most 20 tags and each tag may contain at most 64 Unicode code points.
- Titles and Markdown use the existing Notes payload limits; MCP must not introduce a larger bypass.
- Unsupported roles and invalid target positions return validation errors.
- A missing intake column falls back to the first ordered column.
- A missing, stale, or unusable default returns an actionable error plus visible board candidates.
- Missing or ambiguous human names never trigger a write.
- Unauthorized and MCP-excluded boards behave as not found.
- A card or destination column that disappears during a retry returns a conflict rather than being guessed.
- Completed cards are hidden from `list_kanban_cards` by default only when their column has the `complete` role; callers can include them explicitly.

## Security and Privacy

- Read tools require `notes:read`.
- All mutations require `notes:write`.
- Existing per-user ownership and notebook exclusion logic remains authoritative.
- No card-delete or board-structure write tool is added.
- No new public route, credential, secret, or filesystem access is introduced.
- Audit records include tool name, MCP client ID, board ID, card ID, and source/destination column IDs where relevant. They do not include title, Markdown, tags, or work-log content.
- MCP tool annotations distinguish read-only calls from non-destructive writes for clients and approval systems.
- Existing MCP rate limiting and OAuth token handling remain unchanged.

When used through a cloud assistant, board content accessed through MCP is sent to that assistant's provider under the same privacy model already documented for profile Notes.

## Testing and Verification

### Unit and service tests

Cover:

- board discovery and MCP exclusions;
- default-board and intake-column resolution;
- unique column-role behavior;
- card filtering, complete-role handling, and stable ordering;
- create, patch, work-log append, move, and same-column reorder operations;
- field validation and compatibility with legacy cards;
- preservation of unrelated card and board fields;
- unrelated-conflict retries and targeted-conflict rejection; and
- stale default, missing card, empty board, and missing destination errors.

### MCP tests

Cover:

- advertised names, input schemas, descriptions, and annotations;
- `notes:read` versus `notes:write` enforcement;
- stable JSON results without raw board JSON;
- friendly validation, not-found, ambiguity, and conflict responses; and
- content-free write audit events.

### Dashboard tests

Cover:

- setting and clearing the default board;
- setting and replacing the intake column;
- assigning, transferring, and clearing each semantic role;
- clearing intake/role metadata when a column is deleted;
- preserving unrelated profile settings and board fields; and
- displaying configuration labels accessibly.

### Gates

Run the repository's normal formatting, Go build/test, dashboard test/build, and direct MCP JSON-RPC smoke checks. After deployment is separately authorized, verify the tools through Codex and typed ChatGPT against the demo server.

## Acceptance Scenarios

1. "Add research offline sync as a task" creates a card in the configured default board and intake column after the user explicitly asks to capture it.
2. "Show active Hank tasks due this week" filters the default board by non-complete columns, tag, and due date.
3. "Move the offline sync task to Review" lists candidates, resolves exact IDs, and moves the selected card without rewriting the board.
4. Codex can select a card, move it into the appropriate active stage, perform the work, append dated results and verification, and autonomously move it to Review, Rework, Needs Human, or Complete.
5. A simultaneous dashboard edit to another card is retried safely, while a simultaneous edit to the same card returns a conflict.
6. Existing Notes Kanban boards and older MCP note tools continue to work unchanged.

## Explicit Non-Goals

- ChatGPT Voice invocation before Voice supports apps.
- A separate Hank voice client or OpenAI Realtime API integration.
- Card deletion through MCP.
- MCP creation, deletion, renaming, or reordering of boards and columns.
- Assignees, estimates, dependencies, time tracking, analytics, or a separate issue-tracker service.
- Changes to Hank iOS in this repository.
