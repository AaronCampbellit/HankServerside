# Deployment

Hank Remote is deployed as one Docker Compose stack on one server.

Use the current setup and onboarding guide for the full operator flow:

- `docs/setup-and-onboarding.md`

## Current Shape

The stack runs:

- `cloud`: public dashboard, HTTPS/WebSocket app API, and agent relay
- `postgres`: live cloud-side persistence
- `db-ops`: database checksum, backup, restore-test, and restore worker
- `agent`: home connector for Home Assistant, files, SMB, and notes
- `postgres-restore`: restore-test database, started only for restore verification

This is a singleton deployment:

- one deployment Home
- first registered account becomes admin
- registration is disabled after first setup
- additional users are invited from the dashboard
- normal dashboard pages require Home membership

## Network

Bootstrap default host bind for Cloudflare Tunnel or a local reverse proxy:

```text
127.0.0.1:18080
```

Use `0.0.0.0:18080` only when the cloud HTTP port must be reachable directly on the server network.

Default container listener:

```text
:8080
```

Recommended public path:

```text
Hank app -> Cloudflare Tunnel or reverse proxy -> cloud -> agent
```

The proxy must support WebSocket upgrades for:

- `/ws/app`
- `/ws/agent`

Postgres and the restore database stay on private Docker networks and are not published to the host.

## Env Files

Private repo-root files:

- `.env.cloud`
- `.env.agent`

Keep both files private:

```bash
chmod 600 .env.cloud .env.agent
```

The old `configs/*.env.example` files are gone. Env examples now live in `docs/setup-and-onboarding.md`, and runtime env files live only in the repo root.

For deployment commands, use:

```bash
docker compose --env-file .env.cloud ...
```

That matters because the stack requires `.env.cloud`, and Compose host-port interpolation also needs it as the Compose env file.

For manual first boot without the bootstrap script, run migrations before starting `cloud` normally:

```bash
docker compose --env-file .env.cloud build postgres cloud db-ops
docker compose --env-file .env.cloud up -d postgres
docker compose --env-file .env.cloud run --rm cloud /usr/local/bin/hank-remote-cloud migrate up
docker compose --env-file .env.cloud run --rm cloud /usr/local/bin/hank-remote-cloud migrate status --strict
docker compose --env-file .env.cloud up -d cloud db-ops
```

## First Boot

```bash
cd /srv/hank-remote/HankServerside
scripts/bootstrap-first-run.sh
docker compose --env-file .env.cloud ps
scripts/doctor.sh
```

Expected first-boot services:

- `postgres`
- `db-ops`
- `cloud`

The `agent` starts later, after the dashboard generates `.env.agent`.

## Onboarding

1. Open the public Hank Remote URL.
2. Register the first account.
3. Let that account become the deployment admin.
4. Create the agent setup token from the dashboard.
5. Paste the generated setup block into `.env.agent`.
6. Start the agent:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud --profile agent up -d agent
```

## Backups

The stack includes encrypted pgBackRest backups and integrity checks.

Admin flow:

1. Open `/dashboard/storage`.
2. Confirm checksums are enabled.
3. Run a manual backup.
4. Run a restore test after the first backup exists.

Back up:

- `hank_pgbackrest_repo`
- `hank_db_ops_state`
- `hank_agent_files`
- `hank_agent_notes`
- `.env.cloud`
- `.env.agent`

Keep `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`; backups cannot be restored without that passphrase.

## Verification

```bash
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/readyz
curl -H "Authorization: Bearer $HANK_REMOTE_ADMIN_SESSION_TOKEN" http://127.0.0.1:18080/metrics | head
docker compose --env-file .env.cloud --profile agent ps
scripts/doctor.sh
```

Use the configured `HANK_REMOTE_CLOUD_HOST_PORT` if it is not `18080`.

## Normal Updates

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --env-file .env.cloud --profile agent up --build -d
scripts/doctor.sh
```
