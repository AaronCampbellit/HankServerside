# Notes Kanban Workboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a polished, persistent Notes Kanban board with real task editing, formatting, links, screenshots, and predictable movement.

**Architecture:** Keep `ProfileNotesPage` responsible for note selection, persistence, autosave, and attachment transport. Move the board interaction surface into a focused `KanbanEditor` module with immutable board helpers so behavior can be tested independently. Extend the existing backward-compatible board JSON with optional card presentation metadata rather than adding a separate task service.

**Tech Stack:** React 19, TypeScript, Vitest, Testing Library, Vite, Go protocol types, existing Notes HTTP and attachment APIs, native HTML drag/drop.

## Global Constraints

- No new JavaScript dependencies.
- Preserve the existing Notes API and `hank-note-attachment://` reference format.
- Include `board` in every Kanban save.
- All drag actions must have keyboard/touch button equivalents.
- Keep cards and columns compact enough for a laptop work surface.
- Do not push externally; deploy only to the requested Hank demo server.

---

### Task 1: Persist board data and upload attachments

**Files:**
- Modify: `web/dashboard/src/api/profileNotes.ts`
- Modify: `web/dashboard/src/api/profileNotes.test.ts`
- Modify: `internal/protocol/notes.go`
- Test: `internal/cloud/notes_http_test.go`

**Interfaces:**
- Produces: `SaveProfileNoteInput.board?: KanbanBoard | null`
- Produces: `ProfileNotesClient.uploadAttachment(noteID: string, file: File): Promise<NoteAttachment>`
- Produces: optional `KanbanCard.color` and `KanbanCard.due_date`

- [ ] **Step 1: Write failing API tests**

Assert that saving a Kanban note sends `board`, and that attachment upload sends the original binary `File`, filename query parameter, and media type to `/v1/me/notes/{id}/attachments`.

- [ ] **Step 2: Run the focused tests to prove the gap**

Run: `npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts`

Expected: the board-body and upload tests fail because those inputs are not implemented.

- [ ] **Step 3: Implement the client and protocol additions**

Add the following public shapes and transport behavior:

```ts
export type NoteAttachment = {
  id: string;
  filename: string;
  content_type: string;
  download_url: string;
  markdown_reference: string;
};

uploadAttachment(noteID: string, file: File) {
  return this.api.request<NoteAttachment>(
    `/v1/me/notes/${encodeURIComponent(noteID)}/attachments?filename=${encodeURIComponent(file.name)}`,
    { method: "POST", headers: { "Content-Type": file.type || "application/octet-stream" }, body: file },
  );
}
```

Include `board: input.board` in the JSON save body and add optional JSON fields to the Go `KanbanCard` struct.

- [ ] **Step 4: Run focused client and Go Notes tests**

Run: `npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts`

Run: `go test ./internal/cloud ./internal/protocol`

Expected: PASS.

### Task 2: Build immutable board behavior and rich card rendering

**Files:**
- Create: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Create: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`

**Interfaces:**
- Consumes: `KanbanBoard`, `KanbanCard`, and `NoteAttachment`
- Produces: `boardFromMarkdown(body: string): KanbanBoard`
- Produces: `boardToMarkdown(title: string, board: KanbanBoard): string`
- Produces: `KanbanEditor` with `board`, `attachments`, `onChange`, `onUpload`, and `confirmDelete` props

- [ ] **Step 1: Write failing helper and interaction tests**

Cover Markdown migration, stable unique IDs, card creation, card edit persistence, explicit cross-column movement, drag reordering, column add/rename/reorder/delete, search filtering, safe links, and attachment thumbnails.

- [ ] **Step 2: Run the focused test file**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanEditor.test.tsx`

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement board helpers and the editor shell**

Use immutable copies and normalize `sort_order` after every operation. Split `card.text` into its first non-empty line as the title and the remaining Markdown as the description. Render only the supported inline Markdown subset without `dangerouslySetInnerHTML`: bold, italic, bullet lines, safe links, and image references.

- [ ] **Step 4: Implement work interactions**

Add a compact board toolbar, fixed-width horizontally scrolling columns, inline add composer, real card buttons, HTML drag/drop, explicit left/right movement, column controls, search, and a labeled task-details drawer.

