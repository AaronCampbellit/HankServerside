# First-Time Deployment

Use this for a fresh Hank Remote install on one server.

The full current setup and onboarding flow is here:

- `docs/setup-and-onboarding.md`

## Fresh Install Summary

1. Clone the repo into:

```bash
/srv/hank-remote/HankServerside
```

2. Create the private cloud env file:

```bash
cd /srv/hank-remote/HankServerside
nano .env.cloud
chmod 600 .env.cloud
```

It must include real values for:

- `POSTGRES_PASSWORD`
- `HANK_REMOTE_CLOUD_DATABASE_URL`
- `HANK_REMOTE_DB_OPS_INTENT_SECRET`
- `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`
- `HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL`

3. Start first boot:

```bash
cd /srv/hank-remote/HankServerside
docker compose --env-file .env.cloud up --build -d
docker compose --env-file .env.cloud ps
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

## Env File Locations

- cloud secrets: `/srv/hank-remote/HankServerside/.env.cloud`
- agent token and local connector settings: `/srv/hank-remote/HankServerside/.env.agent`

`.env.agent` can contain SMB passwords and Home Assistant tokens. Keep both env files mode `0600`.

Do not create host `data/` folders for Postgres, files, or notes. Docker named volumes are used by default.

## Normal Updates

After the agent is active:

```bash
cd /srv/hank-remote/HankServerside
git pull
docker compose --env-file .env.cloud --profile agent up --build -d
```
