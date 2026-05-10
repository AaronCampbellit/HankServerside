# Setup And Onboarding

Use this guide for the current Hank Remote server-side setup.

The supported deployment is one Docker Compose stack on one server:

- `postgres`: the live Hank Remote database
- `db-ops`: backup, checksum, and restore worker
- `cloud`: the dashboard, app API, and WebSocket relay
- `agent`: the home connector, started after the first admin creates a setup token
- `postgres-restore`: restore-test database, started only by the restore flow

## 1. Server Folder

Use this folder on the server:

```bash
/srv/hank-remote/HankServerside
```

For a fresh server:

```bash
sudo mkdir -p /srv/hank-remote
sudo chown "$USER":"$USER" /srv/hank-remote
cd /srv/hank-remote
git clone <your-hankserverside-repo-url> HankServerside
cd /srv/hank-remote/HankServerside
```

Do not create `data/postgres`, `data/files`, or `data/notes` by hand. Docker creates the named volumes.

## 2. Env Files

There are two private env files in the repo root:

- `/srv/hank-remote/HankServerside/.env.cloud`
- `/srv/hank-remote/HankServerside/.env.agent`

They are ignored by git. Keep real passwords, tokens, and backup encryption passphrases only in those private files or in your server secret manager.

Docker Compose uses those repo-root env files directly. The only file left under `configs/` is `configs/pgbackrest.conf`, which is a real pgBackRest config asset, not an env template.

## 3. Create `.env.cloud`

Create this file before first boot:

```bash
cd /srv/hank-remote/HankServerside
nano .env.cloud
```

Use this shape:

```env
HANK_REMOTE_CLOUD_ADDR=:8080
HANK_REMOTE_CLOUD_HOST_BIND=0.0.0.0
HANK_REMOTE_CLOUD_HOST_PORT=18080

POSTGRES_DB=hankremote
POSTGRES_USER=hankremote
POSTGRES_PASSWORD=replace-with-real-db-password
HANK_REMOTE_CLOUD_DATABASE_URL=postgres://hankremote:replace-with-real-db-password@postgres:5432/hankremote?sslmode=disable

HANK_REMOTE_SESSION_TTL_SECONDS=604800
HANK_REMOTE_REQUEST_TIMEOUT_SECONDS=30

HANK_REMOTE_AI_PROVIDER=auto
HANK_REMOTE_OLLAMA_BASE_URL=http://ollama:11434
HANK_REMOTE_OLLAMA_CHAT_MODEL=llama3.1
HANK_REMOTE_OLLAMA_EMBEDDING_MODEL=nomic-embed-text
HANK_REMOTE_AI_EMBEDDING_DIMENSION=768
HANK_REMOTE_PROJECT_DOCS_DIR=/app

# Optional OpenAI fallback/provider.
HANK_REMOTE_OPENAI_API_KEY=
HANK_REMOTE_OPENAI_CHAT_MODEL=gpt-4o-mini
HANK_REMOTE_OPENAI_EMBEDDING_MODEL=text-embedding-3-small

# Experimental ChatGPT/Codex link for subscription-backed HankAI chat.
# Keep disabled unless HANK_REMOTE_AI_PROVIDER is chatgpt_codex or you want auto mode to use linked ChatGPT.
HANK_REMOTE_CHATGPT_OAUTH_ENABLED=false
HANK_REMOTE_CHATGPT_AUTH_ISSUER=https://auth.openai.com
HANK_REMOTE_CHATGPT_BACKEND_BASE_URL=https://chatgpt.com/backend-api/codex
HANK_REMOTE_CHATGPT_CLIENT_ID=app_EMoamEEZ73f0CkXaXp7hrann
HANK_REMOTE_CHATGPT_CHAT_MODEL=gpt-5.4-mini

# Optional APNs push notifications for the Hank iOS app.
# Leave blank locally; device registration routes still work with a no-op sender.
HANK_REMOTE_APNS_TEAM_ID=
HANK_REMOTE_APNS_KEY_ID=
HANK_REMOTE_APNS_PRIVATE_KEY=
HANK_REMOTE_APNS_TOPIC=com.dropfile.Hank
HANK_REMOTE_APNS_ENVIRONMENT=sandbox

HANK_REMOTE_DB_OPS_STATE_DIR=/var/lib/hank/db-ops/state
HANK_REMOTE_DB_OPS_LOG_DIR=/var/log/hank/db-ops
HANK_REMOTE_DB_OPS_INTENT_SECRET=replace-with-real-db-ops-secret
HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS=replace-with-real-backup-encryption-passphrase
HANK_REMOTE_DB_OPS_STANZA=hank
HANK_REMOTE_DB_OPS_PGDATA=/var/lib/postgresql/data
HANK_REMOTE_DB_OPS_RESTORE_PGDATA=/var/lib/postgresql/restore
HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL=postgres://hankremote:replace-with-real-db-password@postgres-restore:5432/hankremote?sslmode=disable
HANK_REMOTE_DB_OPS_COMPOSE_FILE=/workspace/docker-compose.yml
```

Use the same database password in `POSTGRES_PASSWORD`, `HANK_REMOTE_CLOUD_DATABASE_URL`, and `HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL`.
Do not wrap either database URL in `< >`; keep the query string exactly as `?sslmode=disable` for the Compose stack.

Keep `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`. Encrypted pgBackRest backups cannot be restored without it.

`HANK_REMOTE_AI_PROVIDER=openai` still means the supported OpenAI API-key path using `HANK_REMOTE_OPENAI_API_KEY`. `HANK_REMOTE_AI_PROVIDER=chatgpt_codex` uses the experimental ChatGPT/Codex device-code link for chat only. Embeddings continue to use Ollama, the OpenAI API key, or Hank Remote's local fallback; ChatGPT subscription OAuth is not used as an embeddings credential.

