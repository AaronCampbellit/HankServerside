# Kanban Card Modal Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Kanban dragging movement-only, replace the right drawer with an enlarged card modal, and support direct inline screenshot pasting.

**Architecture:** `KanbanEditor` remains the board and persistence-state owner. A new `KanbanCardModal` renders the accessible enlarged-card editor and reports edits through explicit callbacks. Drag suppression and movement intent stay in the board; clipboard/file/drop uploads share one canonical attachment-insertion path.

**Tech Stack:** React 19, TypeScript, Vitest, Testing Library, CSS, existing profile Notes attachment API and serialized autosave.

## Global Constraints

- A native card drag never opens the editor, including after the drop completes.
- Only deliberate click or keyboard activation opens the editor.
- Use the existing Kanban board JSON and `hank-note-attachment://` Markdown contract; add no schema, migration, route, dependency, or save transaction.
- The modal must be keyboard accessible, restore card focus on close, and become near-full-screen on narrow screens.
- Image clipboard paste is intercepted; ordinary text and link paste retain native behavior.
- Keep the existing file picker and file-drop paths and route all uploads through the same insertion helper.
- Preserve the Notes page's 750 ms serialized background autosave.

---

### Task 1: Separate card movement from card selection

**Files:**
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Test: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`

**Interfaces:**
- Produces: `moveCard(location, targetColumnID, targetIndex?, preserveEditor?)`, where a closed-card move never calls `setSelected` and `preserveEditor` updates the location of an already-open card.
- Produces: a drag-suppression ref that rejects the browser click dispatched immediately after `dragend`.

- [ ] **Step 1: Write failing movement-only tests**

Add tests that drop `Review brief` into `In progress`, assert its new column, and assert that no dialog exists. Then dispatch `dragStart`, `dragEnd`, and `click` on the card open control and assert the dialog remains closed.

```tsx
it("moves a dragged card without opening its editor", () => {
  Harness({});
  const card = screen.getByRole("button", { name: "Open task Review brief" }).closest("article")!;
  const target = screen.getByRole("heading", { name: "In progress" }).closest("section")!;
  const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

  fireEvent.dragStart(card, { dataTransfer });
  fireEvent.drop(target, { dataTransfer });
  fireEvent.dragEnd(card, { dataTransfer });

  expect(target).toHaveTextContent("Review brief");
  expect(screen.queryByRole("dialog", { name: "Task details" })).not.toBeInTheDocument();
});

