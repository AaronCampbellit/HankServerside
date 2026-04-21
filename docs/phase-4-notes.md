# Phase 4: Notes Sync And Background Flows

## Goal

Move Hank notes onto the same remote architecture so notes can be browsed, updated, and synchronized without direct SMB from the phone.

## Why This Phase Is Separate

Notes are more than just file CRUD. They need sync rules, conflict handling, and possibly background refresh behavior.

## Scope

Build:

- note fetch and save commands
- note metadata sync
- conflict detection and resolution strategy
- optional incremental sync or event notifications

Do not mix this with raw file API semantics if the app needs richer note behavior.

## Design Direction

Treat notes as a first-class domain:

- the app should ask for notes
- the agent should translate that into local file or storage operations

Do not force the app to reconstruct notes semantics only from generic remote file calls if a domain API makes the app simpler.

## Suggested Command Set

- `notes.list`
- `notes.fetch`
- `notes.save`
- `notes.rename`
- `notes.delete`
- `notes.sync`
- `notes.search`
- `notes.tags`
- `notes.tag_rollup`

## Deliverables

- note domain protocol messages
- agent-side notes adapter
- cloud relay for note commands
- conflict model for concurrent edits
- page-type-aware note metadata and kanban payload support

## Exit Criteria

Phase 4 is complete when:

- the app can load notes remotely through Hank Cloud
- the app can save note changes remotely
- conflicts are detected and handled predictably
- notes no longer require direct remote SMB access from the app

## Testing Expectations

Add tests for:

- note fetch and save
- rename and delete
- concurrent edit conflict handling
- missing note behavior
- sync recovery after reconnect

## Recommended First Tasks

1. Define the note domain schema.
2. Add agent-side note storage abstraction.
3. Decide conflict semantics.
4. Add remote note CRUD.
5. Add sync tests with reconnect scenarios.
