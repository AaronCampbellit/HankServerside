# Notes Kanban Workboard Design

## Goal

Turn the Notes Kanban page from a read-only renderer into a polished work board that supports daily project execution: capture, edit, format, link, attach screenshots, reorganize, and complete cards without leaving Notes.

## Direction

Use an "operations desk" layout derived from the approved concept at `/Users/aaroncampbell/.codex/generated_images/019f6ee1-21d0-7011-9077-62001ac57cab/exec-0ac3baa8-6248-4b7b-8cbe-41cb9cb72dc8.png`.

- Preserve the existing Hank Notes rail and dark dashboard shell.
- Let the board consume the editor workspace horizontally instead of forcing three equal columns.
- Use compact slate cards, crisp borders, restrained shadows, and a single Hank cyan accent.
- Use muted per-card accent colors only when the user chooses one.
- Keep controls code-native, keyboard reachable, and readable at laptop density.

## Board Model

The existing `board` JSON remains the source of truth for Kanban notes. The protocol stays backward compatible:

- columns keep `id`, `title`, `sort_order`, and `cards`;
- cards keep `id`, `text`, `sort_order`, `created_at`, and `updated_at`;
- cards gain optional `color` and `due_date` fields;
- rich card content is Markdown stored in `text`;
- screenshots use the existing Notes attachment API and canonical `hank-note-attachment://` Markdown references.

Legacy Markdown-only boards are materialized into board JSON on the first edit. Converting text to Kanban creates columns from `##` headings when possible and otherwise starts with `Inbox`, `In progress`, and `Done`. Converting back to text serializes the board so work is not discarded.

## Primary Workflow

### Board workspace

- A board toolbar provides card search and an `Add column` action.
- Columns scroll horizontally and retain a useful fixed working width.
- Every column supports rename, move left/right, and delete with confirmation when it contains cards.
- An inline `Add task` composer creates a card without opening a modal.
- Empty columns remain obvious drop targets.

### Cards

- Clicking a card opens a right-side `Task details` drawer.
- The drawer edits title, Markdown description, due date, and a restrained card accent.
- Formatting controls cover bold, italic, bullets, and links.
- Links render as safe clickable anchors in the card.
- Image attachment references render as thumbnails using the attachment download URL returned by the existing Notes API.
- Files and screenshots can be selected or dropped into the drawer; unsaved boards are saved before upload.
- Delete and column movement are available in the drawer.

### Movement

- Desktop users drag cards within or across columns.
- Drop position is represented by the target card or the column body.
- Each card also exposes explicit move-left and move-right controls for keyboard and touch users.
- Column movement uses explicit controls rather than drag to avoid competing horizontal gestures.

## Persistence and Failure Behavior

- Every board mutation updates the parent Notes editor state and uses the existing 750 ms background autosave queue.
- `board` is included in the Notes save request; this repairs the current data-loss bug.
- New boards are saved before the first attachment upload.
- Upload failures leave the card draft open and show the existing error toast.
- External links use safe schemes and open in a new tab with `noopener noreferrer`.
- Deleting a non-empty column requires confirmation and deletes its contained cards only after acceptance.

## Responsive Behavior

- Desktop: horizontally scrolling columns plus a fixed-width details drawer.
- Narrow desktop/tablet: the drawer overlays the board instead of shrinking columns below their working width.
- Mobile: columns remain horizontally scrollable; explicit movement controls replace reliance on drag; the drawer becomes a full-width sheet.

## Accessibility

- Buttons have names independent of icons.
- Cards are real buttons, not clickable generic containers.
- Drag state is supplementary; every movement has a button equivalent.
- Focus rings, dialog labeling, escape-to-close, and readable contrast follow existing dashboard primitives.
- Image thumbnails use the attachment filename as alt text.

## Validation

- API tests pin `board` persistence and binary attachment upload.
- component tests cover creating, editing, formatting, moving, reordering, deleting, and uploading cards;
- the dashboard test/build gate must pass;
- the implementation is deployed to the Hank demo server and verified at `/dashboard/profile-notes` in the Browser at desktop and mobile widths;
- the final implementation screenshot is compared directly with the concept for layout, density, palette, typography, card anatomy, and drawer behavior.

## Scope Boundaries

- No new JavaScript dependencies.
- No multi-user assignment system, time tracking, estimates, analytics, swimlanes, or backend task service.
- No change to the stable Notes attachment route.
- No changes to Hank iOS in this repository.
