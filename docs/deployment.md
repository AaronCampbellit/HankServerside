# Deployment

Hank Remote is deployed as one Docker Compose stack on one machine.

That machine runs:

- `cloud`: the public HTTPS/WebSocket entrypoint
- `agent`: the local worker that talks to Home Assistant, SMB, files, and notes
- `postgres`: cloud-side persistence

This is a singleton deployment now:

- one deployment-scoped `Home`
- one first registered admin account
- one connected agent surface
- additional users added as members or admins under that same Home

## Topology

```text
iPhone App -> Cloudflare Tunnel -> Hank Remote Cloud -> internal Docker network -> Hank Remote Agent
```

The cloud binds only to `127.0.0.1:8080`.
External traffic should come through Cloudflare Tunnel.

## Prerequisites

- a Linux server with Docker Engine and Docker Compose installed
- the server can already reach the local systems the agent needs
  - Home Assistant
  - SMB share, if used
  - local file and note storage
- a Cloudflare Tunnel that can proxy HTTP and WebSocket traffic to `http://127.0.0.1:8080`
- a fresh or already-consolidated database
  - this version supports only one Home per deployment
  - if the database already contains more than one row in `homes`, startup will fail until you consolidate it

If Docker is not installed yet, do that before continuing. A typical fresh Ubuntu bootstrap is:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl git
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"
newgrp docker
docker --version
docker compose version
```

## Files Used

- `.env.cloud`
- `.env.agent`
- `docker-compose.yml`

## Environment

Cloud:

- `HANK_REMOTE_CLOUD_ADDR`
- `HANK_REMOTE_CLOUD_DATABASE_URL`
- `POSTGRES_DB`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `HANK_REMOTE_SESSION_TTL_SECONDS`
- `HANK_REMOTE_REQUEST_TIMEOUT_SECONDS`

Agent:

- `HANK_REMOTE_AGENT_CLOUD_URL`
- `HANK_REMOTE_AGENT_ID`
- `HANK_REMOTE_AGENT_TOKEN`
- `HANK_REMOTE_AGENT_HOME_NAME`

Optional agent environment:

- `HANK_REMOTE_HA_BASE_URL`
- `HANK_REMOTE_HA_TOKEN`
- `HANK_REMOTE_HA_TIMEOUT_SECONDS`
- `HANK_REMOTE_SMB_HOST`
- `HANK_REMOTE_SMB_SHARE`
- `HANK_REMOTE_SMB_USERNAME`
- `HANK_REMOTE_SMB_PASSWORD`
- `HANK_REMOTE_SMB_DOMAIN`
- `HANK_REMOTE_AGENT_FILES_ROOT`
- `HANK_REMOTE_AGENT_NOTES_ROOT`

## 1. Prepare the server

```bash
mkdir -p /srv/hank-remote
cd /srv/hank-remote
git clone <your-hankserverside-repo-url> .
mkdir -p data/postgres data/files data/notes
```

If the repo already exists on the server, just `cd` into it and ensure those directories exist.

## 2. Create the env files

```bash
cp configs/cloud.compose.env.example .env.cloud
cp configs/agent.compose.env.example .env.agent
```

### `.env.cloud`

Use the default shape, but replace the PostgreSQL credentials with real values for the server:

```env
HANK_REMOTE_CLOUD_ADDR=:8080
HANK_REMOTE_CLOUD_DATABASE_URL=postgres://hankremote:replace-with-db-password@postgres:5432/hankremote?sslmode=disable
POSTGRES_DB=hankremote
POSTGRES_USER=hankremote
POSTGRES_PASSWORD=replace-with-db-password
HANK_REMOTE_SESSION_TTL_SECONDS=604800
HANK_REMOTE_REQUEST_TIMEOUT_SECONDS=30
```

### `.env.agent`

Keep the cloud URL exactly like this for the Compose deployment:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

Fill in the rest with your real values:

```env
HANK_REMOTE_AGENT_ID=home-main
HANK_REMOTE_AGENT_TOKEN=
HANK_REMOTE_AGENT_HOME_NAME=Home
HANK_REMOTE_HA_BASE_URL=http://<home-assistant-host>:8123
HANK_REMOTE_HA_TOKEN=<home-assistant-token>
```

Notes:

- leave `HANK_REMOTE_AGENT_TOKEN` blank for the first boot
- the raw token is issued by the dashboard after the first admin account is created
- if you are not using SMB, leave all `HANK_REMOTE_SMB_*` values empty
- if SMB is not configured, the agent uses the mounted `./data/files` folder
- note storage uses the mounted `./data/notes` folder unless you change the mounted root

## 3. Start the stack

```bash
cd /srv/hank-remote
docker compose up --build -d
docker compose ps
```

Check local health:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/metrics | head
```

