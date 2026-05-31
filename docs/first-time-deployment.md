# First-Time Deployment

Use this for a fresh Hank Remote install on one server.

The full current setup and onboarding flow is here:

- `docs/setup-and-onboarding.md`

## Fresh Install Summary

1. Clone the repo into:

```bash
/srv/hank-remote/HankServerside
```

2. Bootstrap the cloud stack:

```bash
cd /srv/hank-remote/HankServerside
scripts/bootstrap-first-run.sh
```

For Cloudflare Tunnel or a local reverse proxy, the default bind is `127.0.0.1:18080`. For unattended setup:

```bash
HANK_REMOTE_BOOTSTRAP_NONINTERACTIVE=true \
HANK_REMOTE_BOOTSTRAP_HOST_BIND=127.0.0.1 \
HANK_REMOTE_BOOTSTRAP_HOST_PORT=18080 \
scripts/bootstrap-first-run.sh
```

The script creates `.env.cloud`, builds `postgres`, `cloud`, and `db-ops`, runs migrations, starts the first-boot services, and checks health/readiness.

3. Check the stack:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud ps
scripts/doctor.sh
```

Expected services:

- `postgres`
- `db-ops`
- `cloud`

The `agent` is not expected to run yet.

4. Point Cloudflare Tunnel or the reverse proxy at:

```text
http://127.0.0.1:18080
```

Use the configured `HANK_REMOTE_CLOUD_HOST_PORT` if it is not `18080`.

5. Open the public URL and register the first admin.

The first registration creates the singleton Home and admin membership. Public registration is disabled after that first setup.

6. In the dashboard, create the agent setup token and paste the generated block into:

```bash
cd /srv/hank-remote/HankServerside
nano .env.agent
chmod 600 .env.agent
```

7. Start the agent:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud --profile agent up -d agent
```

8. Open `/dashboard/storage`, run the first manual backup, and then run a restore test after the backup exists.

9. Run a final doctor check:

```bash
cd /srv/hank-remote/HankServerside
scripts/doctor.sh
```

## Env File Locations

- cloud secrets: `/srv/hank-remote/HankServerside/.env.cloud`
- agent token and local connector settings: `/srv/hank-remote/HankServerside/.env.agent`

`.env.agent` can contain SMB passwords and Home Assistant tokens. Keep both env files mode `0600`.

Do not create host `data/` folders for Postgres, files, or notes. Docker named volumes are used by default.
Include `hank_pgbackrest_repo`, `hank_note_attachments`, `hank_db_ops_state`, `hank_agent_files`, `hank_agent_notes`, `.env.cloud`, and `.env.agent` in server backups.

## Normal Updates

After the agent is active:

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --env-file .env.cloud --profile agent up --build -d
scripts/doctor.sh
```
