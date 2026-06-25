# HankAI Chat Tool Improvement Plan

Status: Superseded by current HankAI implementation and active eval guidance. Retained only as historical planning context; use [hankai-local-model-evals.md](../../hankai-local-model-evals.md) and current code for live validation.

## Goal

Make Hank chat better at common personal-data and operator asks by tightening the
system prompt, deterministic intent routing, typed tool coverage, and regression
tests around real user phrases.

This plan keeps the current Hank Remote architecture:

- the cloud owns chat orchestration, auth, audit, retrieval, and confirmation
- the home agent owns local network access to SMB, Home Assistant, notes, media,
  and other home resources
- the iPhone app supplies device-only context and client tools such as EventKit
  calendar operations
- the model explains and summarizes, but typed Hank tools perform reads and
  writes

## Current Baseline

The server already has a useful typed-tool spine:

- `notes.list`, `notes.search`, and `notes.append`
- `files.search`
- attachment staging and confirmed attachment commits to Notes or SMB
- `calendar.create_event` through a client tool and confirmation flow
- `calendar.search` and `calendar.update_event` result finalization when the
  client tool is requested
- Home Assistant state queries
- project-doc answers over README, AGENTS, SERVER_SYNC, docs, and runbooks
- media search/download planning
- general retrieved-context answers

The biggest gaps are phrasing coverage, missing first-class intents for common
operations, and prompt/tool guidance that does not yet describe how each source
should be used together.

## Prompt Improvements

Update the default HankAI system prompt so it gives the model a clearer operating
contract:

- Prefer Hank tools over guessing when the user asks about notes, SMB files,
  calendar, Home Assistant, project docs, or prior Hank chat.
- Treat source names as user concepts: Notes, File Server or SMB shares,
  Calendar, Home Assistant, Hank project docs, and Hank chat history.
- Ask a short clarification when a target note, folder, event, entity, or share
  is ambiguous.
- Never claim a write happened until a typed tool result or confirmed client-tool
  result says it happened.
- For attachments, distinguish between metadata already staged by the server and
  raw bytes that must be committed by the app or agent.
- For calendar, account for the user's device timezone and ask for missing date,
  time, duration, or calendar when needed.
- For project-doc answers, prefer current docs and runbooks over archived phase
  documents unless the user explicitly asks for historical context.
- For destructive or high-impact actions, explain the intended target and require
  Hank confirmation.

## Tool Backlog

### Notes

Add or harden these intents:

- `notes.create`: create a note when the user clearly asks for a new note.
- `notes.append_text`: append text to a uniquely matched note or list.
- `notes.append_link`: preserve links from the current message or previous turn.
- `notes.append_attachment`: attach staged images, PDFs, or files to a note.
- `notes.find_text`: answer questions about content inside visible notes.
- `notes.summarize`: summarize one note or a filtered set of notes.
- `notes.check_item` and `notes.uncheck_item`: update checklist state.

Example phrases:

- `find the note where I wrote about the nginx setup`
- `add buy batteries to the store list`
- `add this link to the work note`
- `put this picture in the receipts note`
- `summarize my Hank project notes`
- `check off call Patrick in work`

Expected behavior:

- Exact title matches beat title contains, body matches, and semantic matches.
- A uniquely matched short list append can auto-plan, then return a concise card.
- New note creation and ambiguous shared-note edits require confirmation.
- Note responses should include a `note_id` card target when possible.

### SMB Files And Shares

Add or harden these intents:

- `files.search`: search indexed file and folder paths across enabled shares.
- `files.list_folder`: show a folder's immediate contents.
- `files.open`: return a deep-link card for a file or folder.
- `files.upload`: commit staged attachments to a uniquely matched folder.
- `files.create_folder`: create a folder after confirmation.
- `files.move`, `files.copy`, `files.rename`, and `files.delete`: confirmed file
  operations using existing file-operation safety rules.

Example phrases:

- `find the 2025 tax folder`
- `show me PDFs in receipts`
- `put this photo in the house pictures folder`
- `add this PDF to the taxes SMB share`
- `create a folder called Warranty in Documents`
- `rename this file to May invoice`

Expected behavior:

- Share selection should be explicit when more than one source matches.
- Folder upload should use staged attachment metadata and agent-side SMB writes.
- Overwrite, move, copy, rename, delete, and cross-share operations require
  confirmation.
- File cards should carry `source_id`, path, directory/file kind, and display
  title so Hank can deep-link into File Server.

### Calendar

Add or harden these intents:

