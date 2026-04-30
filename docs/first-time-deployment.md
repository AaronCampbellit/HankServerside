# First-Time Deployment

Use this for a fresh HankServerside install on one server.

The first boot starts only:

- `postgres`
- `db-ops`
- `cloud`

The `agent` starts later, after the dashboard issues an agent token and generates `.env.agent`.

## 1. Install Docker

On a fresh Ubuntu server:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl git
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"
newgrp docker
docker --version
docker compose version
```

## 2. Clone HankServerside

```bash
sudo mkdir -p /srv/hank-remote
sudo chown "$USER":"$USER" /srv/hank-remote
cd /srv/hank-remote
git clone <your-hankserverside-repo-url> HankServerside
cd /srv/hank-remote/HankServerside
```

You do not need to create `data/postgres`, `data/files`, or `data/notes`.
Docker creates persistent volumes automatically.

## 3. Check the Defaults

The default cloud port is:

```text
0.0.0.0:18080
```

The cloud container still listens internally on `:8080`.
The agent always uses this internal URL:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

Create `.env.cloud` to replace the backup and db-ops secrets. If port `18080` is already used, include a different host port too:

```env
HANK_REMOTE_DB_OPS_INTENT_SECRET=replace-with-real-db-ops-secret
HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS=replace-with-real-backup-encryption-passphrase
# Optional if 18080 is already used:
HANK_REMOTE_CLOUD_HOST_PORT=18081
```

If Docker reports `port is already allocated`, confirm what owns the port:

```bash
sudo ss -ltnp | grep ':18080'
docker ps --format 'table {{.Names}}\t{{.Ports}}' | grep 18080
```

Then either stop the old service using that port or keep it running and use a different `HANK_REMOTE_CLOUD_HOST_PORT`.

## 4. Start First Boot

```bash
cd /srv/hank-remote/HankServerside
docker compose up --build -d
docker compose ps
```

Expected first-boot services:

- `postgres`
- `db-ops`
- `cloud`

The `agent` should not be running yet.

Check the cloud:

```bash
curl http://127.0.0.1:<host-port>/healthz
curl http://127.0.0.1:<host-port>/readyz
```

Use your custom port if you changed `HANK_REMOTE_CLOUD_HOST_PORT`.

## 5. Put a Public URL in Front

Point Cloudflare Tunnel or your reverse proxy at:

```text
http://127.0.0.1:18080
```

or, if your proxy runs from another machine:

```text
http://<server-ip>:18080
```

Make sure WebSockets work for:

- `/ws/app`
- `/ws/agent`

## 6. Create the First Admin

Open the public Hank Remote URL.

On a fresh deployment:

1. Register the first account.
2. The first account becomes the deployment admin.
3. The singleton Home is created automatically.
4. Open the dashboard.
5. Issue an agent token from the Agent Tokens panel.

## 7. Create `.env.agent`

After the token is issued, the dashboard shows a generated `.env.agent` file.
Use the copy button and paste the full block into:

```bash
cd /srv/hank-remote/HankServerside
nano .env.agent
```

Edit these only if you need them:

- `HANK_REMOTE_HA_BASE_URL`
- `HANK_REMOTE_HA_TOKEN`
- `HANK_REMOTE_SMB_*`

Leave SMB blank to use the Docker-managed files volume.

## 8. Start the Agent

```bash
cd /srv/hank-remote/HankServerside
docker compose --profile agent up -d agent
docker compose --profile agent ps
```

Then check logs:

```bash
docker compose --profile agent logs -f agent
```

In the dashboard, the agent should show as `online`.

## 9. Normal Updates

After the agent has been activated, use the agent profile during updates:

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --profile agent up --build -d
```

## 10. Backups

Back up these Compose volumes. Docker may prefix their actual names with the Compose project name:

- `hank_pgbackrest_repo`
- `hank_db_ops_state`
- `hank_agent_files`
- `hank_agent_notes`

`hank_postgres_data` still holds the live database, but pgBackRest backups in `hank_pgbackrest_repo` are the restore source once the storage worker is running.
The pgBackRest repository is encrypted with `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`; keep that passphrase with your server secrets because backups cannot be restored without it.
If you already created unencrypted pgBackRest backups before this hardening, run a fresh encrypted full backup and retire the old unencrypted repository after your rollback window ends.

The database containers are private to Docker networks:

- only `cloud` publishes a host port
- `postgres` is reachable by `cloud` and `db-ops`
- `postgres-restore` is reachable only by `db-ops` during restore validation

List the exact names on the server with:

```bash
docker compose config --volumes
docker volume ls | grep hank
```

Also back up local override files:

- `/srv/hank-remote/HankServerside/.env.cloud`
- `/srv/hank-remote/HankServerside/.env.agent`
