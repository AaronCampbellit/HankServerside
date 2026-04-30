# Deployment

Hank Remote is deployed as one Docker Compose stack on one machine.

That machine runs:

- `cloud`: the public HTTPS/WebSocket entrypoint
- `db-ops`: database checksum, backup, and restore worker
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

The cloud publishes on `0.0.0.0:18080` by default so the server IP can reach it directly.
The host bind and port are configurable in `.env.cloud`.
External traffic should still come through a firewall, reverse proxy, or Cloudflare Tunnel rather than exposing the service broadly without controls.

## Prerequisites

- a Linux server with Docker Engine and Docker Compose installed
- the server can already reach the local systems the agent needs
  - Home Assistant
  - SMB share, if used
  - local file and note storage
- a Cloudflare Tunnel or reverse proxy that can proxy HTTP and WebSocket traffic to your configured host bind, for example `http://127.0.0.1:18080` or `http://<server-ip>:18080`
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

- `configs/cloud.compose.env.example`
- `configs/agent.compose.env.example`
- optional `/.env.cloud`
- optional `/.env.agent`
- `docker-compose.yml`

## Environment

Cloud:

- `HANK_REMOTE_CLOUD_ADDR`
- `HANK_REMOTE_CLOUD_HOST_BIND`
- `HANK_REMOTE_CLOUD_HOST_PORT`
- `HANK_REMOTE_CLOUD_DATABASE_URL`
- `POSTGRES_DB`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `HANK_REMOTE_SESSION_TTL_SECONDS`
- `HANK_REMOTE_REQUEST_TIMEOUT_SECONDS`

Database operations:

- `HANK_REMOTE_DB_OPS_INTENT_SECRET`
- `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`
- `HANK_REMOTE_DB_OPS_STANZA`
- `HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL`
- `HANK_REMOTE_DB_OPS_COMPOSE_FILE`

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
git clone <your-hankserverside-repo-url> HankServerside
cd /srv/hank-remote/HankServerside
```

If the repo already exists on the server, just `cd /srv/hank-remote/HankServerside`. Docker creates the default persistent volumes for PostgreSQL, local files, and notes automatically.

## 2. Review the default env files

The Compose stack already loads checked-in defaults from:

- `configs/cloud.compose.env.example`
- `configs/agent.compose.env.example`

Create `/.env.cloud` at least for the real db-ops secret and backup encryption passphrase.
Create `/.env.agent` after the dashboard issues an agent token.

### Default cloud env

Use the default shape, but replace the PostgreSQL credentials with real values for the server:

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
HANK_REMOTE_DB_OPS_INTENT_SECRET=replace-with-a-long-random-db-ops-secret
HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS=replace-with-a-long-random-backup-encryption-passphrase
```

If host port `18080` is already in use, change only `HANK_REMOTE_CLOUD_HOST_PORT`, for example:

```env
HANK_REMOTE_CLOUD_HOST_PORT=18081
```

Replace `<host-port>` below with that `HANK_REMOTE_CLOUD_HOST_PORT` value.

Do not change `HANK_REMOTE_AGENT_CLOUD_URL` for the single-host Compose deployment. The agent still connects to `ws://cloud:8080/ws/agent` on the internal Docker network.

If you need to override those defaults on one server, create `/.env.cloud` with only the keys you want to replace, for example:

```env
HANK_REMOTE_CLOUD_HOST_PORT=18080
POSTGRES_PASSWORD=replace-with-real-db-password
HANK_REMOTE_CLOUD_DATABASE_URL=postgres://hankremote:replace-with-real-db-password@postgres:5432/hankremote?sslmode=disable
HANK_REMOTE_DB_OPS_INTENT_SECRET=replace-with-real-db-ops-secret
HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS=replace-with-real-backup-encryption-passphrase
```

### Agent env

Keep the cloud URL exactly like this for the Compose deployment:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

The agent service is behind the `agent` Compose profile, so first boot does not start it without a token.
After the dashboard issues a token, it generates the full `.env.agent` file for you.
The generated file has this shape:

```env
HANK_REMOTE_AGENT_ID=home-main
HANK_REMOTE_AGENT_TOKEN=<issued-token>
HANK_REMOTE_AGENT_HOME_NAME=Home
HANK_REMOTE_HA_BASE_URL=http://<home-assistant-host>:8123
HANK_REMOTE_HA_TOKEN=<home-assistant-token>
```