- `calendar.search`: query uploaded EventKit snapshots.
- `calendar.today`, `calendar.tomorrow`, and `calendar.week`: agenda views.
- `calendar.create_event`: create via client tool after confirmation.
- `calendar.update_event`: move, rename, or change time via client tool after
  confirmation.
- `calendar.delete_event`: cancel or delete via client tool after confirmation.

Example phrases:

- `what do I have tomorrow`
- `what events mention Patrick this week`
- `add dentist appointment Friday at 2`
- `create calendar event for Aaron's birthday on July 31`
- `move my 3pm meeting to 4`
- `delete the dentist appointment tomorrow`

Expected behavior:

- Read-only calendar questions should use indexed calendar snapshots first.
- Missing year can default to the next matching date, but the response should say
  the exact date.
- Missing time, duration, or ambiguous event matches should trigger a follow-up.
- Event edits and deletes always require confirmation.

### Project Docs

Add or harden these intents:

- `project_docs.answer`: answer repo and operator questions from current docs.
- `project_docs.search`: return matching doc cards and snippets.
- `project_docs.compare`: explain differences between current docs and archived
  historical phase docs.

Example phrases:

- `what does AGENTS.md say about SMB`
- `how do I deploy this server`
- `what is left in the backend repair plan`
- `where is the Home Assistant runbook`
- `what docs mention pgvector`

Expected behavior:

- Current docs should outrank archived phase docs.
- Answers should cite the doc title/path in the response text.
- If current docs conflict with archive docs, say which one is current.

### Hank Chat History

Add or harden these intents:

- `assistant.memory_search`: search prior Hank chat turns owned by the user.
- `assistant.recall_decision`: answer "what did we decide" style questions.
- `assistant.summarize_thread`: summarize the current or selected conversation.

Example phrases:

- `what did we decide about SMB shares`
- `find the chat where I asked about calendar defaults`
- `summarize this conversation`

Expected behavior:

- Only user-private assistant conversation memory should be searched.
- Responses should distinguish remembered chat from current notes, files, and
  project docs.

## Intent Routing Changes

The current matcher is intentionally simple. The next improvement should be a
shared slot extractor used before the tool registry executes:

- `action`: find, search, list, open, add, create, attach, upload, move, rename,
  delete, summarize, ask, check off
- `object`: note, list, file, folder, photo, PDF, link, event, calendar, project
  doc, chat history
- `payload`: text, URL, staged attachment IDs, date/time, event title, file name
- `destination`: note title, folder path, share name, calendar name, doc path
- `qualifiers`: today, tomorrow, this week, first result, same note, shared,
  personal, latest

Use the extracted slots to choose the tool, then use the model only for answer
phrasing or summarization after retrieval/tool execution.

## Regression Test Matrix

Add table-driven tests for classification and planning:

| Phrase | Expected intent |
| --- | --- |
| `find information in my notes about SMB` | `notes.search` |
| `find the 2025 tax folder` | `files.search` |
| `put this picture in receipts` | `attachment_commit` to note or file clarification |
| `add this PDF to the taxes SMB share` | `attachment_commit` to SMB |
| `add buy coffee to the store list` | `notes.append` |
| `what do I have tomorrow` | `calendar.search` or agenda client tool |
| `create calendar event for dentist Friday at 2` | `calendar.create_event` |
| `move my dentist appointment to 3` | `calendar.update_event` |
| `delete the dentist appointment tomorrow` | `calendar.delete_event` |
| `what does AGENTS.md say about direct SMB exposure` | `project_docs` |
| `what did we decide about calendar defaults` | `assistant.memory_search` |

## Implementation Order

1. Expand prompt text and tests for the current prompt contract.
2. Add the shared slot extractor and regression tests without changing tool
   execution.
3. Improve routing for notes, file/SMB, calendar, project-doc, and chat-history
   phrases.
4. Add missing client-tool request planners for calendar search/update/delete.
5. Add missing server/agent tool planners for `files.list_folder`,
   `files.create_folder`, and attachment upload target selection.
6. Tighten result cards so every successful tool response can deep-link into the
   correct Hank screen.
7. Run full assistant workflow tests and then the repo validation gate.

## Validation

For each implementation slice:

- add table-driven classifier tests in `internal/cloud/assistant_workflow_test.go`
- add workflow tests for confirmation-required writes
- keep file operations behind existing path containment and source authorization
- verify no tool exposes raw SMB, raw SQL, shell access, or secrets to the model
- run `gofmt -w ./cmd ./internal`
- run `go test ./...`
- run `go build ./...`
