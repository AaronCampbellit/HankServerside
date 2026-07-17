# Kanban Attachment Lifecycle and Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Kanban creation and card controls to the top, make the description surface click-to-edit, delete task-exclusive files safely, and add an admin-only Notes attachment inventory and deletion page.

**Architecture:** Keep board editing in React and attachment bytes in the existing Notes storage volume. Add reusable backend attachment-reference/deletion helpers plus an admin inventory route; let `ProfileNotesPage` serialize board persistence before per-note attachment deletion so a failed board save never removes files.

**Tech Stack:** Go 1.26 standard library HTTP/store layers, PostgreSQL through the existing store wrapper, React 19, TypeScript, Vitest/Testing Library, Vite, Docker Compose demo deployment.

## Global Constraints

- Preserve attachment bytes in `hank_note_attachments` at `/var/lib/hank/note-attachments` and metadata in `note_attachments`.
- Add no schema migration and no new dependency.
- Preserve any attachment referenced by a surviving card; uncertain references default to preservation.
- Inventory and global deletion are admin-only server-side and browser writes retain CSRF protection.
- Save the board deletion before deleting exclusive files; a failed board save deletes no files.
- Card/column confirmations name exclusive files, count and total their bytes, and report shared files as preserved.
- Physical cleanup errors are surfaced and logged without file contents or raw filesystem paths.
- Do not add bulk delete, recycle-bin, deduplication, comments, assignments, or new Kanban schema.

---

### Task 1: Centralize attachment references and deletion responses

**Files:**
- Modify: `internal/protocol/notes.go`
- Modify: `internal/cloud/note_attachments.go`
- Modify: `internal/cloud/note_attachments_test.go`
- Test: `internal/cloud/notes_api_tokens_test.go`

**Interfaces:**
- Produces: `protocol.NoteAttachmentDeleteResponse { OK bool; NoteRevision string; CleanupComplete bool }`.
- Produces: `removeNoteAttachmentReferences(note domain.UserNote, userID string, attachment domain.NoteAttachment) (domain.UserNote, int, error)` that removes Markdown and Kanban board references and returns the number removed.
- Produces: `deleteStoredNoteAttachment(ctx, note, scope, userID, attachment) (updatedNote, cleanupComplete, error)` for ordinary and admin routes.

- [ ] **Step 1: Write failing reference-removal and physical-deletion tests**

Add tests that construct a Kanban note whose `BodyMarkdown` and `BoardJSON` both contain `hank-note-attachment://natt-1`, call the new helper, and assert the reference is absent from both while unrelated cards remain. Extend an HTTP deletion test to assert the response revision is non-empty and `os.Stat(path)` returns `os.ErrNotExist`.

```go
func TestRemoveNoteAttachmentReferencesUpdatesMarkdownAndKanbanBoard(t *testing.T) {
    note := domain.UserNote{PageType: protocol.NotePageTypeKanban, BodyMarkdown: "![capture](hank-note-attachment://natt-1)", BoardJSON: `{"columns":[{"id":"inbox","cards":[{"id":"one","text":"One\n![capture](hank-note-attachment://natt-1)"},{"id":"two","text":"Keep me"}]}]}`}
    updated, removed, err := removeNoteAttachmentReferences(note, "user-1", domain.NoteAttachment{ID: "natt-1"})
    if err != nil { t.Fatal(err) }
    if removed < 2 || strings.Contains(updated.BodyMarkdown+updated.BoardJSON, "natt-1") { t.Fatalf("reference remained: %#v", updated) }
    if !strings.Contains(updated.BoardJSON, "Keep me") { t.Fatal("unrelated card changed") }
}
```

- [ ] **Step 2: Run the focused Go tests and verify RED**

Run: `go test ./internal/cloud -run 'Test(RemoveNoteAttachmentReferences|NoteAttachmentDelete)' -count=1`

Expected: FAIL because the new helper/response fields do not exist and board JSON references remain.

- [ ] **Step 3: Implement the additive response and centralized helper**

Add the protocol response:

```go
type NoteAttachmentDeleteResponse struct {
    OK              bool   `json:"ok"`
    NoteRevision    string `json:"note_revision"`
    CleanupComplete bool   `json:"cleanup_complete"`
}
```

Replace `noteWithoutAttachmentReference` with a helper that uses the existing escaped-ID regex against body Markdown and each `protocol.KanbanCard.Text` in decoded board JSON. Re-encode the board and call `revisionAndChecksum(body, pageType, boardJSON)` once.

