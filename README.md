# Hank Remote

`Hank Remote` is the server-side companion for Hank. It runs a public cloud service plus a local agent service on the same machine so the iPhone app can reach Home Assistant, files, and notes remotely over one authenticated API surface.

The cloud service now also serves a React/Vite management dashboard at `/` and `/dashboard` for app auth, home creation, agent visibility, token lifecycle operations, Settings, files, notes, HankAI, backup/restore, and troubleshooting. Hank Remote does not currently serve a standalone PWA; the removed `/pwa` route family is documented in `docs/PWA/current-scope.md`.

## What Exists Now

- cloud HTTP auth and home management:
  - `POST /v1/auth/register`
  - `POST /v1/auth/login`
  - `POST /v1/auth/logout`
  - `POST /v1/auth/change-password`
  - `POST /v1/auth/invitations/preview`
  - `POST /v1/auth/invitations/signup`
  - `GET /v1/me`
  - `POST /v1/me/devices/apns`
  - `DELETE /v1/me/devices/{deviceID}/apns`
  - `GET /v1/me/notification-settings`
  - `PUT /v1/me/notification-settings`
  - `GET /v1/me/notes`
  - `POST /v1/me/notes`
  - `GET /v1/me/notes/search`
  - `GET /v1/me/notes/tags`
  - `GET /v1/me/notes/tag-rollup`
  - `GET /v1/me/notes/{noteID}`
  - `PUT /v1/me/notes/{noteID}`
  - `POST /v1/me/notes/{noteID}/append`
  - `DELETE /v1/me/notes/{noteID}`
  - `GET /v1/me/profile`
  - `PUT /v1/me/profile`
  - `GET /v1/me/profile-secret-vault`
  - `PUT /v1/me/profile-secret-vault`
  - `GET /v1/me/profile-backup`
  - `PUT /v1/me/profile-backup`
  - `GET /v1/oauth/openai/status`
  - `GET /v1/oauth/openai/start`
  - `POST /v1/ws/app-ticket`
  - `GET /v1/home`
  - `GET /v1/home/setup-status`
  - `PUT /v1/home`
  - `POST /v1/home/invitations/accept`
  - `GET /v1/home/members`
  - `GET /v1/home/members/invitations`
  - `POST /v1/home/members/invitations`
  - `DELETE /v1/home/members/invitations/{invitationID}`
  - `DELETE /v1/home/members/{userID}`
  - `PUT /v1/home/members/{userID}/role`
  - `PUT /v1/home/members/{userID}/password`
  - `GET /v1/home/permissions`
  - `PUT /v1/home/permissions`
  - `GET /v1/home/members/{userID}/permissions`
  - `PUT /v1/home/members/{userID}/permissions`
  - `GET /v1/home/quick-links`
  - `POST /v1/home/quick-links`
  - `POST /v1/home/quick-links/checks`
  - `PUT /v1/home/quick-links/order`
  - `PUT /v1/home/quick-links/{linkID}`
  - `DELETE /v1/home/quick-links/{linkID}`
  - `GET /v1/home/audit-events`
  - `GET /v1/home/query-telemetry`
  - `GET /v1/home/agent`
  - `POST /v1/home/agent/restart`
  - `GET /v1/home/agent/tokens`
  - `POST /v1/home/agent/tokens`
  - `DELETE /v1/home/agent/tokens/{tokenID}`
  - `GET /v1/home/notes-api-tokens`
  - `POST /v1/home/notes-api-tokens`
  - `DELETE /v1/home/notes-api-tokens/{tokenID}`
  - `POST /v1/home/files/downloads`
  - `POST /v1/home/files/uploads`
  - `GET /v1/home/notes`
  - `GET /v1/home/notes/search`
  - `GET /v1/home/notes/tags`
  - `GET /v1/home/notes/tag-rollup`
  - `GET /v1/home/notes/{noteID}`
  - `PUT /v1/home/notes/{noteID}`
  - `POST /v1/home/notes/{noteID}/append`
  - `DELETE /v1/home/notes/{noteID}`
  - `GET /v1/home/sync`
  - `GET /v1/home/service-profiles`
  - `PUT /v1/home/service-profiles/{serviceType}`
  - `GET /v1/home/storage/status`
  - `GET /v1/home/storage/config`
  - `PUT /v1/home/storage/config`
  - `GET /v1/home/storage/events`
  - `DELETE /v1/home/storage/events`
  - `POST /v1/home/storage/backup`
  - `POST /v1/home/storage/restore-test`
  - `POST /v1/home/storage/restore-primary`
  - `GET /v1/home/assistant/status`
  - `GET /v1/home/assistant/settings`
  - `PUT /v1/home/assistant/settings`
  - `GET /v1/home/assistant/models`
  - `GET /v1/home/assistant/sessions`
  - `POST /v1/home/assistant/sessions`
  - `GET /v1/home/assistant/sessions/{sessionID}`
  - `DELETE /v1/home/assistant/sessions/{sessionID}`
  - `GET /v1/home/assistant/sessions/{sessionID}/messages`
  - `POST /v1/home/assistant/sessions/{sessionID}/messages`
  - `DELETE /v1/home/assistant/sessions/{sessionID}/attachments/{attachmentID}/discard`
  - `GET /v1/home/assistant/runs/{runID}`
  - `POST /v1/home/assistant/runs/{runID}/confirm`
  - `POST /v1/home/assistant/runs/{runID}/client-tool-results`
  - `PUT /v1/home/assistant/calendar-index`
  - `GET /v1/home/assistant/logs`
  - `GET /v1/home/assistant/media-settings`
  - `PUT /v1/home/assistant/media-settings`
  - `GET /v1/home/assistant/media-image`
  - `GET /v1/home/assistant/media-jobs/{jobID}`
  - `POST /v1/home/assistant/media-jobs/{jobID}/cancel`
  - `GET /v1/home/file-jobs`
  - `GET /v1/home/file-jobs/{jobID}`
  - `POST /v1/home/file-jobs/{jobID}/cancel`
  - `POST /v1/home/file-jobs/{jobID}/retry`
  - `POST /v1/home/file-jobs/{jobID}/rollback`
  - `GET /v1/file-transfers/{transferID}`
  - `GET /v1/file-transfers/{transferID}/status`
  - `PUT /v1/file-transfers/{transferID}`
