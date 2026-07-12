# Remote MCP endpoint (optional)

Hank Remote can expose a [Model Context Protocol](https://modelcontextprotocol.io) endpoint so
desktop/web AI apps (ChatGPT, Claude) can read this project's documentation and read/write the
signed-in user's Hank notes over an authenticated remote connection.

It is **off by default** and gated by `HANK_REMOTE_MCP_ENABLED`. When disabled, all of the routes
below return 404 and nothing else changes.

Scope is deliberately narrow: **project docs + a read-only source snapshot**, optional
**live read-only project context sources**, and **profile notes (read/write/delete)**. Home
Assistant, general SMB/file operations, calendars, the secret vault, and shared/home notes are
**not** exposed.

The `code-reference/` source snapshot is a build-time copy of the Go source (`cmd/`, `internal/`,
`go.mod`) shipped in the image by `Dockerfile.server` — it is **not** the running process's source,
and `.env*`, `.git`, `data/`, and `*.db` are excluded by `.dockerignore` so nothing sensitive is
copied. It shares the `docs:read` scope and the same path-containment + extension allowlist as the
docs tools.

> A local stdio variant for tools that run MCP servers on your own machine (Claude Desktop, Claude
> Code, Cursor) lives outside this repo in `Projects/MCP/`. This document covers the in-server
> remote endpoint, which is what ChatGPT requires.

## Enabling it

| Env var | Default | Purpose |
|---------|---------|---------|
| `HANK_REMOTE_MCP_ENABLED` | `false` | Turns the endpoint + OAuth routes on. |
| `HANK_REMOTE_PUBLIC_BASE_URL` | _(request host)_ | Public HTTPS origin (e.g. `https://hank.example.com`). Used as the OAuth issuer and in discovery metadata. **Required for ChatGPT**, whose servers reach the endpoint from outside your network. |
| `HANK_REMOTE_MCP_DOCS_DIR` | falls back to `HANK_REMOTE_PROJECT_DOCS_DIR`, then `.` | Directory the docs tools read from. In the Docker image this is `/app`, where `README.md`, `AGENTS.md`, `SERVER_SYNC.md`, `docs/`, `schemas/`, and a `code-reference/` source snapshot are shipped. |

## Routes

| Route | Auth | Purpose |
|-------|------|---------|
| `GET /.well-known/oauth-protected-resource` | public | RFC 9728 resource metadata (points at the authorization server). |
| `GET /.well-known/oauth-authorization-server` | public | RFC 8414 authorization-server metadata. |
| `POST /v1/oauth/mcp/register` | public | RFC 7591 Dynamic Client Registration (rate-limited). |
| `GET/POST /v1/oauth/mcp/authorize` | dashboard session | Consent screen; POST issues a short-lived auth code. |
| `POST /v1/oauth/mcp/token` | PKCE | Exchanges the code (and refresh tokens) for access tokens. |
| `POST /v1/mcp` | Bearer access token | The MCP JSON-RPC endpoint (`initialize`, `tools/list`, `tools/call`). |

## OAuth model

- OAuth 2.1, **public clients with PKCE** (S256) — ChatGPT and Claude self-register via DCR and
  authenticate with PKCE, so there is no shared client secret.
- `GET /authorize` reuses the existing **dashboard session** to identify the user, then renders a
  consent screen. The consent POST is CSRF-protected (double-submit token). If the browser is not
  signed into the Hank dashboard, the page asks the user to sign in first.
- Access tokens (1h) and refresh tokens (30d, rotating) are stored **hashed**; raw values are
  never persisted or logged. Codes are single-use with a 90s TTL.
- Tables: `mcp_oauth_clients`, `mcp_oauth_auth_codes`, `mcp_oauth_tokens` (migration `000016`).

## Tools and scopes

| Tool | Scope required |
|------|----------------|
| `list_docs`, `search_docs`, `read_doc` | `docs:read` |
| `list_notes`, `search_notes`, `list_note_tags`, `get_note` | `notes:read` |
| `create_note`, `update_note` | `notes:write` |
| `append_note` | `notes:append` or `notes:write` |
| `delete_note` | `notes:delete` |
| `list_context_sources`, `list_context_files`, `search_context`, `read_context_file` | `docs:read` |

The user picks which scopes to grant on the consent screen; `notes:delete` is unchecked by
default. Note access is always scoped to the **authenticated user's own profile notes** and is
audited (`mcp.tool_called`, `mcp_oauth.*`).

### Note and notebook exclusions

The lock icon in Hank Notes marks a note or notebook as excluded from MCP. This is an AI privacy
marker, not encryption or a user-facing security lock. An excluded notebook also excludes every
note currently inside it. Moving an otherwise unlocked note out makes it visible to MCP again.
MCP list, search, tag, fetch, append, update, and delete operations treat excluded records as
nonexistent; normal Hank Notes and HankAI continue to see them.

### Live project context sources

Settings > AI & MCP > MCP Context Sources grants MCP read-only access to a project folder on an
existing File Server share. Choose a display name, share, and share-relative folder, then use
**Test** to verify live access. The source is temporarily unavailable while the agent or share is
offline.

The home agent rejects traversal, hidden and dependency/build trees, `.env*`, binaries,
oversized files, and symlink escapes. MCP receives source-relative paths and has no context write,
rename, upload, or delete tool. Search returns at most 50 matches and is bounded to 10,000 files
and 20 MB of inspected text; individual reads are limited to 400 KB.

## Connecting a client

1. Set `HANK_REMOTE_MCP_ENABLED=true` and `HANK_REMOTE_PUBLIC_BASE_URL=https://your-host`, restart
   the cloud service.
2. Sign into the Hank dashboard in your browser.
3. **ChatGPT**: Settings → Connectors → add a custom connector pointing at
   `https://your-host/v1/mcp`. Complete the OAuth/consent prompt.
4. **Claude** (web/desktop): Settings → Connectors → add a custom connector with the same URL.

## Privacy

When used from a cloud assistant, the **content of the docs, the `code-reference/` source
snapshot, and the notes accessed is sent to that assistant's provider** (e.g. OpenAI for ChatGPT).
Notes can be personal even though other personal-data surfaces are excluded, and the snapshot
exposes your application source. Grant a read-only set of scopes if you only want to pull context
in, and prefer Claude if you do not want note or source content reaching OpenAI.

## Security checklist (for changes here)

- Every route is authenticated or intentionally public (per the table above).
- Consent writes are CSRF-protected; tokens/codes are hashed at rest and never logged.
- Tool calls enforce per-user scope server-side; only profile-note operations are reachable.
- Schema changes go through `internal/migrations` (no startup mutations).
