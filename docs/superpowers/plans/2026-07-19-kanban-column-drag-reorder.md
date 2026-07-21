# Kanban Column Drag Reordering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add direct drag-and-drop reordering for Notes Kanban columns while retaining the existing left and right controls.

**Architecture:** Extend the native HTML drag handling already owned by `KanbanEditor` with separate column drag state and a column-specific data-transfer type. Reorder the existing column objects through the current `commit`/`normalizeBoard` path so sequential `sort_order` values and autosave behavior remain unchanged.

**Tech Stack:** React 19, TypeScript, native HTML Drag and Drop, Vitest, Testing Library, CSS.

## Global Constraints

- Keep the change dashboard-only; do not change APIs, protocol messages, persistence types, MCP tools, or database schema.
- Make only a dedicated header grip draggable; do not make the entire column header draggable.
- Retain the existing named left and right column buttons as the keyboard-accessible fallback.
- Keep card dragging behavior independent from column dragging.
- Preserve every column object and its cards, role, intake identity, and other metadata while changing only order and normalized `sort_order` values.
- Add no dependency.
- Do not commit, push, deploy, or modify the Hank Build board without explicit user authorization.
- Preserve the unrelated untracked `.codex/` directory.

---

### Task 1: Column drag behavior

**Files:**
- Modify: `web/dashboard/src/dashboard/KanbanEditor.test.tsx:157-195`
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx:15-20, 165-180, 229-236, 288-318, 427-460, 502-515`

**Interfaces:**
- Consumes: `KanbanEditorProps.board`, `KanbanEditorProps.onChange`, existing `commit(next: KanbanBoard)`, and normalized ordered columns.
- Produces: a button named `Drag <column title> column`, `application/x-hank-kanban-column` drag data, and an `onChange` board whose `columns` reflect the dropped order.
- Preserves: the existing `moveColumn(columnID, direction)` and card drag functions as independent fallback and card movement paths.

- [ ] **Step 1: Write the failing column reorder test**

Add this test after `places the add task action beside the column controls`:

```tsx
it("reorders columns by dragging a dedicated header grip", () => {
  const { change } = Harness({});
  const grip = screen.getByRole("button", { name: "Drag Inbox column" });
  const target = screen.getByRole("heading", { name: "Done" }).closest("section")!;
  const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

  fireEvent.dragStart(grip, { dataTransfer });
  expect(grip).toHaveAttribute("aria-grabbed", "true");
  fireEvent.dragOver(target, { dataTransfer });
  expect(target).toHaveClass("is-column-drop-target");
  fireEvent.drop(target, { dataTransfer });

  const latest = change.mock.calls.at(-1)?.[0];
  expect(dataTransfer.setData).toHaveBeenCalledWith("application/x-hank-kanban-column", "inbox");
  expect(latest?.columns?.map((column) => column.id)).toEqual(["doing", "done", "inbox"]);
  expect(latest?.columns?.map((column) => column.sort_order)).toEqual([0, 1, 2]);
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
npm --prefix web/dashboard test -- --run src/dashboard/KanbanEditor.test.tsx
```

Expected: FAIL because no button named `Drag Inbox column` exists.

- [ ] **Step 3: Add separate column drag state and mutation functions**

In `KanbanEditor.tsx`, add a column data type beside the existing location type:

```tsx
const columnDragDataType = "application/x-hank-kanban-column";
```

Add state beside the existing card drag state:

```tsx
const [draggingColumnID, setDraggingColumnID] = useState("");
const [columnDropTargetID, setColumnDropTargetID] = useState("");
```

Keep `moveColumn` unchanged as the button path. Add these functions after it:

```tsx
function reorderColumn(columnID: string, targetColumnID: string) {
  const sourceIndex = columns.findIndex((column) => column.id === columnID);
  const targetIndex = columns.findIndex((column) => column.id === targetColumnID);
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return;
  const next = [...columns];
  const [moved] = next.splice(sourceIndex, 1);
  next.splice(targetIndex, 0, moved);
  commit({ columns: next });
}

function beginColumnDrag(columnID: string, event: DragEvent<HTMLElement>) {
  setDraggingColumnID(columnID);
  setColumnDropTargetID("");
  setDragging(null);
  setDropTarget("");
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData(columnDragDataType, columnID);
}

function dropColumn(event: DragEvent, targetColumnID: string) {
  event.preventDefault();
  if (draggingColumnID) reorderColumn(draggingColumnID, targetColumnID);
  endColumnDrag();
}

function endColumnDrag() {
  setDraggingColumnID("");
  setColumnDropTargetID("");
}
```

- [ ] **Step 4: Wire the dedicated grip and disambiguated drop handlers**

Extend the column section class and event handlers:

```tsx
<section
  className={`kanban-column${dropTarget === column.id ? " is-drop-target" : ""}${draggingColumnID === column.id ? " is-column-dragging" : ""}${columnDropTargetID === column.id ? " is-column-drop-target" : ""}`}
  key={column.id}
  aria-labelledby={`kanban-column-${column.id}`}
  onDragOver={(event) => {
    event.preventDefault();
    if (draggingColumnID) {
      setColumnDropTargetID(column.id || "");
      setDropTarget("");
      return;
    }
    setDropTarget(column.id || "");
  }}
  onDragLeave={(event) => {
    if (event.currentTarget.contains(event.relatedTarget as Node)) return;
    if (draggingColumnID) setColumnDropTargetID("");
    else setDropTarget("");
  }}
  onDrop={(event) => draggingColumnID
    ? dropColumn(event, column.id || "")
    : dropCard(event, column.id || "", cards.length)}
>
```

Add the grip as the first item in `.kanban-column-actions`:

```tsx
<button
  className="kanban-column-grip"
  type="button"
  draggable
  aria-label={`Drag ${column.title} column`}
  aria-grabbed={draggingColumnID === column.id}
  onDragStart={(event) => beginColumnDrag(column.id || "", event)}
  onDragEnd={endColumnDrag}
>
  <SmallIcon name="grip" />
</button>
```

Allow column drops that originate over a card to bubble to the column section while preserving card drops:

```tsx
onDragOver={(event) => { if (!draggingColumnID) event.preventDefault(); }}
onDrop={(event) => {
  if (draggingColumnID) return;
  event.stopPropagation();
  dropCard(event, column.id || "", cardIndex);
}}
```

- [ ] **Step 5: Run the focused test and verify GREEN**

Run:

```bash
npm --prefix web/dashboard test -- --run src/dashboard/KanbanEditor.test.tsx
```

Expected: PASS, including the pre-existing card drag tests.

- [ ] **Step 6: Write the failing self-drop test**

Add:

```tsx
it("does not change the board when a column is dropped on itself", () => {
  const { change } = Harness({});
  const grip = screen.getByRole("button", { name: "Drag Inbox column" });
  const inbox = screen.getByRole("heading", { name: "Inbox" }).closest("section")!;
  const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

  fireEvent.dragStart(grip, { dataTransfer });
  fireEvent.drop(inbox, { dataTransfer });

  expect(change).not.toHaveBeenCalled();
  expect(inbox).not.toHaveClass("is-column-drop-target");
});
```

- [ ] **Step 7: Run the focused test and verify self-drop behavior**

Run:

```bash
npm --prefix web/dashboard test -- --run src/dashboard/KanbanEditor.test.tsx
```

Expected: PASS because `reorderColumn` rejects equal source and target indexes and `endColumnDrag` clears transient state.

- [ ] **Step 8: Review Task 1 without committing**

Run:

```bash
git diff --check -- web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx
git diff -- web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx
```

Expected: no whitespace errors; the diff contains only column drag behavior and focused tests. Do not commit without explicit authorization.

---

### Task 2: Column drag visual feedback

**Files:**
- Modify: `web/dashboard/src/styles.test.ts:64-80`
- Modify: `web/dashboard/src/styles.css:4682-4790`

**Interfaces:**
- Consumes: `.kanban-column-grip`, `.kanban-column.is-column-dragging`, and `.kanban-column.is-column-drop-target` classes emitted by Task 1.
- Produces: grab/grabbing pointer feedback, subdued active-column feedback, and a distinct target highlight that does not reuse `.is-drop-target`.

- [ ] **Step 1: Write the failing stylesheet test**

Add after `keeps Kanban card editing chrome compact`:

```ts
it("distinguishes column dragging from card drop feedback", () => {
  expect(ruleBodies(".kanban-column-grip").at(0)).toContain("cursor: grab");
  expect(ruleBodies(".kanban-column.is-column-dragging").at(0)).toContain("opacity:");
  expect(ruleBodies(".kanban-column.is-column-drop-target").at(0)).toContain("outline:");
});
```

- [ ] **Step 2: Run the focused stylesheet test and verify RED**

Run:

```bash
npm --prefix web/dashboard test -- --run src/styles.test.ts
```

Expected: FAIL because the three column drag selectors do not exist.

- [ ] **Step 3: Add minimal column drag styles**

Add beside the existing Kanban column and action styles:

```css
.kanban-column.is-column-dragging {
  opacity: .58;
}

.kanban-column.is-column-drop-target {
  outline: 2px solid color-mix(in srgb, var(--brand-dark) 76%, var(--line));
  outline-offset: -2px;
}

.kanban-column-grip {
  cursor: grab;
}

.kanban-column-grip:active {
  cursor: grabbing;
}
```

- [ ] **Step 4: Run the focused stylesheet test and verify GREEN**

Run:

```bash
npm --prefix web/dashboard test -- --run src/styles.test.ts
```

Expected: PASS.

- [ ] **Step 5: Run both changed test files together**

Run:

```bash
npm --prefix web/dashboard test -- --run src/dashboard/KanbanEditor.test.tsx src/styles.test.ts
```

Expected: PASS with no errors or warnings.

- [ ] **Step 6: Review Task 2 without committing**

Run:

```bash
git diff --check -- web/dashboard/src/styles.css web/dashboard/src/styles.test.ts
git diff -- web/dashboard/src/styles.css web/dashboard/src/styles.test.ts
```

Expected: no whitespace errors; the diff contains only drag grip and column feedback styling plus its test. Do not commit without explicit authorization.

---

### Task 3: Frontend regression validation

**Files:**
- Verify only: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Verify only: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`
- Verify only: `web/dashboard/src/styles.css`
- Verify only: `web/dashboard/src/styles.test.ts`

**Interfaces:**
- Consumes: completed component behavior and styles from Tasks 1 and 2.
- Produces: test, type-check, and production-build evidence for the dashboard change.

- [ ] **Step 1: Run the complete frontend test suite**

Run:

```bash
make frontend-test
```

Expected: all Vitest suites pass.

- [ ] **Step 2: Run the frontend type check and production build**

Run:

```bash
make frontend-build
```

Expected: TypeScript emits no errors and Vite completes a production build.

- [ ] **Step 3: Run the repository frontend check gate**

Run:

```bash
make frontend-check
```

Expected: the combined frontend test and build gate passes.

- [ ] **Step 4: Inspect the final scoped diff and worktree**

Run:

```bash
git diff --check
git status --short
git diff -- web/dashboard/src/dashboard/KanbanEditor.tsx web/dashboard/src/dashboard/KanbanEditor.test.tsx web/dashboard/src/styles.css web/dashboard/src/styles.test.ts docs/superpowers/specs/2026-07-19-kanban-column-drag-reorder-design.md docs/superpowers/plans/2026-07-19-kanban-column-drag-reorder.md
```

Expected: no whitespace errors; only the approved spec, this plan, focused component/tests/styles, and the pre-existing unrelated `.codex/` entry are present.

- [ ] **Step 5: Report impact and remaining actions**

Report:

```text
Security impact: none; no auth, route, secret, file, or external-call behavior changed.
Database impact: none; no schema or migration change.
Validation: focused Kanban and stylesheet tests, full frontend tests, frontend production build, and frontend check gate passed.
Remaining action: no commit, push, deployment, or Hank Build card mutation was performed.
```
