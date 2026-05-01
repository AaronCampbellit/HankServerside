# Single-Host Docker Compose Runbook

Use this runbook for the supported live deployment shape:

- one server
- one Docker Compose stack
- one singleton Home
- first registered user becomes admin
- Cloudflare Tunnel or a reverse proxy in front of `cloud`

The complete current setup flow is:

- `docs/setup-and-onboarding.md`

## Files

Private server files in the repo root:

- `.env.cloud`
- `.env.agent`

Compose file:

- `docker-compose.yml`

Use `docker compose --env-file .env.cloud ...` for deployment commands so Compose sees the host bind and port values from `.env.cloud`.

## First Boot

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud up --build -d
docker compose --env-file .env.cloud ps
```

Expected services:

- `postgres`
- `db-ops`
- `cloud`

The `agent` is profile-gated and should stay stopped until `.env.agent` exists.

## Bootstrap

1. Open the public Hank Remote URL.
2. Register the first account.
3. Let the first account become the admin automatically.
4. Create the agent setup token in the dashboard.
5. Paste the generated setup block into `.env.agent`.
6. Start the agent:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud --profile agent up -d agent
```

Public registration is disabled after the first Home exists. Add more users through dashboard invitations.

## Verify

```bash
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/readyz
curl http://127.0.0.1:18080/metrics | head
docker compose --env-file .env.cloud --profile agent ps
```

Use the configured `HANK_REMOTE_CLOUD_HOST_PORT` if it is not `18080`.

Dashboard checks:

- Home agent shows online
- sync status loads
- storage status loads for admins
- first manual backup succeeds
- restore test succeeds after the first backup exists

## Common Operations

View logs:

```bash
docker compose --env-file .env.cloud logs -f cloud postgres db-ops
docker compose --env-file .env.cloud --profile agent logs -f agent
```

Restart everything after the agent is active:

```bash
docker compose --env-file .env.cloud --profile agent restart
```

Rebuild after pulling changes:

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --env-file .env.cloud --profile agent up --build -d
```

## Storage Notes

Postgres is private to Docker networks. It is not published to the host.

Back up:

- `hank_pgbackrest_repo`
- `hank_db_ops_state`
- `hank_agent_files`
- `hank_agent_notes`
- `.env.cloud`
- `.env.agent`

Keep `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`; encrypted pgBackRest backups cannot be restored without it.

If an existing database has checksums disabled, schedule downtime and run:

```bash
cd /srv/hank-remote/HankServerside
scripts/enable-pg-checksums.sh
```