Centralize metadata/note deletion and physical `os.Remove`. Treat `os.ErrNotExist` as cleanup-complete. Log other removal failures using attachment ID and note public ID only, and return `CleanupComplete: false` with an HTTP error after the committed database update.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/cloud -run 'Test(RemoveNoteAttachmentReferences|NoteAttachment)' -count=1`

Expected: PASS, including API-token attachment coverage.

- [ ] **Step 5: Commit the lifecycle helper**

```bash
git add internal/protocol/notes.go internal/cloud/note_attachments.go internal/cloud/note_attachments_test.go internal/cloud/notes_api_tokens_test.go
git commit -m "fix: make note attachment deletion observable"
```

### Task 2: Add the admin attachment inventory and delete route

**Files:**
- Modify: `internal/domain/notes.go`
- Modify: `internal/store/attachments.go`
- Modify: `internal/store/store_test.go`
- Create: `internal/cloud/note_attachment_admin.go`
- Create: `internal/cloud/note_attachment_admin_test.go`
- Modify: `internal/cloud/home_singleton.go`

**Interfaces:**
- Produces: `domain.NoteAttachmentInventoryRecord` containing attachment metadata, public note ID/title/scope, owner email, Markdown and board JSON used for context calculation.
- Produces: `Store.ListLiveNoteAttachmentInventory(ctx) ([]domain.NoteAttachmentInventoryRecord, error)` and `Store.GetLiveNoteAttachmentInventoryByID(ctx, attachmentID)`.
- Produces admin routes `GET /v1/home/note-attachments`, `GET /v1/home/note-attachments/{id}/content`, and `DELETE /v1/home/note-attachments/{id}`.

- [ ] **Step 1: Write failing store inventory tests**

Insert profile and home notes with live/deleted attachments. Assert the inventory returns only live rows, newest first, with note title, scope (`profile` or `home`), and owner email.

```go
items, err := db.ListLiveNoteAttachmentInventory(ctx)
if err != nil { t.Fatal(err) }
if len(items) != 2 || items[0].Attachment.DeletedAt != nil { t.Fatalf("inventory = %#v", items) }
if items[0].NoteTitle == "" || items[0].OwnerEmail == "" { t.Fatalf("missing context: %#v", items[0]) }
```

- [ ] **Step 2: Run the store test and verify RED**

Run: `go test ./internal/store -run TestListLiveNoteAttachmentInventory -count=1`

Expected: FAIL because the inventory methods/types do not exist.

- [ ] **Step 3: Implement inventory queries over existing tables**

Join `note_attachments na` to `user_notes un` and `users u`, filtering `na.deleted_at IS NULL` and `un.deleted_at IS NULL`. Derive scope with `CASE WHEN un.home_id IS NULL THEN 'profile' ELSE 'home' END`. Do not expose `storage_key` in protocol responses.

- [ ] **Step 4: Write failing handler authorization, totals, content, and deletion tests**

Create an admin and member session. Assert:

```go
member := requestJSONStatus(t, server, memberToken, http.MethodGet, "/v1/home/note-attachments", nil, http.StatusForbidden)
member.Body.Close()
requestJSON(t, server, adminToken, http.MethodGet, "/v1/home/note-attachments", nil, &inventory)
if inventory.TotalFiles != len(inventory.Attachments) || inventory.TotalBytes != 12 { t.Fatalf("bad totals: %#v", inventory) }
requestJSON(t, server, adminToken, http.MethodDelete, "/v1/home/note-attachments/natt-1", nil, &deleted)
```

After DELETE, assert the item disappears, its physical file is gone, and its references are absent from body and board JSON. Assert content GET is admin-only and returns inline image bytes.

- [ ] **Step 5: Run the handler tests and verify RED**

Run: `go test ./internal/cloud -run TestAdminNoteAttachment -count=1`

Expected: FAIL with 404 because the route is not registered.

- [ ] **Step 6: Implement the admin handler and protocol payload**

Define private response types in the handler unless reused by another package:

```go
type adminNoteAttachmentItem struct {
    ID string `json:"id"`; Filename string `json:"filename"`; ContentType string `json:"content_type"`
    SizeBytes int64 `json:"size_bytes"`; CreatedAt time.Time `json:"created_at"`
    NoteID string `json:"note_id"`; NoteTitle string `json:"note_title"`; NoteScope string `json:"note_scope"`
    OwnerEmail string `json:"owner_email"`; ReferenceCount int `json:"reference_count"`
    Contexts []string `json:"contexts"`; DownloadURL string `json:"download_url"`
}
```

Resolve the singleton membership already supplied to `handleHomeSubroutes`, require `domain.HomeRoleAdmin`, calculate contexts/reference counts without double-counting Kanban body projection, and reuse Task 1 deletion/serve helpers. Register `handleHomeNoteAttachmentsAdmin` before storage/audit handlers.

- [ ] **Step 7: Run store and cloud tests and verify GREEN**

Run: `go test ./internal/store ./internal/cloud -run 'Test(ListLiveNoteAttachmentInventory|AdminNoteAttachment)' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit the admin API**