Notes:

- the checked-in default leaves the token empty and the agent profile disabled for first boot
- the raw token is issued by the dashboard after the first admin account is created
- the dashboard-generated `.env.agent` block includes the token, agent ID, Home name, optional Home Assistant fields, optional SMB fields, and mounted file/note roots
- if you are not using SMB, leave all `HANK_REMOTE_SMB_*` values empty
- if SMB is not configured, the agent uses the Docker-managed `hank_agent_files` volume
- note storage uses the Docker-managed `hank_agent_notes` volume unless you change the mounted root

## 3. Start the stack

```bash
cd /srv/hank-remote/HankServerside
docker compose up --build -d
docker compose ps
```

This starts `postgres`, `db-ops`, and `cloud`.
It intentionally does not start `agent` until you create `.env.agent` from the issued token.

Check local health:

```bash
curl http://127.0.0.1:<host-port>/healthz
curl http://127.0.0.1:<host-port>/readyz
curl http://127.0.0.1:<host-port>/metrics | head
```

Expected result:

- `/healthz` returns `200`
- `/readyz` returns `200`
- `/metrics` returns Prometheus text

At this point the `cloud`, `db-ops`, and `postgres` services should be healthy.
The `agent` is not expected to be running yet.

## 4. Point Cloudflare Tunnel at the cloud service

Configure the public hostname to forward to:

```text
http://127.0.0.1:<host-port>
```

The tunnel must allow WebSocket upgrades for:

- `/ws/app`
- `/ws/agent`

If you manage Cloudflare Tunnel with a config file, the ingress shape is usually:

```yaml
ingress:
  - hostname: hank.example.com
    service: http://127.0.0.1:<host-port>
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

## 6. Install the generated agent env file

After issuing the token, the dashboard shows a generated `.env.agent` block.
Copy that whole block into `/.env.agent`:

```bash
cd /srv/hank-remote/HankServerside
nano .env.agent
```

Edit the Home Assistant and SMB values in that file if you need them.
Leave the SMB values blank to use the Docker-managed files volume.

Then start the agent profile:

```bash
cd /srv/hank-remote/HankServerside
docker compose --profile agent up -d agent
```

## 7. Verify the agent connection

Check logs:

```bash
docker compose logs -f cloud
docker compose --profile agent logs -f agent
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
2. Copy the generated `.env.agent` block.
3. Start the agent profile with `docker compose --profile agent up -d agent`.
4. Confirm `GET /v1/home/agent` shows an online agent.

### Home Assistant

1. Save the Home Assistant service profile from the dashboard.
2. Confirm the profile status becomes healthy.
3. Confirm `GET /v1/home/sync` shows a `last_backup_at` value for the saved profile.

### Files

1. Browse files from the app or dashboard flow.
2. If SMB is configured, confirm the agent can see the target share.
3. If SMB is not configured, confirm file operations work against the Docker-managed files volume.
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