Expected result:

- `/healthz` returns `200`
- `/readyz` returns `200`
- `/metrics` returns Prometheus text

At this point the `cloud` and `postgres` services should be healthy.
The `agent` may be running without a valid token yet, which is expected on first boot.

## 4. Point Cloudflare Tunnel at the cloud service

Configure the public hostname to forward to:

```text
http://127.0.0.1:8080
```

The tunnel must allow WebSocket upgrades for:

- `/ws/app`
- `/ws/agent`

If you manage Cloudflare Tunnel with a config file, the ingress shape is usually:

```yaml
ingress:
  - hostname: hank.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

## 5. Bootstrap the deployment

Open the public Hank Remote URL in a browser.

On a fresh deployment:

1. Register the first user account.
2. That first user becomes the deployment admin automatically.
3. The singleton deployment `Home` is created automatically.
4. Open the dashboard and issue an agent token from the Home agent section.

You no longer create a Home manually for a normal deployment.

If registration works but later calls report that the target home agent is offline, the bootstrap/auth path is already correct and the remaining issue is the agent connection, token, or `/ws/agent` path.

## 6. Install the issued agent token

Put the raw token from the dashboard into `.env.agent`:

```env
HANK_REMOTE_AGENT_TOKEN=<issued-token>
```

Then restart only the agent:

```bash
cd /srv/hank-remote
docker compose up -d --no-deps agent
```

## 7. Verify the agent connection

Check logs:

```bash
docker compose logs -f cloud
docker compose logs -f agent
```

Verify in the dashboard:

- the Home agent panel shows one agent
- the agent status becomes `online`
- Home sync status loads

Optional direct API check after you have an authenticated browser session:

- `GET /v1/home/agent` should show exactly one configured agent with status `online`

## 8. Live test checklist

Use this checklist for a real deployment validation pass.

### Auth and bootstrap

1. Open the public URL.
2. Register the first account.
3. Confirm the dashboard loads without any Home picker.
4. Confirm `GET /v1/home` returns one Home for the authenticated user.

### Agent

1. Issue one agent token.
2. Restart the agent with the issued token.
3. Confirm `GET /v1/home/agent` shows an online agent.

### Home Assistant

1. Save the Home Assistant service profile from the dashboard.
2. Confirm the profile status becomes healthy.
3. Confirm `GET /v1/home/sync` shows a `last_backup_at` value for the saved profile.

### Files

1. Browse files from the app or dashboard flow.
2. If SMB is configured, confirm the agent can see the target share.
3. If SMB is not configured, confirm file operations work against `./data/files`.
4. Test one upload and one download.

### Notes

1. Create or edit a shared Home note from the app.
2. Confirm it appears through the server-backed Home notes API.
3. Confirm collaboration still works through `/ws/app`.
4. Confirm `GET /v1/home/sync` shows notes sync status.

Important current behavior:

- app `notes.sync` uses the cloud note store
- note backup timestamps reflect cloud-stored shared note updates
- config backup timestamps update when a service profile apply succeeds
- the server does not currently create a brand-new note or config backup record on every app sync call

## 9. Routine operations

Restart everything:

```bash
docker compose restart
```

Restart only the agent:

```bash
docker compose restart agent
```

Rebuild after code changes:

```bash
cd /srv/hank-remote
git pull
docker compose up --build -d
```

## 10. Backups

Back up at least:

- `data/postgres`
- `.env.cloud`
- `.env.agent`

Also back up any real content stored under:

- `data/files`
- `data/notes`

The service stores cloud metadata in PostgreSQL.
Agent-side files and notes live in the mounted host directories and need host-level backup coverage.

## 11. Security notes

- keep `.env.cloud` and `.env.agent` readable only by the service user
- never share raw agent tokens, session tokens, Home Assistant tokens, or SMB credentials
- rotate agent tokens by issuing a new token, updating `.env.agent`, restarting the agent, then revoking the old token
- do not expose the cloud container directly on a public interface; keep the bind on `127.0.0.1:8080`
- do not mount Docker control sockets into the public cloud container

## 12. Troubleshooting pointers

- If `/healthz` or `/readyz` fail, inspect `docker compose logs -f cloud postgres`.
- If login works but the agent stays offline, inspect `docker compose logs -f agent` and recheck `HANK_REMOTE_AGENT_TOKEN` plus `HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent`.
- If Home Assistant actions fail, recheck `HANK_REMOTE_HA_BASE_URL` and `HANK_REMOTE_HA_TOKEN`.
- If file browsing fails, decide whether the deployment is supposed to use SMB or the mounted `./data/files` fallback, then verify only that path.