```bash
git add internal/domain/notes.go internal/store/attachments.go internal/store/store_test.go internal/cloud/note_attachment_admin.go internal/cloud/note_attachment_admin_test.go internal/cloud/home_singleton.go
git commit -m "feat: add admin note attachment inventory"
```

### Task 3: Build the admin Settings attachment manager

**Files:**
- Create: `web/dashboard/src/api/noteAttachments.ts`
- Create: `web/dashboard/src/api/noteAttachments.test.ts`
- Create: `web/dashboard/src/settings/AttachmentsSettings.tsx`
- Create: `web/dashboard/src/settings/AttachmentsSettings.test.tsx`
- Modify: `web/dashboard/src/ui/navConfig.ts`
- Modify: `web/dashboard/src/settings/SettingsLayout.tsx`
- Modify: `web/dashboard/src/settings/SettingsLayout.test.tsx`
- Modify: `web/dashboard/src/router.ts`
- Modify: `web/dashboard/src/router.test.ts`
- Modify: `web/dashboard/src/App.tsx`
- Modify: `web/dashboard/src/App.test.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Produces: `NoteAttachmentInventory`, `NoteAttachmentInventoryItem`, and `noteAttachmentsClient.load()/remove(id)`.
- Consumes: Task 2 JSON routes and `useConfirmDialog`.

- [ ] **Step 1: Write failing API client tests**

```ts
it("loads and deletes admin note attachments", async () => {
  const request = vi.fn(async () => ({ total_files: 1, total_bytes: 2048, attachments: [{ id: "natt-1" }] }))
  const client = new NoteAttachmentsClient(testTransport(request))
  expect((await client.load()).attachments).toHaveLength(1)
  await client.remove("natt-1")
  expect(request).toHaveBeenLastCalledWith("/v1/home/note-attachments/natt-1", { method: "DELETE" })
})
```

- [ ] **Step 2: Run the API test and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/api/noteAttachments.test.ts`

Expected: FAIL because the client module does not exist.

- [ ] **Step 3: Implement the focused API client**

Normalize `attachments` with `arrayFrom`; keep server totals numeric; URL-encode attachment IDs; rely on the shared transport for cookie/CSRF behavior.

- [ ] **Step 4: Write failing Settings navigation and page tests**

Assert the admin rail contains `Attachments ADMIN`, non-admin rail omits it, `/dashboard/settings/attachments` resolves as admin-only, and the page renders `1 file`, `2.0 KB`, note/owner context, Open, and Delete. Confirm deletion copy contains filename, size, note title, and “removed everywhere”. After confirmation, assert the row and totals refresh.

