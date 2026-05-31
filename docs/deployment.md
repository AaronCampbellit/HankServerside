# Deployment

Hank Remote deploys as one Docker Compose stack on one server.

Use these docs by purpose:

- `docs/first-time-deployment.md`: short fresh-server checklist
- `docs/setup-and-onboarding.md`: canonical setup, env, onboarding, backup, and update guide
- `docs/runbooks/`: operational failure and maintenance runbooks

## Current Shape

The stack runs:

- `cloud`: public dashboard, app API, WebSocket relay, and deployment docs page
- `postgres`: live cloud-side persistence
- `db-ops`: database checksum, backup, restore-test, and restore worker
- `agent`: home connector for Home Assistant, files, SMB, media, and notes
- `postgres-restore`: restore-test database, started only for restore verification

This is a singleton deployment: one deployment Home, first registered account becomes admin, and later users are invited from the dashboard.

## Network

The bootstrap default bind is:

```text
127.0.0.1:18080
```

Point Cloudflare Tunnel or a same-host reverse proxy at that address. Use `0.0.0.0:18080` only when the cloud HTTP port must be reachable directly on the server network.

The proxy must support WebSocket upgrades for:

- `/ws/app`
- `/ws/agent`

Postgres and the restore database stay on private Docker networks and are not published to the host.

## Fresh Install

```bash
sudo mkdir -p /srv/hank-remote
sudo chown "$USER":"$USER" /srv/hank-remote
cd /srv/hank-remote
git clone <your-hankserverside-repo-url> HankServerside
cd /srv/hank-remote/HankServerside
scripts/bootstrap-first-run.sh
scripts/doctor.sh
```

Then open the public URL, register the first admin, create the agent setup file from the dashboard, and install it:

```bash
pbpaste | ssh <server-user>@<server-host> 'cd /srv/hank-remote/HankServerside && scripts/install-agent-env.sh'
ssh <server-user>@<server-host> 'cd /srv/hank-remote/HankServerside && scripts/doctor.sh'
```

If `pbpaste` is not available on the server session, paste the dashboard-generated block into `.env.agent`, run `chmod 600 .env.agent`, then run:

```bash
docker compose --env-file .env.cloud --profile agent up -d agent
```

## Backups

Back up:

- `hank_pgbackrest_repo`
- `hank_note_attachments`
- `hank_db_ops_state`
- `hank_agent_files`
- `hank_agent_notes`
- `.env.cloud`
- `.env.agent`

Keep `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`; encrypted database backups cannot be restored without that passphrase.

## Updates

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --env-file .env.cloud --profile agent up --build -d
scripts/doctor.sh
```
