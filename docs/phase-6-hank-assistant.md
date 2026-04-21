# Phase 6: Hank Assistant

## Goal

Add a first-class `Hank` chat tab powered by a local Ollama model running alongside `hankserverside`.

The assistant should:
- answer grounded questions over the user's Hank data
- search notes, calendar content, and SMB files
- perform explicit actions such as appending to a note or creating a calendar event
- return structured UI targets such as a note ID or SMB folder path so Hank can deep-link into the right screen

This phase should extend the current cloud and agent architecture instead of creating a second backend.

## Product Scope

Initial user stories:
- "Add dog food to the Store list."
- "Add Aaron's birthday to July 31."
- "Find the tax documents 2025 folder."

Initial capabilities:
- read notes
- append to or create notes
- search calendar events
- create calendar events
- search SMB folders and files
- return actionable targets for Hank navigation

Out of scope for the first implementation:
- arbitrary code execution
- raw SQL access from the model
- direct SMB access from the model
- general internet browsing
- autonomous background changes without explicit user intent

## High-Level Design

```text
Hank iPhone App
    |
    | HTTPS / streaming chat API
    v
Hank Remote Cloud
    |
    | chat orchestration + retrieval + auth + audit
    |              |
    |              | Ollama HTTP
    |              v
    |         local LLM + embedding model
    |
    | app command relay / agent command relay
    v
Hank Remote Agent
    |                |
    | notes/files    | SMB / notes storage
    v                v
local capabilities   NAS / filesystem

Hank iPhone App
    |
    | client tool bridge for EventKit-only operations
    v
device calendars
```

Core rule:
- `hankserverside` is the assistant runtime and policy layer
- Ollama is only the reasoning engine
- tools stay explicit, typed, permission-checked, and audited

## Why Ollama Should Not Query Postgres Directly

The model can only access Postgres if we wire that access through code. We should not give the model direct database credentials or raw SQL ability.

Instead:
- `hankserverside` reads and writes Postgres
- `hankserverside` chooses which indexed content to send to the model
- `hankserverside` executes approved tools and returns results to the model
- every mutation is logged with actor, tool, arguments, and outcome

This keeps:
- authorization in one place
- request validation in one place
- audit history complete
- failure handling deterministic

## Storage Direction

Use the existing PostgreSQL deployment as both:
- relational store
- vector store

Do not add a second database for v1.

Postgres extensions to enable:
- `pgvector`
- `pg_trgm`
- optionally `unaccent`

This gives us:
- vector similarity search for note and calendar content
- trigram fuzzy search for titles, note names, and SMB paths
- one backup, one migration path, one operational surface

## Assistant Subsystems

### 1. Chat Orchestrator

Runs inside the cloud service and owns:
- session creation
- message persistence
- retrieval assembly
- tool planning
- tool execution
- response streaming
- final assistant answer formatting

This layer should be deterministic around tool use even when the model output is not.

### 2. Retrieval and Indexing

Build a unified searchable corpus from:
- profile notes and shared Home notes
- device calendar snapshots supplied by Hank
- SMB file and folder index snapshots supplied by the agent

Retrieval should be hybrid:
- exact and prefix match
- trigram fuzzy match
- vector similarity
- recency and source-type re-ranking

### 3. Tool Runtime

Typed tools only. No free-form shell or SQL tools.

Initial server-side tools:
- `notes.search`
- `notes.fetch`
- `notes.append_text`
- `notes.create`
- `files.search`
- `files.stat`
- `files.list`

Initial client-side tools:
- `calendar.search`
- `calendar.create_event`
- `calendar.update_event`

Client-side is required because current Hank calendar access is device EventKit-based, not server-owned.

### 4. Audit and Safety

Persist:
- user prompt
- retrieved context IDs
- tool calls
- tool call arguments
- tool results summary
- final assistant answer
- mutation confirmation state

High-risk mutations should require confirmation before execution.

Initial confirmation-required actions:
- creating or editing calendar events
- creating new notes when the target note match is ambiguous
- any future delete or destructive file action

Low-risk explicit actions can be auto-run:
- append a short item to a uniquely matched shopping list note

## Data Model

Suggested schema names are intentionally plain so they can live beside the current store code without introducing a second naming style.

### Chat Tables

`assistant_sessions`
- `id uuid primary key`
- `home_id text not null`
- `user_id text not null`
- `title text not null default ''`
- `last_message_at timestamptz not null`
- `created_at timestamptz not null`
- `updated_at timestamptz not null`

`assistant_messages`
- `id uuid primary key`
- `session_id uuid not null`
- `role text not null`
- `status text not null`
- `content_json jsonb not null`
- `model_name text not null`
- `prompt_tokens integer`
- `completion_tokens integer`
- `created_at timestamptz not null`