After signing in to the dashboard, open `AI Settings` to manage the HankAI harness. Those settings are stored in the database and apply immediately to the next HankAI message:
- which Hank sources can be sent to the active provider
- whether HankAI can use your private past conversations as memory
- the system prompt HankAI uses
- the server-owned maximum context window used for provider requests

`Project docs` is one of those sources. By default the Docker image makes `README.md`, `AGENTS.md`, `SERVER_SYNC.md`, and every markdown file under `docs/` available from `/app`. For local runs, `HANK_REMOTE_PROJECT_DOCS_DIR=.` points HankAI at the checkout root.
The Hank dashboard and AI Settings page show the current index counts, embedding counts, and whether retrieval is using `pgvector` or the JSON embedding fallback.

If the server should only be reached by a local Cloudflare Tunnel or local reverse proxy, use:

```env
HANK_REMOTE_CLOUD_HOST_BIND=127.0.0.1
```

If port `18080` is already in use, change:

```env
HANK_REMOTE_CLOUD_HOST_PORT=18081
```

Important: run Compose with `--env-file .env.cloud`. The stack requires `.env.cloud`, and Compose needs that flag for host bind and port interpolation.

## 4. First Boot

Start only the first-boot services:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud up --build -d
docker compose --env-file .env.cloud ps
```

Expected running services:

- `postgres`
- `db-ops`
- `cloud`

The `agent` should not be running yet.

Check the server:

```bash
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/readyz
curl http://127.0.0.1:18080/metrics | head
```

Use your custom port if you changed `HANK_REMOTE_CLOUD_HOST_PORT`.

## 5. Public URL

Point Cloudflare Tunnel or your reverse proxy at the cloud service:

```text
http://127.0.0.1:18080
```

or, if your proxy runs from another machine:

```text
http://<server-ip>:18080
```

The proxy must allow WebSocket upgrades for:

- `/ws/app`
- `/ws/agent`

Postgres is not published to the host. It stays on private Docker networks.

## 6. First Admin

Open the public Hank Remote URL.

On a fresh database:

1. Register the first account.
2. That account becomes the deployment admin.
3. The singleton Home is created automatically.
4. The dashboard opens for that admin.

Registration is disabled after this first setup. Additional users should be added from the dashboard invitation flow.

## 7. Create `.env.agent`

In the dashboard, create an agent setup token from the Home agent section. The token is shown once as a full `.env.agent` block.

Paste that block into:

```bash
cd /srv/hank-remote/HankServerside
nano .env.agent
```

The generated file should look like this:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
HANK_REMOTE_AGENT_ID=home-main
HANK_REMOTE_AGENT_TOKEN=replace-with-issued-token
HANK_REMOTE_AGENT_HOME_NAME=Home

HANK_REMOTE_HA_BASE_URL=http://homeassistant:8123
HANK_REMOTE_HA_TOKEN=replace-with-home-assistant-token
HANK_REMOTE_HA_TIMEOUT_SECONDS=10

HANK_REMOTE_SMB_HOST=
HANK_REMOTE_SMB_SHARE=
HANK_REMOTE_SMB_USERNAME=
HANK_REMOTE_SMB_PASSWORD=
HANK_REMOTE_SMB_DOMAIN=

HANK_REMOTE_AGENT_FILES_ROOT=/srv/hank/files
HANK_REMOTE_AGENT_NOTES_ROOT=/srv/hank/notes
```

Keep this value unchanged for the single-server Compose deployment:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

Leave the SMB values blank unless you are using SMB. When SMB is blank, the agent uses the Docker-managed `hank_agent_files` volume for file operations.

## 8. Start The Agent

After `.env.agent` exists:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud --profile agent up -d agent
docker compose --env-file .env.cloud --profile agent ps
```

Check the agent logs:

```bash
docker compose --env-file .env.cloud --profile agent logs -f agent
```

The dashboard should show the Home agent as online.

## 9. Storage And Backups

Open `/dashboard/storage` as an admin.

After first boot:

1. Confirm checksum status is enabled.
2. Run a manual backup.
3. Run a restore test after the first backup exists.
4. Confirm storage status shows no new failures.

Default schedule:

- full backup: Sunday at 02:00
- differential backup: Monday through Saturday at 02:00
- checksum status: every 15 minutes
- `pg_amcheck`: Sunday at 03:30
- restore verification: Sunday at 04:00

Back up these volumes and files:

- `hank_pgbackrest_repo`
- `hank_db_ops_state`
- `hank_agent_files`
- `hank_agent_notes`
- `/srv/hank-remote/HankServerside/.env.cloud`
- `/srv/hank-remote/HankServerside/.env.agent`

`hank_postgres_data` is the live database volume. Once pgBackRest is running, `hank_pgbackrest_repo` is the database restore source.

## 10. Normal Updates

After the agent is active, update the stack with the agent profile:

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --env-file .env.cloud --profile agent up --build -d
```

Check status:

```bash
docker compose --env-file .env.cloud --profile agent ps
curl http://127.0.0.1:18080/readyz
```

Use your custom port if you changed `HANK_REMOTE_CLOUD_HOST_PORT`.

## Verification Checklist

- `.env.cloud` exists in `/srv/hank-remote/HankServerside`
- `.env.cloud` has real database, db-ops, and backup encryption secrets
- `.env.agent` does not exist until after the first admin creates an agent token
- first boot starts `postgres`, `db-ops`, and `cloud`
- first admin registration works once on a fresh database
- later public registration returns `403`
- the agent starts only after `.env.agent` exists
- `/dashboard/storage` loads for admins
- Home agent status becomes online
- the Hank app can sign in through the public URL
