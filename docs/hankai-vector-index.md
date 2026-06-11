# HankAI Vector Index

HankAI stores assistant retrieval data in PostgreSQL. Production requires
pgvector; JSON embedding columns are retained only as migration/backfill
evidence and are not a production retrieval fallback. Chat providers do not get raw database access. The cloud searches the
index server-side, filters by the user's HankAI settings and permissions, then
sends selected snippets and cards to the configured model.

## Indexed Sources

- Profile notes: note title, note key, page type metadata, and markdown/body
  text owned by the signed-in user.
- Shared home notes: visible note title, note key, page type metadata, and
  markdown/body text visible to the signed-in user.
- Calendar snapshots: event title, calendar id, location, notes, start time,
  search text, and metadata for device id, external event id, all-day status,
  start, and end.
- Home Assistant state snapshots: entity id, state, JSON attributes, and
  last-changed/last-updated metadata. The agent supplies these over the
  existing outbound command path.
- File index entries: crawled file/folder path, name, source id, directory flag,
  size, modified time, and minimal metadata. File contents are not indexed by
  the current crawler.
- Hank project docs: root markdown files and markdown under `docs/`, including
  operator guides and runbooks from `HANK_REMOTE_PROJECT_DOCS_DIR`.
- HankAI conversation memory: prior user and assistant message text plus result
  card metadata for the same user when conversation memory is enabled.

## Provider Behavior

- Self-hosted model setups use Ollama chat and Ollama embeddings when
  `HANK_REMOTE_OLLAMA_BASE_URL` is configured.
- OpenAI API-key setups use OpenAI chat and OpenAI embeddings. The default
  embedding model is `text-embedding-3-small` unless
  `HANK_REMOTE_OPENAI_EMBEDDING_MODEL` overrides it.
- ChatGPT/Codex subscription linking is used for chat and can use the linked
  OpenAI token for embeddings when Ollama and an explicit OpenAI API key are not
  configured. If that embedding call is unavailable, HankAI falls back locally
  instead of exposing the vector database directly.

## Retrieval Flow

1. HankAI refreshes the enabled source indexes during relevant chat/tool flows.
2. The prompt is embedded with the active embedding provider.
3. The store searches pgvector, then merges lexical matches with vector-ranked
   context.
4. Retrieved items are filtered by HankAI settings and Hank permissions.
5. The model receives only the selected context list, not database credentials,
   raw SQL, raw SMB access, or unfiltered source data.

Use `/v1/home/assistant/status` to inspect the active provider, embedding model,
vector mode, and per-source index counts.
