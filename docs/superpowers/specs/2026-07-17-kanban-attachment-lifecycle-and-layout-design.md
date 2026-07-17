# Kanban Attachment Lifecycle and Layout Design

## Goal

Polish the Notes Kanban workflow so task creation and editing controls are immediately available, description editing begins from a click anywhere in the description surface, and uploaded task files have an explicit, auditable lifecycle. Add an admin-only attachment manager that shows live Notes attachment storage and supports deliberate deletion.

## Confirmed Product Decisions

- Put each column's Add task control directly below its header and above its cards.
- Put every selected-card option at the top of the enlarged card editor: column, due date, color, Move left, Move right, and Delete task.
- Keep attachment upload and the description editor below those options.
- Make the entire description preview clickable. A click enters edit mode and focuses the description textarea so typing can begin immediately.
- Delete files that are exclusive to a deleted task. Preserve files referenced by any surviving task in the same note.
- Apply the same attachment-aware behavior when deleting a column containing tasks.
- Add an admin-only Settings > Attachments page covering all live Notes attachments in the single-home deployment.

## Kanban Interaction Design

### Column task creation

The Add task button or open task composer renders immediately after the column header. Existing cards follow it. The composer retains its current keyboard behavior: Enter creates, Shift+Enter inserts a newline, and Cancel closes the composer.

### Description editing

The description remains a rich preview when the card first opens. The entire preview is an interactive edit target, including its empty state and unused whitespace. Activating it switches to the textarea and focuses the editor. The existing explicit Edit and Preview controls remain for discoverability and keyboard use.

Clicking a link or attachment inside the rich preview continues to open that resource and must not switch the description into edit mode. Clicking other preview content enters edit mode. The textarea remains the paste target for inline screenshots and normal text.

### Card options

A compact options area appears directly below the title. It contains:

- Column selector;
- Due date;
- Card color;
- Move left;
- Move right;
- Delete task.

Delete remains visually destructive even though it moves to the top. The old bottom footer is removed. On narrow viewports, the controls wrap without introducing horizontal scrolling or hiding Delete.

## Attachment Storage Model

Attachment file bytes remain outside Postgres in the configured Notes attachment root. In the Compose deployment this is the `hank_note_attachments` volume mounted at `/var/lib/hank/note-attachments`. The `note_attachments` table remains the source of metadata such as attachment ID, note ID, owner, filename, content type, byte size, checksum, storage key, and deletion timestamp.

No database schema or migration is required. Existing backup behavior remains unchanged: the database and `hank_note_attachments` volume must be retained together.

## Reference and Ownership Rules

Canonical `hank-note-attachment://` references continue to connect note content and Kanban card descriptions to stored files. Attachment ownership remains note-scoped.

Before deleting a task or column, the client identifies attachment IDs referenced by the cards being removed. It then compares those IDs with every surviving card in the same board:

- an attachment referenced by a surviving card is shared and preserved;
- an attachment referenced only by deleted cards is exclusive and scheduled for permanent deletion;
- attachments not referenced by the deleted cards are unaffected.

Repeated references to the same attachment count as one file. File totals use unique attachment IDs and server-provided byte sizes.

## Task and Column Deletion Flow

The confirmation prompt names the task or column and includes:

- exclusive attachment count;
- exclusive attachment filenames;
- exclusive attachment total size;
- shared attachment count, when nonzero, with a statement that shared files will be preserved;
- a permanent-deletion warning.

Example: `Delete "Site review"? This will permanently delete 2 attached files (3.4 MB): site.png and field-notes.pdf. 1 shared file will be kept because another task uses it. This can't be undone.`

After confirmation:

1. Build the board without the selected task or column.
2. Save that board through the normal Notes save path and wait for the new revision.
3. If the board save fails, keep the task and every attachment unchanged.
4. After the board save succeeds, delete each exclusive attachment through the authenticated note-attachment API.
5. Carry the returned note revision forward after each deletion so serialized autosave remains consistent.
6. Remove deleted attachments from the in-memory attachment collection.

If an attachment deletion fails after the board save, the task remains deleted, the failed attachment remains a live unreferenced attachment, and the UI reports that cleanup is incomplete. It remains visible in Settings > Attachments for manual deletion. Successful attachment deletions are not rolled back.

The attachment DELETE response gains the updated note revision. This is an additive API change and does not alter authentication or token scope behavior.

## Physical File Deletion

The existing attachment deletion path continues to soft-delete the database row and remove the physical file after the note update commits. File-removal errors must no longer be silently ignored: they are logged with attachment identity but no file contents, and the response reports cleanup failure. Existing orphan-file maintenance remains a secondary recovery path.

