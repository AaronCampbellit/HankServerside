# Deployment And Setup

Use this as the single current guide for Hank Remote server-side setup and deployment.

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

## 3. Recommended Bootstrap

On a fresh server, use the bootstrap script. It creates `.env.cloud`, builds the first-boot images, starts Postgres, runs migrations, starts `cloud` and `db-ops`, and checks `/healthz` plus `/readyz`.

```bash
cd /srv/hank-remote/HankServerside
scripts/bootstrap-first-run.sh
```

For an unattended install, set values before running the script:

```bash
cd /srv/hank-remote/HankServerside
HANK_REMOTE_BOOTSTRAP_NONINTERACTIVE=true \
HANK_REMOTE_BOOTSTRAP_HOST_BIND=127.0.0.1 \
HANK_REMOTE_BOOTSTRAP_HOST_PORT=18080 \
scripts/bootstrap-first-run.sh
```

The bootstrap default bind is `127.0.0.1`, which is the safe setting for Cloudflare Tunnel or a local reverse proxy on the same server. Use `HANK_REMOTE_BOOTSTRAP_HOST_BIND=0.0.0.0` only when the cloud HTTP port must be reachable directly on the server network.

Run the doctor after bootstrap and after later updates:

```bash
cd /srv/hank-remote/HankServerside
scripts/doctor.sh
```

## 4. Manual `.env.cloud` Reference

Use this only when you are not using `scripts/bootstrap-first-run.sh`. Create this file before first boot:

```bash
cd /srv/hank-remote/HankServerside
nano .env.cloud
chmod 600 .env.cloud
```

Use this shape:

```env
HANK_REMOTE_CLOUD_ADDR=:8080
HANK_REMOTE_CLOUD_HOST_BIND=127.0.0.1
HANK_REMOTE_CLOUD_HOST_PORT=18080

POSTGRES_DB=hankremote
POSTGRES_USER=hankremote
POSTGRES_PASSWORD=replace-with-real-db-password
HANK_REMOTE_CLOUD_DATABASE_URL=postgres://hankremote:replace-with-real-db-password@postgres:5432/hankremote?sslmode=disable

HANK_REMOTE_SESSION_TTL_SECONDS=604800
HANK_REMOTE_REQUEST_TIMEOUT_SECONDS=120
HANK_REMOTE_SECRET_ENCRYPTION_KEY=replace-with-stable-random-secret-encryption-key

HANK_REMOTE_AI_PROVIDER=auto
HANK_REMOTE_OLLAMA_BASE_URL=http://ollama:11434
HANK_REMOTE_OLLAMA_CHAT_MODEL=llama3.1
HANK_REMOTE_OLLAMA_EMBEDDING_MODEL=nomic-embed-text
# Embedding dimension is fixed at 768 by the production schema.
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
# Host Docker socket group id. On Linux: stat -c '%g' /var/run/docker.sock
HANK_REMOTE_DB_OPS_DOCKER_GID=
```

Use the same database password in `POSTGRES_PASSWORD`, `HANK_REMOTE_CLOUD_DATABASE_URL`, and `HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL`.
Do not wrap either database URL in `< >`; keep the query string exactly as `?sslmode=disable` for the Compose stack.

Keep `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`. Encrypted pgBackRest backups cannot be restored without it.

Set `HANK_REMOTE_DB_OPS_DOCKER_GID` to the numeric group owner of `/var/run/docker.sock` before starting `db-ops`. The db-ops container runs as a non-root user and needs that supplementary group for restore-test orchestration.

Keep `HANK_REMOTE_SECRET_ENCRYPTION_KEY` stable after first use. Hank Remote uses it to encrypt stored OAuth tokens, APNs device tokens, and profile secret vault data at rest. If this key is lost, already encrypted application secrets cannot be read and must be re-linked or re-entered.