- [ ] **Step 5: Implement details, formatting, and media**

The drawer owns a local draft and commits title/description/due-date/color changes on input. Formatting helpers wrap the current textarea selection. Upload calls the parent transport, appends the returned canonical Markdown reference, and preserves the draft on failure.

- [ ] **Step 6: Run the focused tests**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanEditor.test.tsx`

Expected: PASS.

### Task 3: Integrate the board with Notes autosave

**Files:**
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`

**Interfaces:**
- Consumes: `KanbanEditor`, `boardFromMarkdown`, and `boardToMarkdown`
- Produces: board mutations through the existing `Editor` and 750 ms save queue

- [ ] **Step 1: Write failing integration tests**

Assert that switching to Kanban creates a usable default board, a board mutation schedules a save containing `board`, switching to text retains a Markdown representation, and a first screenshot upload saves a new note before posting bytes.

- [ ] **Step 2: Run the focused Notes tests**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/ProfileNotesPage.test.tsx`

Expected: new assertions fail against the read-only editor.

- [ ] **Step 3: Replace the read-only implementation**

Import the focused editor, remove the old renderer, store fetched attachments in `Editor`, initialize board data on conversion, pass every mutation through `updateEditor`, and include `editor.board` in `saveNote`.

- [ ] **Step 4: Make upload save-before-send**

Return the saved editor from the non-queued `saveNote` path. Before an upload, save dirty or new board state, use the resolved note ID, upload the file, and merge the returned attachment into current editor state.

- [ ] **Step 5: Run focused integration tests**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/ProfileNotesPage.test.tsx`

Expected: PASS.

### Task 4: Match the approved visual system and responsive behavior

**Files:**
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: semantic class names from `KanbanEditor.tsx`
- Produces: desktop board, overlay drawer, mobile sheet, drag/drop states, and reduced-motion behavior

- [ ] **Step 1: Implement extracted design tokens and layout**

Use existing Hank color variables plus local board variables for a 292 px column, 360 px drawer, 8-10 px radii, crisp one-pixel borders, compact 12-14 px UI type, and restrained elevation.

- [ ] **Step 2: Implement interaction states**

Style hover, focus-visible, selected, dragging, drop-target, empty, upload, disabled, due-date, and six restrained card accent variants.

- [ ] **Step 3: Implement responsive rules**

Keep horizontal columns at all widths. Overlay the drawer below 1180 px and make it a full-width sheet below 680 px. Avoid converting the board to a vertical card list.

- [ ] **Step 4: Run production frontend gates**

Run: `npm --prefix web/dashboard run check`

Expected: all Vitest tests and the Vite production build pass.

### Task 5: Full verification and demo deployment

**Files:**
- No source changes expected; fix regressions in the owning task files if found.

**Interfaces:**
- Produces: live demo proof and concept-fidelity evidence

- [ ] **Step 1: Run repository checks**

Run: `gofmt -w ./cmd ./internal`

Run: `go test ./...`

Run: `go build ./...`

Run: `git diff --check`

Expected: PASS.

- [ ] **Step 2: Commit the implementation locally**

Stage only Kanban-related source, tests, protocol, docs, and generated dashboard output required by the repo workflow. Do not push.

- [ ] **Step 3: Deploy the exact local commit to demo**

Use the `hankserverside-demo-server` workflow. Preserve `hankdemo-cloudflared`, rebuild the dashboard/cloud services, migrate before startup if required, and verify the deployed commit and bundle freshness.

- [ ] **Step 4: Run live health and doctor checks**

Run public `/healthz` and `/readyz`, remote `scripts/doctor.sh`, and relevant migration/schema checks.

- [ ] **Step 5: Exercise the rendered workflow in Browser**

The flow under test is: `/dashboard/profile-notes` -> open/create a Kanban note -> add and format a card -> attach a screenshot -> move it across columns -> reload -> observe the same board.

Check page identity, meaningful DOM, no framework overlay, console health, desktop and mobile screenshots, and persisted interaction state.

- [ ] **Step 6: Compare implementation to concept**

Use `view_image` on both the generated concept and the final browser screenshot. Inspect layout, density, palette, type, card anatomy, drawer, spacing, icons, and responsive behavior. Fix any material mismatch before handoff.
