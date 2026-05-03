# Runbook: Storage Failure

Use this when `/readyz` reports storage failure, `/dashboard/storage` reports backup/checksum failures, or the cloud process cannot connect to PostgreSQL.

## Check

1. inspect the cloud, PostgreSQL, and db-ops logs:
   `docker compose --env-file .env.cloud logs -f cloud postgres db-ops`
2. open `/dashboard/storage` and check the failure log
3. verify `HANK_REMOTE_CLOUD_DATABASE_URL` points at the expected PostgreSQL service and database
4. verify the PostgreSQL container is healthy and its data volume is mounted
5. verify the pgBackRest repository and database filesystems are not full
6. confirm checksums are enabled:
   `docker compose --env-file .env.cloud exec postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select current_setting('data_checksums')"`
7. confirm `.env.cloud` has the same `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS` used when the encrypted backup was created
8. confirm Postgres is not exposed on the host:
   `docker compose --env-file .env.cloud ps postgres postgres-restore`

## Recovery

1. restore PostgreSQL availability or free disk space
2. if checksums are disabled on an existing cluster, schedule maintenance and run `scripts/enable-pg-checksums.sh`
3. if `pg_amcheck` or checksum logs report corruption, use `/dashboard/storage` to run a restore test against the latest good pgBackRest backup
4. if the restore test passes and primary data is corrupt, request primary restore from `/dashboard/storage`
5. restart the Compose stack or at least `postgres`, `db-ops`, and `cloud` if restore orchestration did not restart them
6. confirm `/readyz` returns `200` again

## Common Storage Events

- `pgBackRest stanza creation failed. exit status 31`: check the redacted output excerpt in `/dashboard/storage` first. If it mentions cipher/decrypt, restore the original `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`; an encrypted pgBackRest repo cannot be read with a new passphrase. If it mentions `repo1-path` or repository access, set the backup target back to `/var/lib/pgbackrest` unless the Compose file also mounts the custom path into both `postgres` and `db-ops`.
- `option 'repo1-cipher-pass' is not allowed on the command-line`: redeploy the latest Compose stack. The passphrase must be supplied through `PGBACKREST_REPO1_CIPHER_PASS`, not as a pgBackRest command argument.
- `Restore verification database URL is invalid.` or `missing key/value separator "=" in URI query parameter`: fix `HANK_REMOTE_DB_OPS_RESTORE_DATABASE_URL` in `.env.cloud`. It must not be wrapped in `< >`, and the query string should be `?sslmode=disable`.
- `pg_amcheck could not complete.`: treat this as setup or connectivity failure until the output excerpt reports corruption. Confirm the database URL works from `db-ops`, then rerun the check.
- `pg_amcheck reported a database integrity problem.`: treat this as a real database integrity incident. Stop writes if possible, run a restore test from the newest good backup, and preserve the output excerpt for diagnosis.
- `PostgreSQL data checksums are not enabled for this cluster.`: this is expected for a database volume created before checksums were added to Compose. It is not repaired by restarting; schedule downtime and run `scripts/enable-pg-checksums.sh`.

## Log Cleanup

`/dashboard/storage` shows one combined storage log. Admins can clear it when they want a fresh troubleshooting run. The event file is also pruned automatically to the newest 2,000 entries and at most 5 MB so storage logs do not grow without bound. Compose also rotates service stdout/stderr logs at 10 MB per file with 3 files kept per service.

## Task Status

`/dashboard/storage` shows queued and running backup/restore work above the backup list. While a full backup, differential backup, restore test, or primary restore is active, the page refreshes automatically and displays the latest worker step.

## Verify

- `/readyz` reports `storage: ready`
- auth and home-management endpoints work again
- existing homes and sessions are present after restore
- `/dashboard/storage` shows the latest backup/checksum task without new failures
- `/metrics` includes `hank_remote_db_backup_last_success_unixtime` after a successful backup
- a restore test reports success and does not report missing Hank tables or login-role drift
