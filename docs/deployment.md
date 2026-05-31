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

Default public-facing host bind:

```text
0.0.0.0:18080
```

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

## First Boot

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud up --build -d
docker compose --env-file .env.cloud ps
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
```

Use the configured `HANK_REMOTE_CLOUD_HOST_PORT` if it is not `18080`.

## Normal Updates

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --env-file .env.cloud --profile agent up --build -d
```
