# Runbook: Storage Failure

Use this when `/readyz` reports storage failure or the cloud process cannot connect to PostgreSQL.

## Check

1. inspect the cloud and PostgreSQL container logs for connection or migration errors
2. verify `HANK_REMOTE_CLOUD_DATABASE_URL` points at the expected PostgreSQL service and database
3. verify the PostgreSQL container is healthy and its data volume is mounted
4. verify the database filesystem is not full

## Recovery

1. restore PostgreSQL availability or free disk space
2. if the PostgreSQL data directory is corrupt, restore from backup
3. restart the Compose stack or at least the `postgres` and `cloud` services
4. confirm `/readyz` returns `200` again

## Verify

- `/readyz` reports `storage: ready`
- auth and home-management endpoints work again
- existing homes and sessions are present after restore