Rebuild after code changes:

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --profile agent up --build -d
```

## 10. Database Integrity And Backups

The Compose stack now includes database integrity checks and pgBackRest backups:

- `postgres` uses the custom `hank-postgres` image with pgBackRest and PostgreSQL checksum tools.
- new clusters are created with `POSTGRES_INITDB_ARGS=--data-checksums`
- `db-ops` runs scheduled checksum status checks, `pg_amcheck`, pgBackRest backups, restore tests, and primary restore intents
- `cloud` reads the shared db-ops state/log volumes and shows them at `/dashboard/storage`
- pgBackRest uses encrypted repositories with `repo1-cipher-type=aes-256-cbc` and `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`
- `postgres-restore` starts only under the `restore` profile during restore validation

Back up at least:

- Docker volume `hank_pgbackrest_repo`
- Docker volume `hank_db_ops_state`
- optional `/.env.cloud`
- optional `/.env.agent`

Also back up any real content stored under:

- Docker volume `hank_agent_files`
- Docker volume `hank_agent_notes`

The old raw PostgreSQL volume `hank_postgres_data` is still important, but pgBackRest is the restore source once backups are running.
Agent-side files and notes live in Docker volumes by default and need volume backup coverage.

Database network access is intentionally narrow:

- `postgres`, `cloud`, and `db-ops` share an internal database network.
- `postgres-restore` and `db-ops` share a separate internal restore network.
- only `cloud` publishes a host port.
- `postgres-restore` mounts the backup repository read-only.

If this deployment already has unencrypted pgBackRest backups, create a fresh encrypted full backup with the new cipher pass and keep the old unencrypted repository offline only until you no longer need it for rollback.

### Check whether checksums are already enabled

```bash
cd /srv/hank-remote/HankServerside
docker compose exec postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select current_setting('data_checksums')"
```

If it prints `on`, no checksum migration is needed.

### Enable checksums on an existing cluster

PostgreSQL requires the cluster to be stopped before `pg_checksums --enable` can run.
Use the helper during a maintenance window:

```bash
cd /srv/hank-remote/HankServerside
scripts/enable-pg-checksums.sh
```

After the stack restarts, open `/dashboard/storage` and confirm checksum health shows enabled.

### pgBackRest backups

Default schedule:

- full backup: Sunday at 02:00
- differential backup: Monday through Saturday at 02:00
- checksum status: every 15 minutes
- `pg_amcheck`: Sunday at 03:30
- restore verification: Sunday at 04:00
- retained full backup chains: 2

Run a backup immediately from `/dashboard/storage`.
If you run pgBackRest manually, include the same encryption settings:

```bash
docker compose exec db-ops sh -lc 'pgbackrest --stanza=hank --repo1-cipher-type=aes-256-cbc --repo1-cipher-pass="$HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS" --type=diff backup'
```

The backup repository is stored in `hank_pgbackrest_repo`.
The current target type is `posix`; the dashboard/API config keeps the target typed so an external target can be added later without changing the Hank app contract.

### Restore testing and primary restore

Use `/dashboard/storage` to request a restore test into the internal restore data directory.
Primary restore is intentionally available from the dashboard but requires selecting a backup and typing the configured confirmation phrase.
Only signed-in admins can open the storage dashboard or call storage routes.
Storage alerts and websocket events are redacted; they do not include database passwords, tokens, backup encryption values, or raw command output.

Restore tests validate:

- expected Hank database tables exist
- non-system login role attributes match the main database

During a primary restore, `db-ops`:

1. stops `cloud` and `postgres`
2. writes a restore safety marker in db-ops state
3. runs pgBackRest restore into the primary PostgreSQL data volume
4. restarts `postgres`
5. restarts `cloud`

The public cloud container does not receive Docker host control. Docker socket access is mounted only into the private `db-ops` service.

## 11. Security notes

- keep any local `/.env.cloud` and `/.env.agent` override files readable only by the service user
- never share raw agent tokens, session tokens, Home Assistant tokens, or SMB credentials
- rotate agent tokens by issuing a new token, copying the generated `.env.agent` block, refreshing the agent profile, then revoking the old token
- do not expose the cloud container broadly without a firewall, reverse proxy, or Cloudflare Tunnel; set `HANK_REMOTE_CLOUD_HOST_BIND=127.0.0.1` if this server should only accept local proxy traffic
- do not mount Docker control sockets into the public cloud container

## 12. Troubleshooting pointers

- If Docker reports `port is already allocated`, run `sudo ss -ltnp | grep ':<host-port>'` and `docker ps --format 'table {{.Names}}\t{{.Ports}}' | grep <host-port>`, then either stop the old service or set a different `HANK_REMOTE_CLOUD_HOST_PORT` in `.env.cloud`.
- If `/healthz` or `/readyz` fail, inspect `docker compose logs -f cloud postgres` and confirm the chosen `HANK_REMOTE_CLOUD_HOST_PORT` is actually free on the host.
- If login works but the agent stays offline, inspect `docker compose --profile agent logs -f agent` and recheck `HANK_REMOTE_AGENT_TOKEN` plus `HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent`.
- If Home Assistant actions fail, recheck `HANK_REMOTE_HA_BASE_URL` and `HANK_REMOTE_HA_TOKEN`.
- If file browsing fails, decide whether the deployment is supposed to use SMB or the Docker-managed files volume fallback, then verify only that path.
