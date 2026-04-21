# Hank Remote

`Hank Remote` is the server-side companion for Hank. It runs a public cloud service plus a local agent service on the same machine so the iPhone app can reach Home Assistant, files, and notes remotely over one authenticated API surface.

The cloud service now also serves a management dashboard at `/` for app auth, home creation, agent visibility, and token lifecycle operations.

## What Exists Now

- cloud HTTP auth and home management:
  - `POST /v1/auth/register`
  - `POST /v1/auth/login`
  - `POST /v1/auth/logout`
  - `GET /v1/me`
  - `POST /v1/ws/app-ticket`
  - `GET /v1/home`
  - `PUT /v1/home`
  - `POST /v1/home/invitations/accept`
  - `GET /v1/home/members`
  - `POST /v1/home/members/invitations`
  - `DELETE /v1/home/members/{userID}`
  - `PUT /v1/home/members/{userID}/role`
  - `GET /v1/home/permissions`
  - `PUT /v1/home/permissions`
  - `GET /v1/home/members/{userID}/permissions`
  - `PUT /v1/home/members/{userID}/permissions`
  - `GET /v1/home/agent`
  - `GET /v1/home/agent/tokens`
  - `POST /v1/home/agent/tokens`
  - `DELETE /v1/home/agent/tokens/{tokenID}`
  - `POST /v1/home/files/downloads`
  - `POST /v1/home/files/uploads`
  - `GET /v1/file-transfers/{transferID}`
  - `PUT /v1/file-transfers/{transferID}`
- cloud WebSocket relay:
  - `GET /ws/app`
  - `GET /ws/agent`
- cloud management UI:
  - `GET /`
  - `GET /dashboard`
  - `GET /docs/deployment`
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
  - `files.download`
  - `files.upload`
  - `files.create_directory`
  - `files.rename`
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

## Quick Start

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

In another shell:

```bash
cp configs/cloud.env.example .env
export $(grep -v '^#' .env | xargs)
make run-cloud
```

3. Register the first admin account. The first successful registration auto-creates the singleton Home:

```bash
curl -s http://127.0.0.1:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"aaron@example.com","password":"change-me-123"}'
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
cp configs/agent.env.example .env.agent
export $(grep -v '^#' .env.agent | xargs)
make run-agent
```

5. Connect the app side over `/ws/app` and send routed `app.command` envelopes with `request_id`. Preferred auth is `Authorization: Bearer <session_token>` for HTTP and a short-lived ticket from `POST /v1/ws/app-ticket` for `/ws/app`.

6. Open [http://127.0.0.1:8080/](http://127.0.0.1:8080/) for the management dashboard.

## Development Commands

```bash
make tidy
make fmt
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

This project is deployed as one Docker Compose stack on one machine. That same machine should already have network access to Home Assistant, SMB, local files, and notes. The cloud binds only to `127.0.0.1:8080`, and Cloudflare Tunnel exposes that service externally.

1. Copy the compose env examples:

```bash
cp configs/cloud.compose.env.example .env.cloud
cp configs/agent.compose.env.example .env.agent
mkdir -p data/postgres data/files data/notes
```

2. Keep the internal agent cloud URL in `.env.agent`:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

3. Build and start both services:

```bash
docker compose up --build -d
```

4. Point Cloudflare Tunnel at `http://127.0.0.1:8080`.
5. Open the public URL, register the first admin account, and issue an agent token.
6. Put the issued token into `.env.agent` as `HANK_REMOTE_AGENT_TOKEN`, then restart the `agent` service:

```bash
docker compose up -d --no-deps agent
```

## Current Notes

- file upload and download now use resumable HTTP streaming endpoints coordinated by the cloud over the agent WebSocket; retries can reopen the same transfer with an `offset` query parameter
- Home Assistant, file, and notes access stay on the home agent; the cloud never needs those local credentials
- agent and app auth are separate
- the cloud and agent always run together on the same machine under one Compose stack
- the dashboard issues tokens, but deployment changes are applied by editing `.env.agent` and restarting the `agent` service
- file access can use either the local `./data/files` folder or a direct SMB connection configured in the dashboard
- remote notes now expose additive metadata for `page_type`, preview text, extracted tags, remote search, tag rollups, and kanban board payloads

## Operations Docs

- deployment guide: `docs/deployment.md`
- runbooks:
  - `docs/runbooks/agent-offline.md`
  - `docs/runbooks/auth-failures.md`
  - `docs/runbooks/home-assistant-failures.md`
  - `docs/runbooks/file-transfer-failures.md`
  - `docs/runbooks/storage-failures.md`
  - `docs/runbooks/token-rotation.md`