`HANK_REMOTE_AI_PROVIDER=openai` still means the supported OpenAI API-key path using `HANK_REMOTE_OPENAI_API_KEY`. `HANK_REMOTE_AI_PROVIDER=chatgpt_codex` uses the experimental ChatGPT/Codex device-code link for chat only. Browser-redirect OpenAI OAuth is not supported. Embeddings continue to use Ollama, the OpenAI API key, or Hank Remote's local fallback; ChatGPT subscription OAuth is not used as an embeddings credential.
Production should run with pgvector available through the bundled Postgres image. The JSON embedding fallback exists only for local development or degraded local testing when pgvector is not available. The production vector schema is fixed at 768 dimensions. Do not set `HANK_REMOTE_AI_EMBEDDING_DIMENSION` in production unless a future migration explicitly changes the vector dimension.

After signing in to the dashboard, open `AI Settings` to manage the HankAI harness. Those settings are stored in the database and apply immediately to the next HankAI message:
- which Hank sources can be sent to the active provider
- whether HankAI can use your private past conversations as memory
- which configured AI provider HankAI should use for this Home/user, including Local Ollama, linked ChatGPT/Codex, OpenAI API key, or the configured default
- which chat and embedding model overrides HankAI should use for testing local and external providers
- which prompt profile is active: the stricter ChatGPT/Codex profile, the local-model planner profile, or a custom prompt
- the chat model override for the active chat provider, including ChatGPT/Codex subscription-backed chat
- the system prompt HankAI uses

The dashboard model controls do not replace server secrets or provider endpoints. Keep base URLs, API keys, and ChatGPT/Codex device-code settings in `.env.cloud`; use the GUI to switch between already configured providers, models, embedding models, and prompt profiles.
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

## 5. Manual First Boot

If you created `.env.cloud` manually, build and start only the first-boot services:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud build postgres cloud db-ops
docker compose --env-file .env.cloud up -d postgres
docker compose --env-file .env.cloud run --rm cloud /usr/local/bin/hank-remote-cloud migrate up
docker compose --env-file .env.cloud run --rm cloud /usr/local/bin/hank-remote-cloud migrate status --strict
docker compose --env-file .env.cloud up -d cloud db-ops
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
```

Use your custom port if you changed `HANK_REMOTE_CLOUD_HOST_PORT`.

`/metrics` requires a signed-in admin session. Scrape it through an authenticated internal path, or keep it behind an internal reverse proxy that injects admin credentials, for example:

```bash
curl -H "Authorization: Bearer $HANK_REMOTE_ADMIN_SESSION_TOKEN" \
  http://127.0.0.1:18080/metrics | head
```

Do not expose unauthenticated `/metrics` to the public internet. `/healthz` and `/readyz` stay unauthenticated for deployment checks.

## 6. Public URL

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

The agent authenticates to `/ws/agent` with `Authorization: Bearer <agent-token>` and `X-Hank-Agent-ID`. Query-string agent credentials are not supported.

Postgres is not published to the host. It stays on private Docker networks.

## 7. First Admin

Open the public Hank Remote URL.

On a fresh database:

1. Register the first account.
2. That account becomes the deployment admin.
3. The singleton Home is created automatically.
4. The dashboard opens for that admin.

Registration is disabled after this first setup. Additional users should be added from the dashboard invitation flow.

## 8. Create `.env.agent`

In the dashboard, create an agent setup token from the Home agent section. The token is shown once as a full `.env.agent` block.

Copy that block, then install it on the server. From your Mac, with the copied block in your clipboard:

```bash
pbpaste | ssh <server-user>@<server-host> 'cd /srv/hank-remote/HankServerside && scripts/install-agent-env.sh'
```

The helper writes `.env.agent` with mode `0600` and starts the Compose `agent` profile.

Or paste it manually inside an SSH session:

```bash
cd /srv/hank-remote/HankServerside
nano .env.agent
chmod 600 .env.agent
```

`.env.agent` contains the Hank Remote agent token and may later contain Settings-managed Home Assistant, SMB, and media-service credentials. Treat it as a secret file and keep mode `0600`.

The Compose agent container is allowed to update the bind-mounted `.env.agent` file so Settings > Connections can persist Home Assistant and SMB credentials after first start. SMB shares are stored only in `HANK_REMOTE_SMB_SHARES_JSON`. Keep the host file `0600`; only set `HANK_REMOTE_AGENT_CONTAINER_USER` if you also manage `.env.agent` and agent volume ownership for that custom container user.

The generated file should look like this:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
HANK_REMOTE_AGENT_ID=home-main
HANK_REMOTE_AGENT_TOKEN=replace-with-issued-token
HANK_REMOTE_AGENT_HOME_NAME=Home
HANK_REMOTE_AGENT_CONFIG_PATH=/app/.env.agent

HANK_REMOTE_AGENT_FILES_ROOT=/srv/hank/files
HANK_REMOTE_AGENT_NOTES_ROOT=/srv/hank/notes

# Optional Hermes Agent bridge for HankAI `/Hermes ...` chat.
HANK_REMOTE_HERMES_API_BASE_URL=
HANK_REMOTE_HERMES_API_KEY=
HANK_REMOTE_HERMES_MODEL=hermes-agent
HANK_REMOTE_HERMES_TIMEOUT_SECONDS=120
```

