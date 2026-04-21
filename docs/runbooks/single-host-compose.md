# Single-Host Docker Compose Runbook

Use this runbook for the supported real deployment shape:

- one machine
- one Docker Compose stack
- `cloud`, `agent`, and `postgres` together
- Cloudflare Tunnel exposing your configured host bind, for example `http://127.0.0.1:8080`

This deployment is singleton-only:

- one deployment Home
- first registered user becomes admin
- one agent surface for that deployment

## Preconditions

- Docker Engine and Docker Compose are installed
- the repo is present on the server
- the server can reach the local services the agent needs
- a Cloudflare Tunnel is ready to proxy to your configured host bind, for example `http://127.0.0.1:8080`

## Files Used

- `.env.cloud`
- `.env.agent`
- `docker-compose.yml`

## 1. Prepare the workspace

```bash
cd /srv
git clone <your-hankserverside-repo-url> hank-remote
cd /srv/hank-remote
mkdir -p data/postgres data/files data/notes
cp configs/cloud.compose.env.example .env.cloud
cp configs/agent.compose.env.example .env.agent
```

## 2. Review `.env.cloud`

Use the normal single-host shape:

```env
HANK_REMOTE_CLOUD_ADDR=:8080
HANK_REMOTE_CLOUD_HOST_BIND=127.0.0.1
HANK_REMOTE_CLOUD_HOST_PORT=8080
HANK_REMOTE_CLOUD_DATABASE_URL=postgres://hankremote:replace-with-db-password@postgres:5432/hankremote?sslmode=disable
POSTGRES_DB=hankremote
POSTGRES_USER=hankremote
POSTGRES_PASSWORD=replace-with-db-password
HANK_REMOTE_SESSION_TTL_SECONDS=604800
HANK_REMOTE_REQUEST_TIMEOUT_SECONDS=30
```

If host port `8080` is already in use, change only `HANK_REMOTE_CLOUD_HOST_PORT`, for example `18080`.
Replace `<host-port>` below with that `HANK_REMOTE_CLOUD_HOST_PORT` value.

## 3. Fill in `.env.agent`

Keep this value unchanged:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

Fill in the rest:

```env
HANK_REMOTE_AGENT_ID=home-main
HANK_REMOTE_AGENT_TOKEN=
HANK_REMOTE_AGENT_HOME_NAME=Home
HANK_REMOTE_HA_BASE_URL=http://<ha-host>:8123
HANK_REMOTE_HA_TOKEN=<ha-token>
```

Notes:

- leave `HANK_REMOTE_AGENT_TOKEN` blank before first bootstrap
- fill it after you issue a token from the dashboard
- leave SMB variables blank unless you are actually using SMB

## 4. Start the stack

```bash
cd /srv/hank-remote
docker compose up --build -d
docker compose ps
curl http://127.0.0.1:<host-port>/healthz
curl http://127.0.0.1:<host-port>/readyz
```

## 5. Configure Cloudflare Tunnel

Forward the public hostname to:

```text
http://127.0.0.1:<host-port>
```

Make sure WebSocket upgrades work for:

- `/ws/app`
- `/ws/agent`

## 6. Bootstrap the deployment

Open the public URL and:

1. register the first account
2. let that account become the deployment admin automatically
3. open the dashboard
4. issue an agent token from the Home agent section

Do not look for a separate Home creation step. The Home is created automatically for the first account on a fresh deployment.

## 7. Activate the agent token

Add the raw token to `.env.agent`:

```env
HANK_REMOTE_AGENT_TOKEN=<issued-token>
```

Restart only the agent:

```bash
cd /srv/hank-remote
docker compose up -d --no-deps agent
```

## 8. Verify the live deployment

Dashboard:

- login works
- no Home picker is present
- the Home agent card shows one online agent
- sync status loads

API:

```bash
curl http://127.0.0.1:<host-port>/healthz
curl http://127.0.0.1:<host-port>/readyz
curl http://127.0.0.1:<host-port>/metrics | head
```

App behavior:

- app login works against the public hostname
- files route through the server
- shared Home notes route through the server
- collaboration still works through `/ws/app`

## 9. Common operations

View logs:

```bash
docker compose logs -f cloud
docker compose logs -f agent
docker compose logs -f postgres
```

Restart:

```bash
docker compose restart
docker compose restart agent
```

Rebuild after pulling changes:

```bash
cd /srv/hank-remote
git pull
docker compose up --build -d
```

## 10. Important constraints

- the cloud stays on `127.0.0.1:<host-port>` on the host, while the container still listens on `:8080`
- the public hostname should come through Cloudflare Tunnel, not a direct public bind
- the agent should keep using `ws://cloud:8080/ws/agent`
- this version supports only one Home per deployment
- deployment changes happen by editing `.env.agent` or `.env.cloud` and restarting services
- the public cloud container should not control Docker on the host
