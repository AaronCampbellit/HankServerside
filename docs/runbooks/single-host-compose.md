# Single-Host Docker Compose Runbook

Use this runbook for the supported live deployment shape:

- one server
- one Docker Compose stack
- one singleton Home
- first registered user becomes admin
- Cloudflare Tunnel or a reverse proxy in front of `cloud`

The complete current setup flow is:

- `docs/deployment.md`

## Files

Private server files in the repo root:

- `.env.cloud`
- `.env.agent`

Protect them after every edit:

```bash
chmod 600 .env.cloud .env.agent
```

The agent service can update `.env.agent` from inside the container so dashboard connection changes persist. If you override `HANK_REMOTE_AGENT_CONTAINER_USER`, make sure that custom user can write `.env.agent` and the agent volumes.

Compose file:

- `docker-compose.yml`

Use `docker compose --env-file .env.cloud ...` for deployment commands so Compose sees the host bind and port values from `.env.cloud`.

## First Boot

```bash
cd /srv/hank-remote/HankServerside
scripts/bootstrap-first-run.sh
docker compose --env-file .env.cloud ps
scripts/doctor.sh
```

Expected services:

- `postgres`
- `db-ops`
- `cloud`

The `agent` is profile-gated and should stay stopped until `.env.agent` exists.

Manual first boot without the bootstrap script must run migrations before starting `cloud` normally:

```bash
docker compose --env-file .env.cloud build postgres cloud db-ops
docker compose --env-file .env.cloud up -d postgres
docker compose --env-file .env.cloud run --rm cloud /usr/local/bin/hank-remote-cloud migrate up
docker compose --env-file .env.cloud run --rm cloud /usr/local/bin/hank-remote-cloud migrate status --strict
docker compose --env-file .env.cloud up -d cloud db-ops
```

## Bootstrap

1. Open the public Hank Remote URL.
2. Register the first account.
3. Let the first account become the admin automatically.
4. Create the agent setup token in the dashboard.
5. Copy the generated `.env.agent` block.
6. Install it from your Mac clipboard:

```bash
pbpaste | ssh <server-user>@<server-host> 'cd /srv/hank-remote/HankServerside && scripts/install-agent-env.sh'
```

If you are already in an SSH session, paste the block into `.env.agent`, run `chmod 600 .env.agent`, then run `docker compose --env-file .env.cloud --profile agent up -d agent`.

Public registration is disabled after the first Home exists. Add more users through dashboard invitations.

## Verify

```bash
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/readyz
curl -H "Authorization: Bearer $HANK_REMOTE_ADMIN_SESSION_TOKEN" http://127.0.0.1:18080/metrics | head
docker compose --env-file .env.cloud --profile agent ps
scripts/doctor.sh
```

Use the configured `HANK_REMOTE_CLOUD_HOST_PORT` if it is not `18080`.

## Monitoring And Alerts

The compose file ships a `monitoring` profile with Prometheus (rule evaluation
against `ops/prometheus/alerts.yml`) and Alertmanager. Both bind to
`127.0.0.1` only; access them over SSH port forwarding, never expose them
publicly.

1. Set a dedicated scrape token in `.env.cloud`:

```bash
echo "HANK_REMOTE_METRICS_SCRAPE_TOKEN=$(openssl rand -hex 32)" >> .env.cloud
```

2. Write the same token where Prometheus reads it (gitignored):

```bash
mkdir -p ops/prometheus/secrets
grep '^HANK_REMOTE_METRICS_SCRAPE_TOKEN=' .env.cloud | cut -d= -f2 | tr -d '\n' > ops/prometheus/secrets/metrics-token
```

The Prometheus container runs as `nobody` (uid 65534), so the token file must
be readable by that user while staying hidden from other host users:

```bash
docker run --rm -v "$(pwd)/ops/prometheus/secrets:/s" alpine sh -c 'chown -R 65534:65534 /s && chmod 400 /s/metrics-token'
```

3. Restart the cloud (to pick up the token) and start monitoring:

```bash
docker compose --env-file .env.cloud up -d cloud
docker compose --env-file .env.cloud --profile monitoring up -d prometheus alertmanager
```

4. Verify: `http://127.0.0.1:9090/targets` shows the `hank-cloud` target up,
   and `http://127.0.0.1:9090/rules` lists the Hank alert rules.

Alertmanager starts with a no-op receiver. Configure a real receiver (email or
webhook) in a server-local copy of `ops/alertmanager/alertmanager.yml` before
relying on alerts in production, and test-fire one alert to confirm delivery.

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
scripts/doctor.sh
```

## Storage Notes

Postgres is private to Docker networks. It is not published to the host.

Back up:

- `hank_pgbackrest_repo`
- `hank_note_attachments`
- `hank_db_ops_state`
- `hank_agent_files`
- `hank_agent_notes`
- `.env.cloud`
- `.env.agent`

Keep `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`; encrypted pgBackRest backups cannot be restored without it.
Keep `hank_note_attachments` with the pgBackRest repository because note attachment files live outside Postgres.

If an existing database has checksums disabled, schedule downtime and run:

```bash
cd /srv/hank-remote/HankServerside
scripts/enable-pg-checksums.sh
```