`assistant_runs`
- `id uuid primary key`
- `session_id uuid not null`
- `message_id uuid not null`
- `state text not null`
- `requires_client_tools boolean not null default false`
- `requires_confirmation boolean not null default false`
- `created_at timestamptz not null`
- `completed_at timestamptz`

`assistant_tool_calls`
- `id uuid primary key`
- `run_id uuid not null`
- `tool_name text not null`
- `tool_scope text not null`
- `arguments_json jsonb not null`
- `result_json jsonb`
- `status text not null`
- `started_at timestamptz not null`
- `completed_at timestamptz`

### Retrieval Tables

`assistant_documents`
- `id uuid primary key`
- `home_id text not null`
- `user_id text`
- `source_type text not null`
- `source_id text not null`
- `title text not null`
- `path text not null default ''`
- `canonical_uri text not null`
- `metadata_json jsonb not null`
- `search_text text not null`
- `updated_at timestamptz not null`

`assistant_chunks`
- `id uuid primary key`
- `document_id uuid not null`
- `chunk_index integer not null`
- `content text not null`
- `content_tsv tsvector`
- `embedding vector(768)`
- `token_count integer not null`
- `updated_at timestamptz not null`

`assistant_file_index`
- `id uuid primary key`
- `home_id text not null`
- `service_profile_id text`
- `path text not null`
- `name text not null`
- `is_directory boolean not null`
- `size_bytes bigint`
- `modified_at timestamptz`
- `search_text text not null`
- `embedding vector(768)`
- `metadata_json jsonb not null`
- `updated_at timestamptz not null`

`assistant_calendar_entries`
- `id uuid primary key`
- `home_id text not null`
- `user_id text not null`
- `device_id text not null`
- `external_event_id text not null`
- `calendar_id text not null`
- `title text not null`
- `location text not null default ''`
- `notes text not null default ''`
- `starts_at timestamptz not null`
- `ends_at timestamptz not null`
- `is_all_day boolean not null`
- `search_text text not null`
- `embedding vector(768)`
- `metadata_json jsonb not null`
- `updated_at timestamptz not null`

Recommended indexes:
- unique index on `(session_id, created_at, id)` for chat ordering
- unique index on `(source_type, source_id)` in `assistant_documents`
- unique index on `(document_id, chunk_index)` in `assistant_chunks`
- unique index on `(home_id, path)` in `assistant_file_index`
- unique index on `(user_id, device_id, external_event_id)` in `assistant_calendar_entries`
- GIN trigram indexes on title, path, and search text
- vector indexes once corpus size justifies them

## Embedding Strategy

Use a dedicated embedding model in Ollama from day one.

Do not use the chat model itself for embeddings unless its embedding support and dimensions are explicitly stable for this deployment.

Recommended pattern:
- chat model via Ollama `/api/chat`
- embedding model via Ollama `/api/embeddings`

Store:
- model name
- embedding dimension
- embedding version

in metadata so re-embedding jobs can detect drift.

Re-index triggers:
- note save or note share change
- file index refresh from agent
- calendar snapshot refresh from app
- embedding model change

## Retrieval Strategy

### Notes

Index:
- note title
- note path or hierarchy breadcrumbs when available
- note body chunks
- tags or page type metadata when available

Query plan:
1. exact or trigram match on note title
2. vector search over note chunks
3. merge and re-rank by title confidence, share visibility, and recency

### SMB Files

Index:
- full normalized path
- basename
- folder/file flag
- extension
- modified timestamp

Query plan:
1. exact path and basename match
2. trigram on normalized path and name
3. optional path embedding for fuzzy natural-language phrasing

For SMB, trigram should carry most of the workload. Embeddings are useful, but ranking should still favor exact or near-exact path matches.

### Calendar

Index:
- event title
- location
- notes
- start and end timestamps
- calendar title and device metadata

Query plan:
1. structured date parsing from the user request
2. title search
3. optional vector search over event title and notes

Calendar actions should rely primarily on parsed dates plus explicit event fields, not embeddings alone.

## Tool Contract Design

The assistant runtime should support two tool scopes:
- `server`
- `client`

### Server Tools

Executed immediately inside `hankserverside`.

Examples:
- `notes.search`
- `notes.fetch`
- `notes.append_text`
- `files.search`
- `files.list`

### Client Tools

Executed by the connected Hank app because the data or permission boundary is device-local.

Initial client tools:
- `calendar.search`
- `calendar.create_event`
- `calendar.update_event`

Future client tools may include:
- opening a note or folder locally after the response is complete
- invoking native share sheet actions

## API and Streaming Contract

Use HTTP for session and history management and a streaming response for active chat turns.

### Suggested HTTP Routes