Keep this value unchanged for the single-server Compose deployment:

```env
HANK_REMOTE_AGENT_CLOUD_URL=ws://cloud:8080/ws/agent
```

After the agent is online, use Settings > Connections to save Home Assistant and SMB credentials; the agent persists those values back into `.env.agent`.

When no SMB shares are configured, the agent uses the Docker-managed `hank_agent_files` volume for file operations. For SMB shares, use Settings > Connections in the dashboard; the agent persists the share list in `HANK_REMOTE_SMB_SHARES_JSON`.

For Hermes Agent chat from HankAI, run Hermes with its API server enabled on the Hermes VM, then open Settings > Connections and save the Hermes API base URL plus API key. The dashboard sends the settings to the online home agent, and the agent persists them into `.env.agent` when `Save on home connector` is enabled. `HANK_REMOTE_HERMES_API_BASE_URL` should be the agent-reachable base URL, for example `http://hermes-vm:8642` or `http://hermes-vm:8642/v1`, and `HANK_REMOTE_HERMES_API_KEY` should match the Hermes `API_SERVER_KEY`. HankAI routes only explicit `/Hermes ...` prompts to Hermes, and the Hermes key stays in `.env.agent`.

If you have an older `.env.agent` with legacy single-share SMB keys, convert it before updating the agent:

```bash
cd /srv/hank-remote/HankServerside
scripts/migrate-agent-smb-env.sh .env.agent
```

## 9. Start The Agent

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

## 10. Storage And Backups

Open `/dashboard/settings#backups` as an admin.

After first boot:

1. Confirm checksum status is enabled.
2. Run a manual backup.
3. Run a restore test after the first backup exists.
4. Confirm storage status shows no new failures.

Primary restore still requires the typed confirmation phrase and now also uses a short-lived admin action token issued immediately before the restore request. The dashboard handles this automatically.

Default schedule:

- full backup: Sunday at 02:00
- differential backup: Monday through Saturday at 02:00
- checksum status: every 15 minutes
- `pg_amcheck`: Sunday at 03:30
- restore verification: Sunday at 04:00

Back up these volumes and files:

- `hank_pgbackrest_repo`
- `hank_note_attachments`
- `hank_db_ops_state`
- `hank_agent_files`
- `hank_agent_notes`
- `/srv/hank-remote/HankServerside/.env.cloud`
- `/srv/hank-remote/HankServerside/.env.agent`

`hank_postgres_data` is the live database volume. Once pgBackRest is running, `hank_pgbackrest_repo` is the database restore source.
`hank_note_attachments` stores note attachment files outside Postgres and must be retained with the database backups.

## 11. Normal Updates

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
scripts/doctor.sh
```

Use your custom port if you changed `HANK_REMOTE_CLOUD_HOST_PORT`.

## Verification Checklist

- `.env.cloud` exists in `/srv/hank-remote/HankServerside`
- `.env.cloud` has real database, db-ops, and backup encryption secrets
- `.env.cloud` has a stable `HANK_REMOTE_SECRET_ENCRYPTION_KEY`
- `.env.agent` does not exist until after the first admin creates an agent token
- first boot starts `postgres`, `db-ops`, and `cloud`
- first admin registration works once on a fresh database
- later public registration returns `403`
- the agent starts only after `.env.agent` exists
- `/dashboard/settings#backups` loads for admins
- Home agent status becomes online
- the Hank app can sign in through the public URL
