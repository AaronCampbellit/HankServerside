# Compact Kanban Card Editor Design

## Goal

Make Kanban columns and the task modal prioritize cards and description editing instead of oversized creation and metadata chrome.

## Approved Layout

- Put a compact icon-only Add task action in each column header immediately before Delete.
- Keep the task composer below the header only while it is active; do not reserve a full-width Add task row.
- Reduce the modal title to a one-line field in a shallow header.
- Keep Column, Due date, Color, movement, and Delete together in a compact metadata panel.
- Reduce formatting controls to a short toolbar.
- Render file upload as a slim clickable/drop row.
- Give the description preview/editor the primary visible area and preserve inline images.

## Constraints

- Preserve current accessible labels, keyboard behavior, drag behavior, autosave, attachment lifecycle, and responsive containment.
- Make no backend, API, authentication, or database changes.
- Verify desktop and narrow responsive layouts on the demo server.

## Test Strategy

- Component tests prove Add task lives in column actions and the title is one line.
- Stylesheet tests enforce the compact modal, toolbar, and upload density.
- Existing dashboard tests protect task creation, editing, movement, uploads, and deletion.