- `GET /v1/home/assistant/sessions`
- `POST /v1/home/assistant/sessions`
- `GET /v1/home/assistant/sessions/{sessionID}`
- `GET /v1/home/assistant/sessions/{sessionID}/messages`
- `POST /v1/home/assistant/sessions/{sessionID}/messages`
- `POST /v1/home/assistant/runs/{runID}/confirm`
- `POST /v1/home/assistant/runs/{runID}/client-tool-results`
- `GET /v1/home/assistant/runs/{runID}`

### Message Submit Request

```json
{
  "content": "Add dog food to the Store list.",
  "client_capabilities": {
    "calendar_search": true,
    "calendar_create": true
  },
  "device_context": {
    "device_id": "ios-device-1",
    "timezone": "America/Chicago"
  }
}
```

### Streaming Events

Suggested event types:
- `run.started`
- `retrieval.completed`
- `assistant.delta`
- `tool_call.requested`
- `tool_call.completed`
- `client_tool.required`
- `confirmation.required`
- `assistant.completed`
- `run.failed`

This keeps the app responsive without forcing it to infer state from free-form text.

## Example Flows

### 1. Append To Store List

1. App sends user message.
2. Server retrieves likely note matches from trigram plus embeddings.
3. Model selects `notes.append_text` against the unique top match.
4. Server executes note mutation.
5. Server returns assistant text plus structured target:
   - note ID
   - note title
   - appended text

If note match confidence is low:
- do not mutate
- return confirmation choices instead

### 2. Add Birthday To Calendar

1. App sends user message and device capability flags.
2. Server parses date intent and requests `calendar.search` or proceeds straight to create if unambiguous.
3. App executes EventKit tool locally.
4. App posts client tool result back to server.
5. Server finalizes assistant response and stores the completed run.

The server remains the orchestrator, but the app remains the only writer to EventKit calendars.

### 3. Find SMB Folder

1. Server searches `assistant_file_index`.
2. If needed, server calls `files.stat` through the agent for top-ranked candidates.
3. Server returns:
   - canonical SMB path
   - display name
   - route target metadata for the app

## Agent Changes

The agent already exposes `notes.*` and `files.*` commands. Extend this path instead of creating a second agent service.

Planned additions:
- `files.search`
- optional `files.index_snapshot`
- optional `notes.index_snapshot`

Preferred pattern:
- the agent owns raw SMB traversal
- the cloud owns Postgres indexing and retrieval

The agent should not own vector search state. That keeps index lifecycle, ranking, and chat audit in one place.

## App-to-Cloud Calendar Bridge

Because Hank calendar support is currently EventKit-based in the app, the app must participate in the assistant run when a calendar tool is required.

Required app behaviors:
- publish local calendar snapshots for indexing
- execute client tool requests for read/write calendar actions
- return structured tool results

Suggested supporting routes:
- `PUT /v1/home/assistant/calendar-index`
- `POST /v1/home/assistant/runs/{runID}/client-tool-results`

The server should treat calendar snapshots as per-user data, not Home-shared data, unless the product later adds explicit sharing semantics.

## Permissions

Assistant access should inherit the existing Home permission model.

Recommended feature gates:
- `notes` permission controls assistant note retrieval and note mutations
- `files` permission controls assistant SMB retrieval
- calendar remains per-user and should require authenticated profile ownership

Later, if the assistant becomes a separately controllable surface, add:
- `assistant` Home permission

For now, the assistant should only expose tools the current user is already allowed to use.

## Safety Rules

- no raw SQL generation
- no shell execution
- no direct filesystem writes outside typed agent commands
- no destructive file actions in v1
- no note deletes in the assistant v1 path
- require confirmation for ambiguous or destructive operations
- store all tool invocations and outcomes

## Rollout Plan

### Step 1: Retrieval Foundation

- enable Postgres extensions
- add assistant schema and migrations
- index notes
- index SMB file metadata
- add calendar snapshot ingestion

### Step 2: Read-Only Chat

- session APIs
- streaming assistant responses
- search-backed answers with citations to Hank entities

### Step 3: Safe Actions

- note append and create
- calendar create via client tools
- SMB deep-link results

### Step 4: Ranking and Quality

- hybrid ranker tuning
- prompt and tool policy hardening
- re-index jobs
- better ambiguity handling

## Open Decisions

- whether assistant chat history is Home-scoped or user-scoped inside one Home
- whether note append should auto-run when confidence is high or always ask confirmation
- whether calendar indexing should be incremental or full-snapshot per app refresh
- whether assistant answers should expose source snippets inline or only on an inspect sheet

## Recommendation Summary

- use the existing Postgres instance with `pgvector`
- keep Ollama behind `hankserverside`, not beside it as a privileged database client
- keep the cloud service as the orchestrator
- keep the agent focused on notes/files capability execution
- keep EventKit operations in the Hank app through a client-tool bridge
- launch with hybrid retrieval from the start so we do not need a schema migration later
