# Single-Host Docker Compose Runbook

Use this runbook for the supported real deployment shape:

- one machine
- one Docker Compose stack
- `cloud`, `db-ops`, `agent`, and `postgres` together
- Cloudflare Tunnel or a reverse proxy exposing your configured host bind, for example `http://127.0.0.1:18080` or `http://<server-ip>:18080`

This deployment is singleton-only:

- one deployment Home
- first registered user becomes admin
- one agent surface for that deployment

## Preconditions

- Docker Engine and Docker Compose are installed
- the repo is present on the server
- the server can reach the local services the agent needs
- a Cloudflare Tunnel or reverse proxy is ready to proxy to your configured host bind, for example `http://127.0.0.1:18080` or `http://<server-ip>:18080`

## Files Used

- `configs/cloud.compose.env.example`
- `configs/agent.compose.env.example`
- optional `/.env.cloud`
- optional `/.env.agent`
- `docker-compose.yml`

## 1. Prepare the workspace

```bash
cd /srv
mkdir -p hank-remote
cd /srv/hank-remote
git clone <your-hankserverside-repo-url> HankServerside
cd /srv/hank-remote/HankServerside
```

Docker creates the default persistent volumes for PostgreSQL, local files, and notes automatically.

## 2. Review the default cloud env

Use the normal single-host shape:

```env
HANK_REMOTE_CLOUD_ADDR=:8080
HANK_REMOTE_CLOUD_HOST_BIND=0.0.0.0
HANK_REMOTE_CLOUD_HOST_PORT=18080
HANK_REMOTE_CLOUD_DATABASE_URL=postgres://hankremote:replace-with-db-password@postgres:5432/hankremote?sslmode=disable
POSTGRES_DB=hankremote
POSTGRES_USER=hankremote
POSTGRES_PASSWORD=replace-with-db-password
HANK_REMOTE_SESSION_TTL_SECONDS=604800
HANK_REMOTE_REQUEST_TIMEOUT_SECONDS=30
HANK_REMOTE_DB_OPS_INTENT_SECRET=replace-with-real-db-ops-secret
HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS=replace-with-real-backup-encryption-passphrase
```

If host port `18080` is already in use, change only `HANK_REMOTE_CLOUD_HOST_PORT`, for example `18081`.
Replace `<host-port>` below with that `HANK_REMOTE_CLOUD_HOST_PORT` value.

Create `/.env.cloud` at least for the real db-ops secret and backup encryption passphrase.
If you need other server-specific overrides, keep them in the same file.

If Docker reports `port is already allocated`, confirm what owns the port:

```bash
sudo ss -ltnp | grep ':<host-port>'
docker ps --format 'table {{.Names}}\t{{.Ports}}' | grep <host-port>
```

Then either stop the old service using that port or keep it running and choose another `HANK_REMOTE_CLOUD_HOST_PORT`.

## 3. Review the agent env shape

Keep this value unchanged:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

The agent service is behind the `agent` Compose profile, so first boot does not start it without a token.
After the dashboard issues a token, it generates a complete `.env.agent` file with this shape:

```env
HANK_REMOTE_AGENT_ID=home-main
HANK_REMOTE_AGENT_TOKEN=<issued-token>
HANK_REMOTE_AGENT_HOME_NAME=Home
HANK_REMOTE_HA_BASE_URL=http://<ha-host>:8123
HANK_REMOTE_HA_TOKEN=<ha-token>
```

Notes:

- the checked-in default keeps the token empty before first bootstrap
- create `/.env.agent` from the generated dashboard block after token issuance
- leave SMB variables blank unless you are actually using SMB

## 4. Start the stack

```bash
cd /srv/hank-remote/HankServerside
docker compose up --build -d
docker compose ps
curl http://127.0.0.1:<host-port>/healthz
curl http://127.0.0.1:<host-port>/readyz
```

This starts `postgres`, `db-ops`, and `cloud`.
It intentionally does not start `agent` until a token exists.

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

## 7. Activate the agent

Copy the generated `.env.agent` block from the dashboard into `/.env.agent`:

```bash
cd /srv/hank-remote/HankServerside
nano .env.agent
```

Edit Home Assistant or SMB values if you need them.
Then start the agent profile:

```bash
cd /srv/hank-remote/HankServerside
docker compose --profile agent up -d agent
```

## 8. Verify the live deployment

Dashboard:

- login works
- no Home picker is present
- the Home agent card shows one online agent
- sync status loads
- storage status loads and shows checksum/backup state

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
docker compose --profile agent logs -f agent
docker compose logs -f postgres
```

Restart first-boot services:

```bash
docker compose restart
```

Restart everything after the agent is active:

```bash
docker compose --profile agent restart
```

Restart only the agent:

```bash
docker compose --profile agent restart agent
```

Rebuild after pulling changes:

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --profile agent up --build -d
```

## 10. Important constraints

- the cloud publishes on `0.0.0.0:<host-port>` on the host by default, while the container still listens on `:8080`
- set `HANK_REMOTE_CLOUD_HOST_BIND=127.0.0.1` if only a local proxy or tunnel should reach the service
- the public hostname should come through Cloudflare Tunnel, not a direct public bind
- Postgres traffic stays on internal Docker networks; `postgres` is not port-published
- `postgres-restore` is profile-gated and is reachable only by `db-ops` during restore validation
- pgBackRest backups are encrypted with `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`
- replace old unencrypted pgBackRest repositories with a fresh encrypted full backup before relying on restore
- the agent should keep using `ws://cloud:8080/ws/agent`
- the agent is profile-gated and starts only when you use `docker compose --profile agent ...`
- this version supports only one Home per deployment
- deployment changes happen by editing optional `/.env.agent` or `/.env.cloud` override files and refreshing services
- the public cloud container should not control Docker on the host
