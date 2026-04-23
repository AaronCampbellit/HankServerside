# First-Time Deployment

Use this for a fresh HankServerside install on one server.

The first boot starts only:

- `postgres`
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
git clone <your-hankserverside-repo-url> .
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

If port `18080` is already used, create `.env.cloud`:

```env
HANK_REMOTE_CLOUD_HOST_PORT=18081
```

## 4. Start First Boot

```bash
cd /srv/hank-remote
docker compose up --build -d
docker compose ps
```

Expected first-boot services:

- `postgres`
- `cloud`

The `agent` should not be running yet.

Check the cloud:

```bash
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/readyz
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
cd /srv/hank-remote
nano .env.agent
```

Edit these only if you need them:

- `HANK_REMOTE_HA_BASE_URL`
- `HANK_REMOTE_HA_TOKEN`
- `HANK_REMOTE_SMB_*`

Leave SMB blank to use the Docker-managed files volume.

## 8. Start the Agent

```bash
cd /srv/hank-remote
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
cd /srv/hank-remote
git pull
docker compose --profile agent up --build -d
```

## 10. Backups

Back up these Compose volumes. Docker may prefix their actual names with the Compose project name:

- `hank_postgres_data`
- `hank_agent_files`
- `hank_agent_notes`

List the exact names on the server with:

```bash
docker compose config --volumes
docker volume ls | grep hank
```

Also back up local override files:

- `/srv/hank-remote/.env.cloud`
- `/srv/hank-remote/.env.agent`
