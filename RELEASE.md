# Hank Remote Release Checklist

Use this checklist for every tagged release. A release is not stable until every
gate below is green and the evidence locations are recorded in the release notes.

## 1. Version And Tag

- Pick the version (`vMAJOR.MINOR.PATCH`, e.g. `v1.0.0-rc1`).
- Build images with the version stamped so `/dashboard` bootstrap and
  `cloud_runtime.version` report it:

```bash
export HANK_REMOTE_BUILD_VERSION="$(git describe --tags --always --dirty)"
export HANK_REMOTE_SOURCE_COMMIT="$(git rev-parse HEAD)"
docker compose --env-file .env.cloud build cloud
```

- `scripts/bootstrap-first-run.sh` exports both variables automatically when it
  runs inside a git checkout.
- Tag only after the full gate passes: `git tag vX.Y.Z && git push origin vX.Y.Z`.

## 2. Local Gate

```bash
make fmt
make tidy
go build ./...
go vet ./...
go test ./...
npm --prefix web/dashboard run check
```

Note: PostgreSQL-backed tests skip unless `HANK_REMOTE_TEST_DATABASE_URL` is
set. Run the full suite against a real Postgres (locally or on the demo server)
before release; a run where the store/cloud DB tests skipped does not count.

## 3. Database Gate

```bash
make migrate-status
make schema-drift-check
```

- Fresh-install proof: `scripts/migration-baseline-validation.sh`.
- No schema change ships outside `internal/migrations/sql`.

## 4. Secret Storage Gate

```bash
docker compose --env-file .env.cloud run --rm cloud \
  /usr/local/bin/hank-remote-cloud secrets status --strict
```

- Must exit clean. If legacy plaintext rows are reported, run
  `secrets reencrypt` with `HANK_REMOTE_SECRET_ENCRYPTION_KEY` set, then re-run
  the strict status check.
- `scripts/doctor.sh` runs this check automatically when the stack is up.

## 5. Live Validation Gate (demo or staging server)

Run the sequence from `docs/demo-validation.md`:

```bash
scripts/doctor.sh
promtool check rules ops/prometheus/alerts.yml
scripts/restart-validation.sh
scripts/file-safety-validation.sh
scripts/schema-drift-check.sh
scripts/migration-baseline-validation.sh
go run ./tools/livevalidation
go run ./tools/adminvalidation
scripts/restore-proof.sh
scripts/metrics-assert.sh
```

Record report paths (under `data/`) in the release notes; the artifacts stay
untracked. Restore proof must be newer than 7 days at release time.

## 6. Operator Upgrade Notes

Include in every release's notes:

- **Migrations**: run `make migrate-up` (or let bootstrap do it) before starting
  the new cloud image; verify with `make migrate-status`.
- **Agents older than the header-auth migration**: agents that still authenticate
  with URL query tokens are rejected. Upgrade path: create a new setup token in
  the dashboard, regenerate `.env.agent` (`scripts/install-agent-env.sh`),
  restart the agent, confirm it shows online, then revoke the old token. See
  `docs/runbooks/agent-offline.md`.
- **Secret encryption**: deployments that ever ran without
  `HANK_REMOTE_SECRET_ENCRYPTION_KEY` must pass the strict secrets status check
  (section 4) before the release is considered applied.
- **Env file permissions**: `.env.cloud` and `.env.agent` must be `chmod 600`.

## 7. Monitoring Gate

- Prometheus and Alertmanager are up (`docker compose --profile monitoring ps`)
  and `ops/prometheus/alerts.yml` loads without errors.
- At least one Alertmanager notification receiver is configured and has been
  test-fired.
- `scripts/metrics-assert.sh` passes against the deployed instance.
