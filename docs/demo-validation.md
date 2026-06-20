# Hank Demo Validation

This document keeps demo-server testing separate from production code. The validation harness is committed because it proves production readiness, but demo secrets, live host details, and generated reports stay private and untracked.

## Committed Validation Files

These files are production-safe and should be committed with the backend repair work:

- `tools/livevalidation/main.go`: end-to-end live app, agent, Home Assistant, and file-flow validation.
- `tools/adminvalidation/main.go`: admin UI/API validation for current Settings > Backups audit/query telemetry, File Server file jobs, and query telemetry APIs.
- `tools/loadtest/loadtest_test.go`: single-home target load scenarios and JSON report output.
- `scripts/restart-validation.sh`: restart-recovery test wrapper.
- `scripts/file-safety-validation.sh`: file policy and managed-job safety wrapper.
- `scripts/migration-baseline-validation.sh`: fresh DB plus baseline validation.
- `scripts/schema-drift-check.sh`: live schema drift comparison.
- `scripts/scale-validation.sh`: synthetic 1M file-index, 100k note, and attachment-size fixture.
- `scripts/production-load-report.sh`: load test plus resource report.
- `scripts/backup-during-traffic.sh`: backup and restore-test while load is running.
- `scripts/restore-proof.sh`: restore proof report against `postgres-restore`.
- `scripts/query-telemetry-report.sh`: top query report from `pg_stat_statements`.
- `scripts/metrics-assert.sh`: authenticated metrics coverage assertion.
- `scripts/bootstrap-first-run.sh`: fresh-server first boot for demo and production-like installs.
- `scripts/doctor.sh`: post-bootstrap and post-update health check.

## Demo-Only Private Inputs

Do not commit these:

- `.env.cloud`
- `.env.agent`
- any Home Assistant token
- any SMB username/password or share-specific secret
- any Cloudflare tunnel token or one-off tunnel command
- any demo host SSH key, known-host file, or password
- any live session token used by validation tools
- generated `data/` report artifacts

The repository `.gitignore` keeps `.env.*` and `data/` out of source control. Keep any demo-specific credentials in the operator's private password manager or private server notes.

## Demo Environment Variables

Use environment variables to bind the generic validation tools to a specific demo server. Do not hard-code demo hostnames, LAN IPs, share names, or tokens into source files.

Common variables:

```bash
export HANK_REMOTE_LIVE_BASE_URL="https://your-demo-host.example.com"
export HANK_REMOTE_LIVE_SESSION_TOKEN="<private session token>"
export HANK_REMOTE_LIVE_SOURCE_ONE="replace-with-first-demo-source-id"
export HANK_REMOTE_LIVE_SOURCE_TWO="replace-with-second-demo-source-id"

export HANK_REMOTE_LOADTEST_BASE_URL="$HANK_REMOTE_LIVE_BASE_URL"
export HANK_REMOTE_LOADTEST_SESSION_TOKEN="$HANK_REMOTE_LIVE_SESSION_TOKEN"
export HANK_REMOTE_LOADTEST_FILE_SOURCE="$HANK_REMOTE_LIVE_SOURCE_ONE"
export HANK_REMOTE_LOADTEST_FILE_SOURCE_TWO="$HANK_REMOTE_LIVE_SOURCE_TWO"
```

For local compose validation, also set:

```bash
export COMPOSE_PROJECT_NAME="hank_validation"
export HANK_REMOTE_CLOUD_ENV_FILE=".env.cloud"
```

## HankAI Local Model Eval

For the current demo setup, the Hank cloud can test against the LAN Ollama
instance at:

```bash
export HANK_REMOTE_OLLAMA_BASE_URL="http://192.168.86.158:11434"
```

Before running the eval harness, verify the demo host and the running cloud
container can reach Ollama:

```bash
curl -fsS http://192.168.86.158:11434/api/tags
docker compose --env-file .env.cloud exec cloud sh -lc 'wget -qO- http://192.168.86.158:11434/api/tags'
```

Run the live HankAI eval harness with:

```bash
HANK_REMOTE_LIVE_BASE_URL="https://hankdemo.campbellservers.com" \
HANK_REMOTE_LIVE_SESSION_TOKEN="$HANK_REMOTE_LIVE_SESSION_TOKEN" \
HANK_REMOTE_HANKAI_EXPECT_PROVIDER="ollama" \
HANK_REMOTE_HANKAI_EXPECT_OLLAMA_URL="http://192.168.86.158:11434" \
go run ./tools/hankaieval
```

Reports are generated under `data/hankai-evals/` and must remain untracked.

## Future Demo Run Order

Run this sequence after the demo stack is up, the agent is online, Home Assistant is reachable, and two file sources are configured with synthetic test data only:

```bash
HANK_REMOTE_BOOTSTRAP_NONINTERACTIVE=true \
HANK_REMOTE_BOOTSTRAP_HOST_BIND=127.0.0.1 \
HANK_REMOTE_BOOTSTRAP_HOST_PORT=18080 \
scripts/bootstrap-first-run.sh
scripts/doctor.sh
```

```bash
make fmt
make tidy
make build
go test -count=1 ./...
```

```bash
promtool check rules ops/prometheus/alerts.yml
scripts/restart-validation.sh
scripts/file-safety-validation.sh
scripts/schema-drift-check.sh
scripts/migration-baseline-validation.sh
go run ./tools/livevalidation
go run ./tools/hankaieval
go run ./tools/adminvalidation
scripts/scale-validation.sh
scripts/production-load-report.sh
scripts/backup-during-traffic.sh
scripts/restore-proof.sh
scripts/query-telemetry-report.sh
scripts/metrics-assert.sh
```

`tools/adminvalidation` follows current canonical dashboard routes. If a route
split removes or renames an operator page, update this tool in the same change
so demo validation does not silently test stale UI paths.

If `promtool` is unavailable on the demo host, install Prometheus tooling on that host rather than editing the alert rules around the missing binary.

## Generated Artifacts

Validation output is intentionally generated under `data/` and should remain untracked:

- `data/restart-validation/`
- `data/file-safety-validation/`
- `data/schema-drift/`
- `data/migration-baseline/`
- `data/scale-validation/`
- `data/load-reports/`
- `data/backup-traffic/`
- `data/restore-reports/`
- `data/query-telemetry/`

Keep important demo evidence by copying report paths into the release notes or operator notes, not by committing generated reports.

## Synthetic Data Rule

Demo validation must use synthetic files, notes, attachments, and assistant-index rows. Do not run destructive file-operation tests against real user data. The validation tools create paths under `_hank_validation/` and `_hank_load/`; if a run fails midway, clean those prefixes from the configured demo file sources before the next run.
