# Notes Kanban Card Modal Editor Design

## Goal

Make card movement and card editing feel like separate, predictable actions. Dragging a card only moves it. Clicking a card opens a larger version of that card as a focused editor over the board. Screenshots can be pasted directly into the description and appear inline.

## Interaction Contract

### Dragging and clicking

- A native card drag never opens the editor, including after the drop completes.
- A card opens only from a deliberate click or keyboard activation on its open control.
- Dropping a closed card leaves it closed.
- Moving a card from inside an already-open editor keeps that editor open and updates its column location.
- The existing left/right movement buttons remain available for keyboard and touch users and do not open a closed card.
- Drag state suppresses the synthetic click that some browsers dispatch after `dragend`.

### Card editor

- Replace the right-side drawer with a centered modal that visually reads as an enlarged card.
- The board remains visible behind a dimmed backdrop without changing column widths.
- The modal inherits the selected card's color accent and contains:
  - editable title;
  - Markdown description editor and formatting toolbar;
  - inline screenshot previews;
  - column and due-date controls;
  - card color controls;
  - file picker and drop target;
  - move-left, move-right, and delete actions.
- Close through the close button, Escape, or a click on the backdrop. Clicking inside the modal never closes it.
- Opening focuses the title. Closing restores focus to the originating card when it still exists.
- The modal uses `role="dialog"`, `aria-modal="true"`, a useful accessible name, and keeps keyboard focus within its controls while open.
- On narrow screens, the modal becomes a near-full-screen card editor with independently scrolling content and fixed actions.

## Inline Screenshot Pasting

- Pasting one or more clipboard image files into the description intercepts only the image items. Normal text and link pastes retain the browser's default behavior.
- Each image uploads through the existing note-attachment API. No new route, storage model, or attachment format is introduced.
- Successful uploads insert canonical `hank-note-attachment://` Markdown image references at the current description selection or caret, preserving clipboard order.
- The caret moves after the inserted references so the user can continue typing naturally.
- Uploaded images render immediately in an inline media strip inside the modal and continue to render on the collapsed board card.
- The existing file picker and drag/drop target remain as secondary upload paths and use the same insertion logic.
- While uploads are running, the editor shows progress without blocking title, metadata, or movement edits.
- A failed upload leaves existing card text untouched and shows an inline retryable error. Successful images from a multi-image paste remain inserted even if a later image fails.

## State and Persistence

- Continue storing title and description in the existing card text field and metadata in the existing Kanban board JSON.
- Continue using the Notes page's 750 ms serialized background autosave. The modal has no separate Save button.
- Modal open/closed state and drag-suppression state are local UI state and are never persisted.
- Attachment Markdown references remain the durable link between a card and uploaded note attachments.
- This change adds no schema, migration, route, authentication, authorization, or secret-handling behavior.

## Component Boundaries

- `KanbanEditor` continues to own board movement and selection state.
- Extract the enlarged editor into a focused `KanbanCardModal` component. It receives selected-card data and explicit edit, move, upload, close, and delete callbacks rather than owning persistence.
- Use one attachment-insertion helper for clipboard paste, file selection, and file drop so all upload paths produce the same canonical Markdown.
- Movement accepts an explicit editor-preservation intent, preventing closed-card drag operations from selecting a card as a side effect.

## Validation

Automated tests must prove:

- dropping a card into another column moves it without opening the dialog;
- a click opens the modal and keyboard activation remains supported;
- a drag followed by a click-like browser event is suppressed;
- modal movement keeps an already-open editor open;
- Escape and backdrop close the modal while inside clicks do not;
- image paste uploads the clipboard file, inserts the canonical reference at the caret, and renders the preview;
- ordinary text paste is not intercepted;
- multi-image paste preserves ordering and handles partial failure;
- due date, color, formatting, attachment upload, and autosave integration continue working.

Rendered demo QA must cover the Notes Kanban route, a real cross-column drag, a deliberate card click, modal editing, screenshot paste, reload persistence, console health, and desktop plus narrow-width layout.

## Intentional Non-goals

- No collaborative presence, comments, assignments, project taxonomy, or new card schema.
- No replacement of the existing Markdown attachment contract.
- No custom pointer-based drag engine in this pass.
- No separate modal save or cancel transaction; edits continue to autosave as they are made.
