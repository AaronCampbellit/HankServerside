# Note Notebook Database Integrity Design

## Goal

Make notebook membership a database-enforced relationship while preserving the
existing Notes API and allowing loose root notes.

The existing `user_notes.parent_id` value is already persisted and exposed as
the parent notebook's stable `note_id`. The change strengthens that model so
PostgreSQL rejects invalid hierarchy states even when a write bypasses the
cloud service's Go validation.

## Scope

- Keep `parent_id IS NULL` as a valid loose root note.
- Keep `parent_id` in API, protocol, collaboration, and client payloads.
- Enforce that every active note with a non-null parent belongs to the same
  owner and references an active notebook.
- Prevent notebooks with live children from being deleted or converted to
  another page type until their children are moved or made loose.
- Add migration, store/API coverage, migration checks, and schema-drift
  compatibility.

This change does not add nested notebooks, multi-parent notes, cascading
deletion, automatic reparenting, or client-specific behavior.

## Data Model

`user_notes.parent_id` continues to contain the logical `note_id` of the parent
notebook. A null value means the note is at the root.

The database will enforce the structural relationship with a composite foreign
key:

```sql
FOREIGN KEY (owner_user_id, parent_id)
    REFERENCES user_notes (owner_user_id, note_id)
```

The existing unique index on `(owner_user_id, note_id)` is the referenced key.
Because PostgreSQL foreign keys permit null referencing columns, loose notes
remain valid without a sentinel parent.

The foreign key provides existence and same-owner integrity. Focused trigger
functions provide the rules that a foreign key cannot express:

- an active child cannot reference itself;
- an active child's parent must have `page_type = 'notebook'`;
- an active child's parent must not be soft-deleted;
- a notebook with live children cannot be soft-deleted or changed to `text` or
  `kanban`.

Hard deletion of a referenced notebook is restricted by the foreign key.

## Migration

Migration 26 will run atomically through the existing migration runner.

Before adding constraints, it will audit every row with a non-null `parent_id`
for the existence and owner scope required by the foreign key. For active rows,
it also audits notebook semantics. The migration will fail with a clear
exception if it finds:

- a missing parent;
- a parent owned by another user;
- a parent that is not a notebook;
- a soft-deleted parent;
- a self-reference.

The migration will not silently detach, reassign, or delete notes. An operator
must correct invalid historical data and rerun the migration, preserving an
auditable and lossless upgrade path.

After the audit passes, the migration will:

1. Ensure the existing owner/note unique key is available to the foreign key.
2. Add the composite foreign key.
3. Create the hierarchy-validation trigger functions and triggers.

The down migration removes only the new triggers, functions, and foreign key.
It does not mutate note data or remove the pre-existing index.

Fresh and upgraded databases both apply migration 26 through the same ordered
migration path. Previously checksummed migrations, including migration 1, will
not be edited.

## Write and Delete Behavior

The cloud service keeps its current validation so API callers receive stable,
friendly client errors before a database write. Database validation is the
last line of defense for direct store writes, collaboration writes, future
services, and operational mistakes. Parent-type and active-parent trigger
checks apply to active child rows; the foreign key continues to protect the
stored parent identity of deleted rows.

Saving or moving a note under a valid active notebook succeeds. Saving a loose
note succeeds. Invalid parent assignments fail.

Deleting or changing the type of a notebook that has live children fails. The
operator or user must first move each child to another notebook or to the root.
This avoids orphaning notes and avoids destructive cascade behavior.

Soft-deleted child rows do not block a notebook transition because they are not
part of the active hierarchy. Their historical `parent_id` remains stored for
recovery and audit purposes.

## Compatibility and Security

No API, protocol, route, authentication, authorization, or client payload
changes are required. Existing clients continue reading and writing
`parent_id`.

The composite key prevents cross-owner relationships at the database layer.
No secrets, file contents, or additional user data are logged. The migration
does not broaden note visibility or sharing.

## Testing and Validation

Tests will be written before production changes and will cover:

- the migration contains the expected audit, foreign key, and trigger rules;
- loose root notes remain valid;
- a note persists beneath an active notebook across store fetch/list paths;
- direct writes reject missing, cross-owner, non-notebook, deleted, and
  self-referential parents;
- a referenced notebook cannot be soft-deleted or converted;
- moving children to the root allows the notebook transition;
- existing API notebook lifecycle and scoped-search behavior remains valid.

Validation will include targeted migration/store/cloud tests, `make fmt`,
`go test ./...`, `make build`, `make migrate-status`, and
`make schema-drift-check` when the configured PostgreSQL environment supports
the database-backed checks. Any skipped PostgreSQL validation will be reported
explicitly.

## Operational Outcome

Upgrades preserve valid notebook relationships. Invalid historical rows stop
the migration instead of being silently rewritten. After migration, the
database itself guarantees that active notes can only be organized beneath
active notebooks owned by the same user.
