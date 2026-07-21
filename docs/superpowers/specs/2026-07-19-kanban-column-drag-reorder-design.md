# Kanban Column Drag Reordering Design

## Goal

Let users directly reorder Notes Kanban columns by dragging a dedicated grip in each column header. Preserve the existing left and right move buttons as the keyboard-accessible fallback.

## Scope

This is a dashboard-only interaction change in the existing Notes Kanban editor. It does not add or change an API route, protocol message, persistence type, database schema, board setting, or MCP operation.

## Interaction

- Each column header displays a dedicated drag grip with an accessible label naming the column.
- Starting a drag from the grip marks that column as the active dragged column.
- Dragging over another column marks it as the current column drop target without activating card-drop behavior.
- Dropping places the dragged column at the target column's position.
- Dropping a column on itself is a no-op.
- Ending or canceling a drag clears all column drag state and visual feedback.
- Existing left and right column move buttons remain available and continue to use the same ordered-board mutation path.
- Existing card dragging, column renaming, menus, add-task controls, and delete controls retain their current behavior.

## State and Data Flow

`KanbanEditor` owns a separate dragged-column identifier and column drop-target identifier. Column drag handlers use a distinct drag payload from card dragging so the two interaction types cannot be confused.

The reorder operation removes the dragged column from the current ordered column array, inserts it at the target position, and passes the result through the existing `commit` and `normalizeBoard` path. Normalization rewrites sequential `sort_order` values before the existing page autosave persists the board.

No optimistic local persistence mechanism is added because the editor already rerenders from the normalized board supplied through `onChange`.

## Visual Behavior

The grip uses the existing compact icon language. The active column receives a subdued dragging style, and the target column receives a distinct column-reorder highlight. Column-reorder styling is separate from the existing card-drop target styling so the current drag intent remains understandable.

The grip is the only draggable region. Making the whole header draggable would conflict with rename, menu, add, delete, and move controls.

## Accessibility

- The grip is a named control and exposes its draggable state.
- Existing named left and right buttons remain the non-pointer reorder mechanism.
- Disabled first/last movement behavior remains unchanged.
- Reordering does not move focus or open another editor.

## Error and Edge Cases

- Unknown dragged or target column identifiers produce no mutation.
- A self-drop produces no mutation.
- Drag end always clears transient state.
- Card drags continue to route only through card handlers.
- Column metadata, cards, intake selection, workflow roles, and unrelated board fields remain attached to their original column objects.

## Testing

Focused component tests will first demonstrate the missing behavior, then verify:

- dragging a column grip onto another column emits the expected ordered column array;
- persisted `sort_order` values are sequential after the move;
- a self-drop does not emit a board change;
- existing card dragging still moves a card rather than a column;
- the left and right buttons remain present as accessible fallback controls.

Frontend type checking, the focused Kanban test file, the frontend test suite, and the production frontend build will validate the completed change.

## Impact

Security impact: none; no authentication, authorization, routes, secrets, files, or external calls change.

Database impact: none; the existing ordered `board_json` representation is reused without a migration.