- cloud WebSocket relay:
  - `GET /ws/app`
  - `GET /ws/agent`
- cloud management UI:
  - `GET /`
  - `GET /dashboard`
  - `GET /dashboard/hank`
  - `GET /dashboard/home-assistant`
  - `GET /dashboard/profile-notes`
  - `GET /dashboard/file-server`
  - `GET /dashboard/settings`
  - `GET /dashboard/settings/home`
  - `GET /dashboard/settings/quick-links`
  - `GET /dashboard/settings/people`
  - `GET /dashboard/settings/connections`
  - `GET /dashboard/settings/ai`
  - `GET /dashboard/settings/apps`
  - `GET /dashboard/settings/backups`
  - `GET /dashboard/settings/recovery`
  - `GET /dashboard/settings/join-home`
  - `GET /docs/deployment`
- current browser surface:
  - the operator dashboard is the supported browser UI
  - React/TypeScript source lives in `web/dashboard`; Vite builds the embedded app into `internal/cloud/ui/react`
  - `/dashboard/settings/*` exposes Settings sections as direct authenticated routes. Dashboard navigation does not use iframe composition; only file preview content uses sandboxed iframes.
  - `/pwa`, `/pwa/`, `/pwa/sw.js`, `/pwa/manifest.webmanifest`, and `/assets/site.webmanifest` are intentionally not served
- cloud operations endpoints:
  - `GET /healthz`
  - `GET /readyz`
  - `GET /metrics`
- PostgreSQL-backed persistence for users, homes, agents, agent tokens, and app sessions
- request/response routing from app sessions to the singleton deployment home agent
- agent-side command handling for:
  - `system.ping`
  - `homeassistant.health`
  - `homeassistant.fetch_states`
  - `homeassistant.fetch_state`
  - `homeassistant.call_service`
  - `files.list`
  - `files.stat`
  - `files.search`
  - `files.download`
  - `files.upload`
  - `files.create_directory`
  - `files.rename`
  - `files.move`
  - `files.move_cancel`
  - `files.move_rollback`
  - `files.delete`
  - `notes.list`
  - `notes.fetch`
  - `notes.save`
  - `notes.rename`
  - `notes.delete`
  - `notes.sync`
  - `notes.search`
  - `notes.tags`
  - `notes.tag_rollup`
  - `media.settings_status`
  - `media.settings_apply`
  - `media.download_jobs`
  - `media.download_cancel`
  - `media.search`
  - `media.plan_download`
  - `media.download_start`
  - `media.download_status`
  - `media.image_fetch`
  - `apps.list`
  - `apps.package_preview`
  - `apps.package_activate`
  - `apps.config_status`
  - `apps.config_apply`
  - `apps.invoke`
  - `config.status`
  - `config.apply`