it("suppresses the click emitted after dragging", () => {
  Harness({});
  const open = screen.getByRole("button", { name: "Open task Review brief" });
  const card = open.closest("article")!;
  const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

  fireEvent.dragStart(card, { dataTransfer });
  fireEvent.dragEnd(card, { dataTransfer });
  fireEvent.click(open);

  expect(screen.queryByRole("dialog", { name: "Task details" })).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanEditor.test.tsx`

Expected: FAIL because `moveCard` selects every moved card and the post-drag click opens the current drawer.

- [ ] **Step 3: Implement movement intent and click suppression**

Change movement and card-open handling to this shape:

```tsx
const suppressOpenRef = useRef(false);
const dragReleaseTimerRef = useRef<number | null>(null);

function moveCard(
  location: CardLocation,
  targetColumnID: string,
  targetIndex = Number.MAX_SAFE_INTEGER,
  preserveEditor = selected?.cardID === location.cardID,
) {
  // Existing immutable removal/insertion logic.
  commit({ columns: nextColumns });
  if (preserveEditor) setSelected({ columnID: targetColumnID, cardID: location.cardID });
}

function beginCardDrag(location: CardLocation, event: DragEvent<HTMLElement>) {
  suppressOpenRef.current = true;
  setDragging(location);
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData("text/plain", location.cardID);
}

function endCardDrag() {
  setDragging(null);
  setDropTarget("");
  if (dragReleaseTimerRef.current !== null) window.clearTimeout(dragReleaseTimerRef.current);
  dragReleaseTimerRef.current = window.setTimeout(() => { suppressOpenRef.current = false; }, 0);
}

function openCard(location: CardLocation) {
  if (suppressOpenRef.current) return;
  setSelected(location);
}
```

Add effect cleanup for `dragReleaseTimerRef`. Pass `false` from board drag/drop and closed-card movement buttons; let modal movement preserve the open editor.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanEditor.test.tsx`

Expected: all Kanban editor tests PASS.

- [ ] **Step 5: Commit movement separation**

```bash
git add web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx
git commit -m "fix: keep Kanban drag movement-only"
```

---

### Task 2: Replace the drawer with an accessible enlarged-card modal

**Files:**
- Create: `web/dashboard/src/dashboard/KanbanCardModal.tsx`
- Create: `web/dashboard/src/dashboard/KanbanCardModal.test.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Modify: `web/dashboard/src/styles.css`
- Test: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`

**Interfaces:**
- Produces: exported `KanbanCardModal` component.
- Consumes: `KanbanCard`, `KanbanColumn[]`, `NoteAttachment[]`, current column ID, title, description, upload state, and explicit edit/move/upload/close/delete callbacks.
- Produces: `onUploadFiles(files: File[], selection?: { start: number; end: number }): Promise<void>` for Task 3.

- [ ] **Step 1: Write failing modal behavior tests**

Create component tests proving backdrop and Escape close, clicks inside do not close, title receives initial focus, Tab wraps from the final control to the first, and the dialog exposes `aria-modal="true"`.

```tsx
it("closes from Escape and the backdrop but not an inside click", () => {
  const onClose = vi.fn();
  const { rerender } = render(<KanbanCardModal {...modalProps({ onClose })} />);
  const dialog = screen.getByRole("dialog", { name: "Task details" });

  expect(dialog).toHaveAttribute("aria-modal", "true");
  fireEvent.click(dialog);
  expect(onClose).not.toHaveBeenCalled();
  fireEvent.keyDown(dialog, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(1);

  onClose.mockClear();
  rerender(<KanbanCardModal {...modalProps({ onClose })} />);
  fireEvent.mouseDown(screen.getByTestId("kanban-card-modal-backdrop"));
  expect(onClose).toHaveBeenCalledTimes(1);
});
```

Update the board test to require a deliberate click to render a modal-shaped dialog and to verify moving the open card right keeps the dialog visible.

- [ ] **Step 2: Run modal tests and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanCardModal.test.tsx src/dashboard/KanbanEditor.test.tsx`

Expected: FAIL because `KanbanCardModal` does not exist and the current editor is a non-modal right-side drawer.

- [ ] **Step 3: Implement the focused modal component**

Create a component with this public contract:

```tsx
export type DescriptionSelection = { start: number; end: number };

export type KanbanCardModalProps = {
  card: KanbanCard;
  title: string;
  description: string;
  columnID: string;
  columns: KanbanColumn[];
  attachments: NoteAttachment[];
  uploading: boolean;
  uploadError: string;
  onTitleChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
  onFormat: (prefix: string, suffix?: string, placeholder?: string) => void;
  onMove: (columnID: string) => void;
  onDueDateChange: (value: string) => void;
  onColorChange: (value: string) => void;
  onUploadFiles: (files: File[], selection?: DescriptionSelection) => Promise<void>;
  onMoveLeft: () => void;
  onMoveRight: () => void;
  canMoveLeft: boolean;
  canMoveRight: boolean;
  onDelete: () => void;
  onClose: () => void;
};
```

Render a `.kanban-card-modal-backdrop` containing an accent-colored `.kanban-card-modal`, use `role="dialog" aria-modal="true" aria-label="Task details"`, stop backdrop propagation inside the card, autofocus the title, and implement a small focus-wrap handler over enabled buttons, inputs, selects, textareas, and links.

- [ ] **Step 4: Integrate modal state and focus restoration**

In `KanbanEditor`, replace the drawer markup with `KanbanCardModal`. Track open-card buttons by card ID and restore focus after close:

```tsx
const cardButtonRefs = useRef(new Map<string, HTMLButtonElement>());

function closeCard() {
  const cardID = selected?.cardID;
  setSelected(null);
  requestAnimationFrame(() => cardButtonRefs.current.get(cardID || "")?.focus());
}
```

Modal column selection and left/right movement call `moveCard(..., true)` so the modal remains open at the new location.

- [ ] **Step 5: Replace drawer CSS with modal-card CSS**

Remove `.details-open` grid-column behavior and drawer positioning. Add a fixed backdrop, centered card with a 720 px maximum width, 86 vh maximum height, accent left border, scrollable body, fixed header/footer, and a near-full-screen `calc(100vw - 20px)` by `calc(100vh - 20px)` mobile treatment below 680 px. Preserve existing field, toolbar, color, upload, and footer visual tokens under modal class names.

- [ ] **Step 6: Run modal and board tests and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanCardModal.test.tsx src/dashboard/KanbanEditor.test.tsx`

Expected: both files PASS; existing formatting, metadata, move, and upload tests remain green.

- [ ] **Step 7: Commit modal editor**

```bash
git add web/dashboard/src/dashboard/KanbanCardModal.tsx web/dashboard/src/dashboard/KanbanCardModal.test.tsx web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx web/dashboard/src/styles.css
git commit -m "feat: edit Kanban tasks in card modal"
```

---

### Task 3: Add canonical inline screenshot pasting

**Files:**
- Modify: `web/dashboard/src/dashboard/KanbanCardModal.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanCardModal.test.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: `onUploadFiles(files, selection)` from Task 2.
- Produces: one parent upload/insertion function shared by clipboard paste, picker change, and drop.

- [ ] **Step 1: Write failing paste tests**

Add a modal test where clipboard items contain an image and verify default is prevented and `onUploadFiles` receives the file plus the textarea selection. Add a plain-text clipboard test that verifies default is not prevented. Add a board test with two uploaded images where the second rejects; assert the first canonical reference remains and the error is visible. Extend the existing `ProfileNotesPage` attachment-autosave test so a pasted image must reach `uploadAttachment` and the later 750 ms `saveNote` board payload.

```tsx
it("routes pasted images through inline upload at the caret", () => {
  const onUploadFiles = vi.fn(async () => undefined);
  render(<KanbanCardModal {...modalProps({ onUploadFiles, description: "Before after" })} />);
  const description = screen.getByLabelText("Description") as HTMLTextAreaElement;
  description.setSelectionRange(7, 7);
  const image = new File(["png"], "capture.png", { type: "image/png" });
  const preventDefault = vi.fn();

  fireEvent.paste(description, {
    clipboardData: { items: [{ kind: "file", type: "image/png", getAsFile: () => image }] },
    preventDefault,
  });

  expect(onUploadFiles).toHaveBeenCalledWith([image], { start: 7, end: 7 });
});
```

- [ ] **Step 2: Run paste tests and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanCardModal.test.tsx src/dashboard/KanbanEditor.test.tsx src/dashboard/ProfileNotesPage.test.tsx`

Expected: FAIL because the description has no image-paste handler, upload accepts only one file appended at the end, and pasted images do not yet reach Notes autosave.

- [ ] **Step 3: Implement clipboard image extraction**

In `KanbanCardModal`, add:

```tsx
function pastedImages(event: ClipboardEvent<HTMLTextAreaElement>): File[] {
  return Array.from(event.clipboardData.items)
    .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file));
}

function handleDescriptionPaste(event: ClipboardEvent<HTMLTextAreaElement>) {
  const files = pastedImages(event);
  if (!files.length) return;
  event.preventDefault();
  const input = event.currentTarget;
  void onUploadFiles(files, { start: input.selectionStart, end: input.selectionEnd });
}
```

Picker change and drop convert `FileList` to `File[]` and call the same callback without a selection.

- [ ] **Step 4: Implement ordered canonical insertion and partial failure**

Replace the single-file parent upload function with a sequential helper. For each successful upload, derive the latest description from the card inside the functional updater, insert the canonical reference at the bounded insertion index, and advance that index. Keep successful references if a later upload fails.

```tsx
async function uploadFiles(files: File[], selection?: DescriptionSelection) {
  if (!selected || !files.length) return;
  setUploading(true);
  setUploadError("");
  let insertion = selection?.start;
  try {
    for (const file of files) {
      const attachment = await onUpload(file);
      const reference = attachment.markdown_reference
        || `![${attachment.filename}](hank-note-attachment://${attachment.id})`;
      updateCard(selected, (card) => {
        const parts = cardTitleAndDescription(card);
        const start = Math.min(insertion ?? parts.description.length, parts.description.length);
        const end = Math.min(selection?.end ?? start, parts.description.length);
        const separator = start > 0 && parts.description[start - 1] !== "\n" ? "\n\n" : "";
        const inserted = `${separator}${reference}`;
        insertion = start + inserted.length;
        selection = { start: insertion, end: insertion };
        const description = `${parts.description.slice(0, start)}${inserted}${parts.description.slice(end)}`;
        return { ...card, text: cardText(parts.title, description) };
      });
    }
  } catch (error) {
    setUploadError(error instanceof Error ? error.message : "File could not be uploaded.");
  } finally {
    setUploading(false);
  }
}
```

After completion, the modal focuses the description and places its caret after the inserted reference on the next render.

- [ ] **Step 5: Render inline modal media**

Filter `attachments` to references present in the current description and render image attachments in a `.kanban-card-modal-media` grid before the file list. Each image uses its `download_url`, filename alt text, and a link to the original. The collapsed card continues using the existing Markdown renderer.

- [ ] **Step 6: Run paste tests and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/KanbanCardModal.test.tsx src/dashboard/KanbanEditor.test.tsx src/dashboard/ProfileNotesPage.test.tsx`