- [ ] **Step 5: Run Settings tests and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/settings/AttachmentsSettings.test.tsx src/settings/SettingsLayout.test.tsx src/router.test.ts src/App.test.tsx`

Expected: FAIL because the route, tab, and component do not exist.

- [ ] **Step 6: Implement the page and navigation**

Use the existing Settings loading/error/panel patterns. Render two summary cards and a responsive row list. Use a local byte formatter:

```ts
function formatBytes(value = 0) {
  if (value < 1024) return `${value} B`
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MB`
  return `${(value / 1024 ** 3).toFixed(1)} GB`
}
```

On Delete, call `dialog.confirm`, then `remove`, then reload the inventory with a success message. Mark `reference_count === 0` as `Unreferenced`.

- [ ] **Step 7: Run focused Settings tests and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/api/noteAttachments.test.ts src/settings/AttachmentsSettings.test.tsx src/settings/SettingsLayout.test.tsx src/router.test.ts src/App.test.tsx`

Expected: PASS.

- [ ] **Step 8: Commit the Settings manager**

```bash
git add web/dashboard/src/api/noteAttachments.ts web/dashboard/src/api/noteAttachments.test.ts web/dashboard/src/settings/AttachmentsSettings.tsx web/dashboard/src/settings/AttachmentsSettings.test.tsx web/dashboard/src/ui/navConfig.ts web/dashboard/src/settings/SettingsLayout.tsx web/dashboard/src/settings/SettingsLayout.test.tsx web/dashboard/src/router.ts web/dashboard/src/router.test.ts web/dashboard/src/App.tsx web/dashboard/src/App.test.tsx web/dashboard/src/styles.css
git commit -m "feat: add note attachment settings manager"
```

### Task 4: Reorder Kanban controls and make descriptions click-to-edit

**Files:**
- Modify: `web/dashboard/src/dashboard/KanbanCardModal.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanCardModal.test.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Preserves existing modal callbacks.
- Produces a top options section and a click-to-edit description preview.

- [ ] **Step 1: Write failing layout and interaction tests**

Assert the Add task button precedes `.kanban-card-stack`; the modal options section precedes the Description heading; no `.kanban-card-modal-footer` exists; Delete is in the top options section; clicking preview whitespace shows/focuses the textarea; and clicking a preview link does not enter edit mode.

```ts
fireEvent.click(screen.getByTestId("kanban-description-preview"))
expect(screen.getByLabelText("Description")).toHaveFocus()
```

- [ ] **Step 2: Run focused component tests and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanCardModal.test.tsx src/dashboard/KanbanEditor.test.tsx`

Expected: FAIL on DOM order, absent preview test ID, and footer placement.

- [ ] **Step 3: Implement the layout changes**

Move the Add task/composer block before `.kanban-card-stack`. In the modal, render metadata/color/movement/Delete directly after the title header and remove the footer. Add a preview click handler that ignores targets within `a`, `button`, or `[data-kanban-no-edit]`, otherwise sets edit mode. Keep the explicit Edit/Preview control.

- [ ] **Step 4: Adjust responsive styles**

Make the top options grid wrap at narrow widths, keep destructive Delete visible, remove obsolete sticky-footer rules, and retain the current modal scroll boundary.

- [ ] **Step 5: Run focused component tests and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanCardModal.test.tsx src/dashboard/KanbanEditor.test.tsx`

Expected: PASS.

- [ ] **Step 6: Commit the Kanban layout polish**

```bash
git add web/dashboard/src/dashboard/KanbanCardModal.tsx web/dashboard/src/dashboard/KanbanCardModal.test.tsx web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx web/dashboard/src/styles.css
git commit -m "feat: surface Kanban creation and card controls"
```

### Task 5: Add attachment-aware task and column deletion orchestration

**Files:**
- Create: `web/dashboard/src/dashboard/kanbanAttachments.ts`
- Create: `web/dashboard/src/dashboard/kanbanAttachments.test.ts`
- Modify: `web/dashboard/src/api/profileNotes.ts`
- Modify: `web/dashboard/src/api/profileNotes.test.ts`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`

**Interfaces:**
- Produces: `attachmentDeletionPlan(board, removedCardIDs, attachments): { exclusive: NoteAttachment[]; shared: NoteAttachment[] }`.
- Produces: `ProfileNotesClient.deleteAttachment(noteID, attachmentID): Promise<{ ok: boolean; note_revision: string; cleanup_complete: boolean }>`.
- Produces: `KanbanEditorProps.onDeleteItems(nextBoard, exclusiveAttachments): Promise<boolean>`.

- [ ] **Step 1: Write failing pure planning tests**

Cover one exclusive reference, one reference reused by a surviving card, duplicate references in a removed card, an unknown attachment ID, and column deletion with multiple cards. Unknown IDs must not enter the exclusive deletion list.

```ts
expect(attachmentDeletionPlan(board, new Set(["one"]), attachments)).toEqual({
  exclusive: [attachments[0]],
  shared: [attachments[1]],
})
```

- [ ] **Step 2: Run the helper test and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/kanbanAttachments.test.ts`

Expected: FAIL because the helper does not exist.

- [ ] **Step 3: Implement reference classification and prompt formatting**

Parse canonical IDs with the same `hank-note-attachment://` shape used by `KanbanRichText`. Use sets for unique IDs. Add a byte formatter and build confirmation text that includes filenames, exclusive total, shared count, and the permanent warning.

- [ ] **Step 4: Write failing API and editor deletion tests**

Assert `deleteAttachment` uses `DELETE /v1/me/notes/{noteID}/attachments/{attachmentID}`. In `KanbanEditor`, assert the task and column prompts describe attachments, `onDeleteItems` receives the next board and exclusive list, and a false result leaves the current UI unchanged.

- [ ] **Step 5: Run API/editor tests and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts src/dashboard/KanbanEditor.test.tsx`

Expected: FAIL because the API method/callback do not exist and current deletion commits immediately.

- [ ] **Step 6: Implement editor deletion callbacks**

Make task and column deletion build `nextBoard`, request confirmation, await `onDeleteItems`, and close/remove UI only on success. Disable repeated destructive actions while the promise is pending.

- [ ] **Step 7: Write failing ProfileNotes serialization tests**

Mock the API so `saveNote` and attachment DELETE calls record order. Assert save occurs first, revisions from each DELETE feed the next client state, successful items leave `editor.attachments`, and a failed save produces zero DELETE calls and keeps the card.

- [ ] **Step 8: Run the Profile Notes test and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/ProfileNotesPage.test.tsx`

Expected: FAIL because the parent callback is absent.

- [ ] **Step 9: Implement serialized save-then-delete in `ProfileNotesPage`**

Add `deleteKanbanItems(nextBoard, attachments)` that:

```ts
clearAutosaveTimer()
const current = latestEditorRef.current
const next = { ...current, board: nextBoard, body: boardToMarkdown(current.title, nextBoard) }
const saved = await saveNote(next, true)
if (!saved) return false
let revision = saved.revision
const remaining = [...saved.attachments]
for (const attachment of attachments) {
  const response = await profileNotesClient.deleteAttachment(saved.noteID, attachment.id)
  revision = response.note_revision || revision
  remaining.splice(remaining.findIndex((item) => item.id === attachment.id), 1)
}
const finalEditor = { ...saved, revision, attachments: remaining }
latestEditorRef.current = finalEditor
latestSavedEditorRef.current = finalEditor
setReady({ editor: finalEditor, savedEditor: finalEditor })
return true
```

Catch post-save cleanup errors, keep the saved board, retain failed attachments, show a cleanup warning, and return true because the task deletion itself succeeded.

- [ ] **Step 10: Run all focused Notes/Kanban tests and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/api/profileNotes.test.ts src/dashboard/kanbanAttachments.test.ts src/dashboard/KanbanEditor.test.tsx src/dashboard/ProfileNotesPage.test.tsx`

Expected: PASS.

- [ ] **Step 11: Commit the deletion workflow**

```bash
git add web/dashboard/src/dashboard/kanbanAttachments.ts web/dashboard/src/dashboard/kanbanAttachments.test.ts web/dashboard/src/api/profileNotes.ts web/dashboard/src/api/profileNotes.test.ts web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx web/dashboard/src/dashboard/ProfileNotesPage.tsx web/dashboard/src/dashboard/ProfileNotesPage.test.tsx
git commit -m "feat: delete task-exclusive note attachments"
```

### Task 6: Broad verification, demo deployment, and rendered QA

**Files:**
- Modify only files required by failures found during verification.

**Interfaces:**
- Consumes every prior task and produces deployment/QA evidence.

- [ ] **Step 1: Run formatting and static checks**

Run:

```bash
gofmt -w ./cmd ./internal
git diff --check
npm --prefix web/dashboard run check
```

Expected: 0 exits; existing jsdom canvas/navigation notices and the Vite chunk-size warning may remain informational.

- [ ] **Step 2: Run all Go verification**

Run:

```bash
go test ./...
go build ./...
make migrate-status
make schema-drift-check
```

Expected: PASS and no migration/schema drift because no schema changed.

- [ ] **Step 3: Commit any verification-only fixes**

Stage only scoped files and commit with a message describing the actual fix. Leave `docs/superpowers/plans/2026-07-17-kanban-mcp-card-operations.md` untouched.

- [ ] **Step 4: Deploy the exact branch commit to the demo server**

Follow the `hankserverside-demo-server` skill: transfer a Git bundle, fast-forward `/home/campbellservers/HankServerside`, rebuild cloud/agent, force-recreate both containers, and verify exact commit parity.

- [ ] **Step 5: Prove server health and storage behavior**

Run remote `scripts/doctor.sh`, public `/healthz`, `/readyz`, and an authenticated test flow that uploads a disposable attachment, deletes its task, verifies the attachment GET returns 404, and verifies the file is absent from `/var/lib/hank/note-attachments` inside the cloud container.

- [ ] **Step 6: Run Browser UI QA**

The flow under test is: Notes Kanban -> Add task at top -> open card -> click description surface -> edit -> inspect top card actions -> delete a disposable task with attachment warning -> Settings > Attachments -> verify totals/list -> delete a disposable attachment -> reload and confirm persistence.

Check desktop and narrow viewport, page identity, meaningful DOM, no framework overlay, error/warn logs, interaction state, and screenshot evidence. Preserve user data by using disposable QA notes/files and remove them when finished.

- [ ] **Step 7: Run fresh completion verification**

Re-run `npm --prefix web/dashboard run check`, `go test ./...`, `go build ./...`, `git diff --check`, exact local/remote HEAD comparison, `/readyz`, and `scripts/doctor.sh` immediately before reporting completion.