Installed first-party `.hankapp` packages can add HankAI slash commands without rebuilding HankServerside when they use the existing app runtime contract. Admins import and configure packages in Settings > Apps. Each installed app has one access mode: `admins_only` or `home_members`; when `home_members` is selected, every command in that app is available to regular home members.

HankServerside is the stable OS/runtime for Hank remote access. Hank apps are installable first-party extensions for optional workflows on top of that runtime. The `.hankapp` package format, manifest schema, Settings > Apps rendering, and `apps.*` commands are compatibility surfaces; breaking changes require a new schema version or a documented migration path. See `docs/hank-app-platform-contract.md`.

## Project Layout

- `cmd/hank-remote-cloud`: public cloud service
- `cmd/hank-remote-agent`: local agent service
- `internal/protocol`: shared wire contract
- `internal/cloud`: auth, routing, relay, readiness, and metrics
- `internal/agent`: reconnect loop and command dispatch
- `internal/store`: PostgreSQL persistence
- `internal/domain`: shared cloud-side models
- `internal/observability`: metrics aggregation
- `docs/architecture.md`: system design notes
- `docs/project-knowledge-index.md`: central map of markdown that HankAI indexes as project docs

## Quick Start

For a production-style first install on one server, use the guided Compose bootstrap:

```bash
sudo mkdir -p /srv/hank-remote
sudo chown "$USER":"$USER" /srv/hank-remote
cd /srv/hank-remote
git clone <your-hankserverside-repo-url> HankServerside
cd /srv/hank-remote/HankServerside
scripts/bootstrap-first-run.sh
scripts/doctor.sh
```

That creates `.env.cloud`, runs migrations, starts `postgres`, `cloud`, and `db-ops`, and verifies health/readiness. The default host bind is `127.0.0.1:18080` for Cloudflare Tunnel or a local reverse proxy. After the first admin registers, create the connector setup file in the dashboard and run `scripts/install-agent-env.sh` with the generated `.env.agent` block.

For local development without Compose:

1. Install dependencies:

```bash
make tidy
```

2. Start PostgreSQL, then start the cloud service:

```bash
docker run --rm --name hankremote-postgres \
  -e POSTGRES_DB=hankremote \
  -e POSTGRES_USER=hankremote \
  -e POSTGRES_PASSWORD=hankremote \
  -p 5432:5432 \
  postgres:17-alpine
```

In another shell, point the cloud process at that local database:

```bash
export HANK_REMOTE_CLOUD_ADDR=:8080
export HANK_REMOTE_CLOUD_DATABASE_URL='postgres://hankremote:hankremote@127.0.0.1:5432/hankremote?sslmode=disable'
export HANK_REMOTE_DB_OPS_INTENT_SECRET='local-dev-db-ops-intent-secret'
export HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS='local-dev-backup-passphrase'
export HANK_REMOTE_SECRET_ENCRYPTION_KEY='local-dev-secret-encryption-key'
make run-cloud
```

For throwaway local-only experiments, `HANK_REMOTE_ALLOW_PLAINTEXT_SECRETS=true` explicitly permits plaintext secret storage. Do not use that opt-out for shared or production-like installs.

3. Register the first admin account. The first successful registration auto-creates the singleton Home:

```bash
curl -s http://127.0.0.1:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"aaron@example.com","password":"change-me-123"}'
```

After first setup, additional users join through admin-created invitations.
Create an invite from Settings > People, share the one-time join URL or invite
code, and have the invitee open `/join` to create their own account and set
their permanent password. Admins can reset an existing member password from
Settings > People; reset actions revoke that user's active sessions and can
require a password change on next login. Break-glass CLI reset is available as:

```bash
hank-remote-cloud users reset-password --email user@example.com --force-change
```

Then issue an agent token:

```bash
curl -s http://127.0.0.1:8080/v1/home/agent/tokens \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"agent_id":"home-main","name":"Main Agent"}'
```

4. Start the home agent:

```bash
# Create .env.agent from the dashboard-generated block or the example shape in docs/deployment.md.
export $(grep -v '^#' .env.agent | xargs)
make run-agent
```

5. Connect the app side over `/ws/app` and send routed `app.command` envelopes with `request_id`. Preferred auth is `Authorization: Bearer <session_token>` for HTTP and a short-lived ticket from `POST /v1/ws/app-ticket` for `/ws/app`.

