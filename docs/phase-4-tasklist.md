# Phase 4 Tasklist

This tasklist turns the notes phase into concrete implementation work.

## Phase 4 Objective

Move Hank notes onto the remote architecture so notes can be listed, fetched, saved, renamed, deleted, and synchronized through Hank Cloud and the home agent.

## Definition Of Done

Phase 4 is done when all of these are true:

1. The app can list and fetch notes remotely.
2. The app can save note changes remotely.
3. Notes use a domain-specific remote API, not just generic remote file calls.
4. Conflicts are detected and handled predictably.

## Recommended Implementation Order

1. Notes domain schema
2. Agent-side notes abstraction
3. Remote CRUD operations
4. Conflict model
5. Sync and reconnect behavior tests

## Task Group 1: Notes Domain Model

### Define Protocol Types

Add types for:

- note summary
- note content
- note revision metadata
- sync status
- conflict response

### Suggested Commands

- `notes.list`
- `notes.fetch`
- `notes.save`
- `notes.rename`
- `notes.delete`
- `notes.sync`

### Required Metadata

At minimum include:

- note ID or stable path
- title
- updated timestamp
- revision or version marker

## Task Group 2: Agent Notes Service

### Create Package

Add:

- `internal/agent/notes`

### Suggested Interface

```go
type Service interface {
    List(ctx context.Context) ([]Summary, error)
    Fetch(ctx context.Context, noteID string) (Document, error)
    Save(ctx context.Context, noteID string, content string, expectedRevision string) (SaveResult, error)
    Rename(ctx context.Context, noteID string, title string) error
    Delete(ctx context.Context, noteID string) error
}
```

### Storage Direction

The first pass can sit on top of local file storage if that matches Hank’s existing note model.

The agent should translate local storage details into note-domain responses.

## Task Group 3: Conflict Handling

### Choose A Simple First Model

Recommended first pass:

- optimistic concurrency using revision IDs or last-modified hashes

Save requests should include an expected revision.

If the current stored revision differs, return a structured conflict response instead of silently overwriting.

### Conflict Response Should Include

- current server revision
- current server content or summary
- clear conflict error code

## Task Group 4: Cloud Relay

### Add Notes Routing

Cloud should:

- authorize the app for the target home
- route note commands to the correct agent
- return structured note responses and conflicts

### Suggested Files

- `internal/cloud/relay_notes.go`
- `internal/cloud/http_notes.go` if HTTP endpoints are preferred

## Task Group 5: Agent Command Handling

### Extend Agent Dispatcher

Handle:

- list notes
- fetch note
- save note
- rename note
- delete note
- sync metadata

## Task Group 6: Sync Behavior

### Build Minimal Sync Support

For the first pass:

- app requests note list
- app fetches changed notes
- save returns new revision metadata

Later sync can become more incremental, but the first pass should at least support reliable reconnect and save behavior.

## Task Group 7: Testing

### Unit Tests

Add tests for:

- revision conflict detection
- note schema encode/decode
- title/path normalization if needed

### Integration Tests

Required scenarios:

1. list notes
2. fetch one note
3. save note successfully
4. rename note
5. delete note
6. save note with stale revision and receive conflict

Reconnect scenarios:

1. agent disconnects and reconnects
2. app resyncs note metadata after reconnect

## Task Group 8: Logging And Errors

Add logs for:

- note fetch
- note save
- note rename
- note delete
- note conflict

Define stable errors:

- `note_not_found`
- `note_conflict`
- `invalid_note_request`

## Suggested File Additions

- `internal/protocol/notes.go`
- `internal/agent/notes/service.go`
- `internal/agent/notes/file_adapter.go`
- `internal/agent/commands_notes.go`
- `internal/cloud/relay_notes.go`
- `internal/agent/notes/*_test.go`

## Suggested Codex Prompt For This Phase

> Implement Phase 4 from `docs/phase-4-notes.md` and `docs/phase-4-tasklist.md`. Add a note-domain protocol, agent-side notes service, remote note CRUD, optimistic concurrency for saves, and tests for fetch, save, rename, delete, and conflict handling. Run formatting and `go build ./...` before finishing.
