# Note Notebook Database Integrity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce active note-to-notebook relationships in PostgreSQL while retaining loose root notes and the existing `parent_id` API contract.

**Architecture:** Add an owner-scoped self-referential foreign key to `user_notes` and PostgreSQL triggers that validate active parent types and protect notebooks with active children. Keep the Go service validation for friendly API errors, while behavior-level PostgreSQL tests prove direct store writes cannot bypass integrity.

**Tech Stack:** Go, PostgreSQL, embedded versioned SQL migrations, standard `testing` package.

## Global Constraints

- Root notes remain represented by `parent_id IS NULL`.
- Existing API, protocol, collaboration, and client `parent_id` payloads remain unchanged.
- Invalid historical active relationships abort migration without rewriting data.
- Notebook deletion and type changes require active children to be moved or made loose first.
- Preserve unrelated worktree changes and do not commit, push, or deploy.

---

### Task 1: Pin Database Hierarchy Behavior

**Files:**
- Modify: `internal/store/user_notes_test.go`

**Interfaces:**
- Consumes: `Store.UpsertUserNote(context.Context, domain.UserNote) error`
- Produces: behavior-level regression coverage for root notes, valid notebook children, invalid parents, and protected notebook transitions

- [ ] **Step 1: Add real PostgreSQL fixtures and failing tests**

Add a focused `TestUserNotesStoreEnforcesNotebookHierarchy` using
`OpenMigrating(ctx, testutil.PostgreSQLTestURL(t))`. Create two users, an active
notebook, a valid child, and a loose note. Use literal table cases to assert
that `UpsertUserNote` rejects:

```go
[]struct {
    name     string
    parentID string
    ownerID  string
}{
    {name: "missing parent", parentID: "missing", ownerID: owner.ID},
    {name: "cross-owner parent", parentID: "other-notebook", ownerID: owner.ID},
    {name: "non-notebook parent", parentID: "plain.md", ownerID: owner.ID},
    {name: "deleted notebook parent", parentID: "deleted-notebook", ownerID: owner.ID},
    {name: "self parent", parentID: "self.md", ownerID: owner.ID},
}
```

Assert valid notebook membership round-trips through `GetProfileNote`, a loose
note saves with an empty `ParentID`, and updates that soft-delete or convert a
notebook with an active child return database errors.

Update `TestUserNotesStoreFullMetadataAndPostgresNotify` to create its
`parent.md` notebook before saving the child so the fixture represents a valid
relationship.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/store -run 'TestUserNotesStore(EnforcesNotebookHierarchy|FullMetadataAndPostgresNotify)' -count=1
```

Expected: the new invalid-parent and notebook-transition assertions fail
because PostgreSQL currently accepts those writes.

### Task 2: Add Migration 26 and Canonical Schema Rules

**Files:**
- Create: `internal/migrations/sql/000026_note_notebook_integrity.up.sql`
- Create: `internal/migrations/sql/000026_note_notebook_integrity.down.sql`
- Modify: `internal/migrations/migrations_test.go`

**Interfaces:**
- Consumes: existing unique index `idx_user_notes_owner_note_id`
- Produces: constraint `user_notes_owner_parent_fkey`, trigger functions `validate_user_note_parent()` and `protect_user_note_notebook_parent()`

- [ ] **Step 1: Add a failing migration contract test**

Add `TestNoteNotebookIntegrityMigrationDefinesDatabaseRules`. Locate migration
version 26 through `All()`, join its statements, and assert it defines:

```go
required := []string{
    "user_notes_owner_parent_fkey",
    "FOREIGN KEY (owner_user_id, parent_id)",
    "validate_user_note_parent",
    "protect_user_note_notebook_parent",
    "page_type <> 'notebook'",
    "deleted_at IS NOT NULL",
}
```

- [ ] **Step 2: Run the migration test and verify RED**

Run:

```bash
go test ./internal/migrations -run TestNoteNotebookIntegrityMigrationDefinesDatabaseRules -count=1
```

Expected: FAIL because migration 26 does not exist.

- [ ] **Step 3: Implement the upgrade migration**

Create migration 26 with:

1. A `DO $$ ... $$` preflight that audits all non-null parents for existence
   and owner scope, then applies deleted/type/self checks to active children.
   It raises an exception when the relationship is invalid.
2. `ALTER TABLE user_notes ADD CONSTRAINT
   user_notes_owner_parent_fkey FOREIGN KEY (owner_user_id, parent_id)
   REFERENCES user_notes(owner_user_id, note_id) ON UPDATE RESTRICT ON DELETE
   RESTRICT`.
3. `validate_user_note_parent()` as a `BEFORE INSERT OR UPDATE OF
   owner_user_id, parent_id, deleted_at` trigger. Return immediately for deleted
   children or null parents; otherwise reject self, missing, deleted, or
   non-notebook parents.
4. `protect_user_note_notebook_parent()` as a `BEFORE UPDATE OF page_type,
   deleted_at` trigger. Reject leaving active notebook state when an active
   child references `OLD.note_id` under the same owner.

Use named dollar quotes supported by `splitSQLStatements`, `SELECT ... FOR
KEY SHARE` when validating the parent, and SQLSTATE `23514` for semantic
hierarchy violations.

- [ ] **Step 4: Implement reversible teardown**

The down migration drops both triggers, both functions, and the foreign key
without modifying data.

Do not edit migration 1 or the checksum compatibility map. Fresh and upgraded
databases both receive the integrity rules by applying migration 26 through the
same ordered migration path.

- [ ] **Step 5: Run migration and store tests and verify GREEN**

Run:

```bash
go test ./internal/migrations ./internal/store -run 'TestNoteNotebookIntegrityMigrationDefinesDatabaseRules|TestUserNotesStore(EnforcesNotebookHierarchy|FullMetadataAndPostgresNotify)' -count=1
```

Expected: PASS, or explicit PostgreSQL test skips only when
`HANK_REMOTE_TEST_DATABASE_URL` is unavailable.

### Task 3: Preserve API Behavior and Validate the Platform

**Files:**
- Modify only if a regression proves necessary: `internal/cloud/notes_store.go`
- Modify only if a regression proves necessary: `internal/cloud/notes_api_test.go`

**Interfaces:**
- Consumes: existing `cloudNotesService.validateNotebookParent`
- Produces: unchanged client-visible notebook lifecycle with database-backed integrity

- [ ] **Step 1: Run existing notebook lifecycle tests**

Run:

```bash
go test ./internal/cloud -run 'TestProfileNotes(NotebookLifecycleAndScopedSearch|RejectTextNoteAsNotebookParent)' -count=1
```

Expected: PASS. If the database trigger exposes a service behavior mismatch,
first add a focused failing API assertion, verify RED, then make the smallest
service correction and rerun to GREEN.

- [ ] **Step 2: Format and run focused packages**

Run:

```bash
make fmt
go test ./internal/migrations ./internal/store ./internal/cloud -count=1
```

Expected: PASS, with PostgreSQL-backed skips reported explicitly.

- [ ] **Step 3: Run whole-platform verification**

Run:

```bash
go test ./...
make build
make migrate-status
make schema-drift-check
git diff --check
```

Expected: all available commands exit zero. Record any database checks that
cannot run due to missing configured PostgreSQL credentials or services.

- [ ] **Step 4: Review scope and requirements**

Inspect `git status --short` and the final diff. Confirm unrelated Kanban edits
are unchanged, no secrets are present, no API contract changed, both new
migration directions exist, and every design requirement has a corresponding
test or validation result.