6. Open [http://127.0.0.1:8080/](http://127.0.0.1:8080/) for the management dashboard.

## Remote MCP endpoint (optional)

Hank Remote can expose an authenticated [MCP](https://modelcontextprotocol.io) endpoint so AI apps
(ChatGPT, Claude) can read this project's docs and read/write the signed-in user's notes. It is
**off by default**; enable with `HANK_REMOTE_MCP_ENABLED=true` and set
`HANK_REMOTE_PUBLIC_BASE_URL=https://your-host`. This adds an OAuth 2.1 surface
(`/.well-known/oauth-*`, `/v1/oauth/mcp/{register,authorize,token}`) and the MCP endpoint at
`POST /v1/mcp`; only docs (read) and profile notes (read/write/delete) are exposed. See
[docs/mcp.md](docs/mcp.md) for routes, scopes, and connecting a client.

The dashboard can also configure live, read-only MCP Context Sources from existing File Server
shares. The home agent performs bounded project file listing, text search, and reads without
giving MCP general file-management access. Notes and notebooks can be marked with a lock icon to
exclude them and their notebook children from MCP.

## Development Commands

```bash
make tidy
make fmt
make frontend-check
make build
make run-cloud
make run-agent
go test ./...
```

## Local Docker Shims

If Docker Desktop is not installed but `podman` is available, this repo includes local `docker` and `docker-compose` wrappers under `bin/`.

Enable them in your current shell with:

```bash
source scripts/use-local-docker.sh
```

After that, repo-local commands like `docker compose up --build` will prefer the local Docker CLI and fall back to Podman only if Docker is unavailable.

## Docker Compose

This project is deployed as one Docker Compose stack on one machine. That same machine should already have network access to Home Assistant, SMB, local files, and notes. The bootstrap path publishes the cloud on `127.0.0.1:18080` by default for Cloudflare Tunnel or a same-host reverse proxy. Use `0.0.0.0` only when the server network must reach the cloud port directly.

Full setup and deployment docs live in `docs/deployment.md`.

1. The Compose stack uses private repo-root env files:

- `.env.cloud` before first boot
- `.env.agent` after the dashboard creates the agent setup token

Docker creates the default persistent volumes automatically for PostgreSQL, local files, and notes.

Use `docker compose --env-file .env.cloud ...` for deployment commands so Compose sees host bind and port overrides from `.env.cloud`.

2. Keep the internal agent cloud URL as:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
HANK_REMOTE_AGENT_CONFIG_PATH=/app/.env.agent
```

If host port `18080` is already taken, set this in `.env.cloud` and keep the agent URL unchanged:

```env
HANK_REMOTE_CLOUD_HOST_PORT=18081
```

The agent service is behind the `agent` Compose profile, so it does not start until you issue a real token.

3. Build, migrate, and start the first-boot services:

```bash
scripts/bootstrap-first-run.sh
scripts/doctor.sh
```

4. Point Cloudflare Tunnel or your reverse proxy at your chosen host bind, for example `http://127.0.0.1:18080` or `http://<server-ip>:18080`.
5. Open the public URL, register the first admin account, and issue an agent token. Public registration is disabled after this first setup.
6. Copy the generated `.env.agent` block from the dashboard, then install it and start the agent profile:

```bash
pbpaste | ssh <server-user>@<server-host> 'cd /srv/hank-remote/HankServerside && scripts/install-agent-env.sh'
```

## Current Notes

- file upload and download now use resumable HTTP streaming endpoints coordinated by the cloud over the agent WebSocket; retries can reopen the same transfer with an `offset` query parameter
- Home Assistant, file, and notes access stay on the home agent; the cloud never needs those local credentials
- agent and app auth are separate
- the cloud and agent run on the same machine under one Compose stack, but the agent starts only after a token exists
- the dashboard issues tokens and generates the `.env.agent` file content; deployment changes are applied by editing `.env.agent` and refreshing the `agent` profile
- file access can use the Docker-managed `hank_agent_files` volume, one or more direct SMB shares, or host folders (directories on the home connector itself), all configured in dashboard Settings; SMB env storage uses `HANK_REMOTE_SMB_SHARES_JSON` and host folders use `HANK_REMOTE_AGENT_FILES_ROOTS_JSON`
- remote notes now expose additive metadata for `page_type`, preview text, extracted tags, remote search, tag rollups, and kanban board payloads

## Operations Docs

- deployment and setup guide: `docs/deployment.md`
- single-host runbook: `docs/runbooks/single-host-compose.md`
- runbooks:
  - `docs/runbooks/agent-offline.md`
  - `docs/runbooks/auth-failures.md`
  - `docs/runbooks/home-assistant-failures.md`
  - `docs/runbooks/file-transfer-failures.md`
  - `docs/runbooks/storage-failures.md`
  - `docs/runbooks/token-rotation.md`
