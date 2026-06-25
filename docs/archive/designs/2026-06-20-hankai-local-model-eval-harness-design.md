# HankAI Local Model Eval Harness Design

Date: 2026-06-20
Status: Implemented. Current operator usage lives in [hankai-local-model-evals.md](../../hankai-local-model-evals.md); this file is historical design context.

## Goal

Build a repeatable local-model eval harness for HankAI before adding more
intents or hardening existing ones. The harness should prove that HankAI routes
requests through typed tools, uses the local Ollama provider correctly, returns
safe structured results, and does not claim unconfirmed writes.

The first demo target uses the existing Ollama instance at:

```text
http://192.168.86.158:11434
```

No host passwords, session tokens, or generated reports are committed.

## Scope

This design covers:

- connecting the demo Hank server to the LAN Ollama URL for testing
- adding a committed eval CLI for live HankAI checks
- recording local-model/provider details, selected intent diagnostics, result
  cards, confirmation state, latency, and pass/fail status
- adding seed eval cases for project docs, notes, files, calendar, Home
  Assistant, memory, multi-source reads, and write-safety behavior
- using generated reports under `data/hankai-evals/`

This design does not yet add new production intents. Intent expansion follows
after the eval harness can measure the current behavior.

## Architecture

Add a new Go CLI under `tools/hankaieval`.

The CLI uses the same live-server style as `tools/livevalidation`:

- `HANK_REMOTE_LIVE_BASE_URL` points at the Hank demo or local server.
- `HANK_REMOTE_LIVE_SESSION_TOKEN` authenticates as a Hank user.
- optional env vars tune run ID, timeout, expected provider, expected model, and
  required eval case groups.

For each eval case, the tool:

1. creates a fresh assistant session unless the case needs a prior turn
2. optionally uploads calendar snapshots or creates synthetic notes through
   existing APIs
3. sends a prompt to `/v1/home/assistant/sessions/{sessionID}/messages`
4. inspects the assistant run response
5. records diagnostics such as `tool_kind`, `intent_kind`, `query`, cards,
   confirmation state, and latency
6. applies deterministic assertions
7. writes a JSON report to `data/hankai-evals/<run-id>.json`

The harness should prefer assertions over model-specific answer text. Examples:

- project-doc prompts should select `project_docs`
- note search prompts should return at least one note card
- calendar delete prompts should require destructive confirmation
- multi-source read prompts should not require confirmation
- local planner cases should show local planner trace or final diagnostics when
  deterministic routing does not match first

## Demo Ollama Connection

Use the direct LAN URL first:

```text
HANK_REMOTE_OLLAMA_BASE_URL=http://192.168.86.158:11434
```

Validation order:

1. From the demo host, confirm `GET /api/tags` reaches the Ollama endpoint.
2. From the running cloud container, confirm the same endpoint is reachable.
3. Set the cloud environment or assistant settings to use the LAN URL.
4. Verify `/v1/home/assistant/status` reports provider `ollama`, the expected
   chat model, configured embeddings, and pgvector mode.
5. Run `tools/hankaieval` against the public demo URL.

If direct container networking fails, use a later design decision for a proxy or
tunnel. Do not add that fallback in this first pass.

## Eval Case Groups

Initial committed groups:

- `provider`: status endpoint reports Ollama, chat configured, embeddings
  configured, and vector store available.
- `project_docs`: project intent and source-path prompts route to project docs.
- `notes`: note list/search/append-safe planning behavior uses note cards and
  note append results.
- `files`: folder/file search returns file cards with source ID and path.
- `calendar`: today/tomorrow search works from indexed snapshots; update/delete
  prompts require confirmation.
- `homeassistant`: entity status/search prompts route through Home Assistant
  query tooling without exposing raw credentials.
- `memory`: prior-chat prompts search only the current user's assistant memory.
- `safety`: destructive or discard-like prompts require confirmation and do not
  claim completion.
- `multi_source`: read-only synthesis uses retrieval without triggering write
  actions.

Groups can be enabled with `HANK_REMOTE_HANKAI_EVAL_GROUPS`; by default the
harness runs all groups that can be satisfied by the current demo state and marks
missing prerequisites as skipped, not passed.

## Result Format

The JSON report contains:

- run ID, started/finished timestamps, base URL host, and git revision when
  available
- assistant provider status and index stats
- case name, group, prompt, expected behavior, status, latency, and error
- selected diagnostics: tool kind, intent kind, query, card kinds, confirmation
  flags, pending action kind, and destructive flag
- a short text preview with sensitive values redacted

Generated reports stay under `data/hankai-evals/` and remain untracked.

## Safety Rules

- The harness never logs session tokens, passwords, raw secrets, or bearer
  headers.
- Demo data must be synthetic and clearly prefixed when created.
- Destructive live eval cases assert that confirmation is required; they do not
  approve destructive actions.
- File and note mutations should use `_hank_eval/` or clearly named synthetic
  targets only.
- Local model output is treated as untrusted. The harness checks server-side
  typed diagnostics and pending action structure, not just prose.

## Testing

Implementation should add:

- unit tests for eval-case assertion helpers
- focused assistant workflow tests for any new intent behavior introduced later
- live validation through the demo server once the session token is available

Expected local commands when Go is available:

```bash
gofmt -w ./cmd ./internal ./tools
go test ./tools/hankaieval ./internal/cloud
go build ./...
```

Demo validation command shape:

```bash
HANK_REMOTE_LIVE_BASE_URL=https://hankdemo.campbellservers.com \
HANK_REMOTE_LIVE_SESSION_TOKEN="$HANK_REMOTE_LIVE_SESSION_TOKEN" \
HANK_REMOTE_HANKAI_EXPECT_PROVIDER=ollama \
HANK_REMOTE_HANKAI_EXPECT_OLLAMA_URL=http://192.168.86.158:11434 \
go run ./tools/hankaieval
```

## Follow-On Order

1. Build and run the local-model eval harness.
2. Add missing high-value intents using the eval report as a baseline.
3. Harden all intent contracts with deterministic routing tests, confirmation
   tests, and safety assertions.
4. Expand live eval coverage after each intent slice.

## Non-Blocking Future Decisions

- The harness should record the active chat model and only require a specific
  model when `HANK_REMOTE_HANKAI_EXPECT_MODEL` is set.
- Whether project/operator intents such as backup verification should be visible
  to non-admin users in HankAI.
- Whether successful target selections should be learned automatically as user
  aliases or only recorded after explicit user approval.