Expected: image paste, ordinary paste, multiple upload ordering, partial failure, picker, drop, preview, Notes autosave, and existing card tests PASS.

- [ ] **Step 7: Commit paste support**

```bash
git add web/dashboard/src/dashboard/KanbanCardModal.tsx web/dashboard/src/dashboard/KanbanCardModal.test.tsx web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx web/dashboard/src/dashboard/ProfileNotesPage.test.tsx web/dashboard/src/styles.css
git commit -m "feat: paste screenshots into Kanban cards"
```

---

### Task 4: Run repository and live demo acceptance

**Files:**
- No production files expected.

**Interfaces:**
- Consumes: final feature-branch commit and demo-server workflow.
- Produces: exact source parity, current dashboard assets, live interaction evidence, and health evidence.

- [ ] **Step 1: Run repository verification**

Run:

```bash
go test ./...
go build ./...
npm --prefix web/dashboard run check
git diff --check
git status --short --branch
```

Expected: all commands exit 0 and the feature branch is clean.

- [ ] **Step 2: Deploy the exact feature branch to the demo**

Use the demo-server skill's connection-first workflow, transfer the exact commit without pushing GitHub, fast-forward `/home/campbellservers/HankServerside` to it, rebuild the dashboard/cloud image, and restart only the affected services.

- [ ] **Step 3: Prove service and asset freshness**

Run `scripts/doctor.sh`, verify `/healthz` and `/readyz`, confirm the remote branch commit equals local `HEAD`, and verify the public page references the newly built JS/CSS assets.

Expected: doctor reports 0 failures and 0 warnings; readiness is `ok`; one agent is online; remote and local commits match; new asset names are served.

- [ ] **Step 4: Exercise the live browser flow**

The flow under test is: `/dashboard/profile-notes` -> drag a closed card across columns -> confirm no dialog -> deliberately click the card -> edit in the enlarged modal -> paste a screenshot into the description -> confirm inline preview -> close -> reload -> confirm movement, text, and screenshot persist.

Required evidence: page identity, meaningful DOM, no framework overlay, no relevant console errors/warnings, desktop screenshot, narrow-width screenshot, and interaction state checks after each target action.

- [ ] **Step 5: Record final branch state**

Run:

```bash
git status --short --branch
git log -5 --oneline --decorate
```

Expected: clean `codex/notes-kanban-workboard`, no push or merge, and demo remote at the exact final commit.