Automated handler tests verify that a successful DELETE makes the attachment unavailable through the API, marks it deleted in storage metadata, removes its canonical reference, and removes the physical file from the configured attachment root.

## Admin Attachment Manager

### Navigation and authorization

Add an Attachments entry to the Settings subnavigation at `/dashboard/settings/attachments`. It is marked admin-only and is omitted or blocked for non-admin users using the same route and server authorization patterns as other administrative settings.

The server provides an admin-only inventory endpoint for all live Notes attachments in the single-home deployment. Browser writes retain existing cookie authentication and CSRF protection. Non-admin requests receive a forbidden response even if the UI route is entered directly.

### Inventory response

The inventory response includes summary totals and attachment rows. Each row provides:

- attachment ID;
- filename;
- content type;
- byte size;
- created time;
- note public ID and title;
- note scope;
- owner identity suitable for admin display;
- reference count;
- an admin-authorized content URL.

The server calculates total live file count and total live bytes from the same filtered result so summary cards and rows cannot disagree.

### Page layout

The page header explains that file bytes are stored in the Notes attachment volume and metadata is stored in Postgres. Two summary cards show total files and total storage. The list defaults to newest first and presents filename, note/task context when resolvable, owner, type, size, upload date, reference state, Open, and Delete.

An unreferenced attachment is visibly labeled so incomplete cleanup can be found quickly. Empty, loading, forbidden, and error states use existing Settings patterns.

### Manual deletion

Each row has a Delete action. Its confirmation includes filename, size, note title, and a warning that the file will be removed everywhere it appears in that note.

The admin deletion handler:

1. verifies admin authorization;
2. loads the attachment and owning note;
3. removes the attachment's canonical references from note Markdown and every Kanban card description in board JSON;
4. saves the note and soft-deletes the attachment metadata together;
5. removes the physical file;
6. returns the updated inventory totals.

Other attachments and unrelated card content are unchanged. A failed note/metadata transaction leaves the physical file in place. A post-commit physical cleanup failure is reported and logged for maintenance recovery.

## Component and Service Boundaries

- `KanbanCardModal` owns only the enlarged-card presentation and edit-mode interaction.
- `KanbanEditor` calculates the next board and classifies selected-card attachment references as exclusive or shared.
- `ProfileNotesPage` owns the serialized save-then-delete workflow because it owns the current note revision, autosave queue, attachment collection, and toast reporting.
- `ProfileNotesClient` exposes per-note attachment deletion and consumes the returned note revision.
- A focused Settings API client and Attachments settings component own admin inventory presentation.
- Cloud attachment helpers centralize reference removal, metadata soft deletion, and physical cleanup so per-note and admin deletion do not diverge.
- Store additions are explicit list/get queries over existing tables; they do not mutate schema.

## Security and Data Safety

- No new public route is introduced.
- The inventory and global deletion routes require an authenticated home admin on the server.
- Existing profile/home note ownership checks remain in force for ordinary attachment deletion.
- Attachment paths continue through the existing cleaned, symlink-contained resolver.
- Responses and logs never include file contents, credentials, or raw filesystem paths.
- Destructive actions require a user-facing confirmation describing the exact file impact.
- Shared-reference detection defaults to preservation when reference parsing is uncertain.

## Validation

Automated coverage must prove:

- Add task/composer renders above the card stack;
- clicking description whitespace enters edit mode and focuses the textarea;
- preview links and attachment clicks do not enter edit mode;
- all card options render near the top and the old footer is absent;
- task and column prompts include unique file count, total size, filenames, and shared-file preservation;
- deleting a task saves the board before issuing attachment deletes;
- a failed board save deletes no attachments;
- shared attachments survive task and column deletion;
- exclusive attachments are soft-deleted and physically removed;
- deletion revisions remain serialized across multiple files;
- inventory totals equal the listed live rows;
- non-admin inventory and deletion requests are forbidden;
- admin deletion removes references from Markdown and Kanban board JSON;
- the Settings page updates totals and rows after deletion;
- existing upload, inline rendering, drag movement, and autosave tests remain green.

Rendered demo QA must cover desktop and narrow-width layouts, Add task placement, description click-to-edit, top card actions, a deletion prompt with attachments, shared-file preservation, Settings inventory totals, manual attachment deletion, reload persistence, and console health.

## Intentional Non-goals

- No bulk-delete or delete-all control.
- No attachment deduplication across notes.
- No file versioning, recycle bin, or restore UI.
- No attachment migration into Postgres or object storage.
- No new Kanban comments, assignments, or project-management schema.
