# Security Hardening Todo

This is the short implementation list from the HankServerside security review.
Items marked implemented are retained as historical rationale; use each section's
current-risk line for remaining work.

## 1. Lock Down Metrics

Status: implemented. `/metrics` now requires an authenticated admin session or, when `HANK_REMOTE_METRICS_SCRAPE_TOKEN` is set, a dedicated Prometheus scrape token; `/healthz` and `/readyz` remain public for deployment checks. The compose `monitoring` profile ships Prometheus + Alertmanager bound to `127.0.0.1` (see `docs/runbooks/single-host-compose.md`).

Current risk: operators can still misconfigure a reverse proxy to expose an unauthenticated scrape path, and the scrape token must be rotated like any other server secret.

Fix:

- Require admin auth for `/metrics`, or bind metrics to an internal-only listener.
- Keep `/healthz` and `/readyz` available for deployment checks.
- Update deployment docs with the chosen metrics access pattern.

## 2. Add CSRF Protection

Status: implemented. Browser cookie-authenticated writes require the dashboard CSRF cookie value in `X-Hank-CSRF-Token`; Bearer-token API clients and WebSocket routes remain exempt.

Current risk: browser writes depend on the dashboard CSRF cookie/header pair being preserved by reverse proxies and custom clients.

Fix:

- Issue a CSRF token to authenticated dashboard sessions.
- Require it for cookie-authenticated `POST`, `PUT`, and `DELETE` requests.
- Exempt Bearer-token API clients and WebSocket upgrade routes as needed.
- Add tests for missing, invalid, and valid CSRF tokens.

## 3. Remove Agent Tokens From URLs

Status: implemented. The agent now sends `Authorization: Bearer <agent-token>` and `X-Hank-Agent-ID`; query-token support has been removed.

Current risk: old deployed agents that still send URL query credentials will be rejected. The operator upgrade path (new setup token, regenerated `.env.agent`, restart, revoke old token) is documented in `docs/runbooks/agent-offline.md` and `RELEASE.md`.

Fix:

- Move agent auth to `Authorization: Bearer <agent-token>` plus an agent ID header, or add a short-lived agent ticket endpoint.
- Keep token hashes in storage.
- Update `.env.agent` generation and agent connection code.
- Query-token support has been removed after the agent migration.

## 4. Encrypt Stored Secrets At Rest

Status: implemented for normal startup. `HANK_REMOTE_SECRET_ENCRYPTION_KEY` is required unless `HANK_REMOTE_ALLOW_PLAINTEXT_SECRETS=true` is explicitly set for local development. The key enables encrypted storage for OpenAI/ChatGPT OAuth tokens, APNs device tokens, and profile secret vault JSON. Service-profile secrets are not currently persisted by the cloud; they are relayed to the agent and applied there.

Current risk: old deployments that previously ran without `HANK_REMOTE_SECRET_ENCRYPTION_KEY` can still contain plaintext OAuth/APNs/profile-vault values until an admin runs the secret storage audit/remediation command. `scripts/doctor.sh` now runs `secrets status --strict` and fails until remediation is done, and the check is part of the `RELEASE.md` gate.

Fix:

- Keep the application encryption key sourced from environment or a server secret manager.
- Keep ChatGPT/OpenAI OAuth access and refresh tokens encrypted.
- Keep APNs device tokens and profile secret vault JSON encrypted.
- Use `hank-remote-cloud secrets status --strict` to detect likely plaintext legacy rows.
- Use `hank-remote-cloud secrets reencrypt` after setting `HANK_REMOTE_SECRET_ENCRYPTION_KEY` to rewrite known plaintext legacy rows.

## 5. Block Local File Symlink Escapes

Status: implemented for local file roots with symlink containment checks on read, write, stat, delete, and rename paths.

Current risk: local file root path checks block `../` traversal, but symlinks inside the root may point outside it.

Fix:

- Resolve symlinks before local file reads, writes, stats, deletes, and renames.
- Verify the final resolved path remains inside the configured root.
- Add regression tests for symlink-to-outside-root reads and writes.

## 6. Harden Note Attachment Paths

Status: implemented with cleaned, symlink-resolved root containment checks for note attachment read/write/delete paths.

Current risk: corrupted historical rows or manually copied files can still leave orphaned bytes under the attachment root until lifecycle cleanup removes safe stale files.

Fix:

- Add a helper that joins attachment root and storage key, cleans it, resolves it, and verifies containment.
- Use it for attachment read/write/delete paths.
- Add tests for malicious or corrupted storage keys.

## 7. Strengthen Destructive Admin Actions

Status: implemented. Primary restore requires the typed confirmation phrase plus a short-lived admin action token.

Current risk: admin session compromise gives access to high-impact storage operations.

Fix:

- Require re-authentication or a short-lived admin action token for primary restore.
- Keep the current typed confirmation phrase.
- Log who requested the action and when.
- Add tests for missing re-auth/admin action token.

## 8. Improve Login Abuse Protection

Status: implemented. Login now has per-email in-memory exponential backoff in addition to IP rate limiting.

Current risk: login backoff rows now have lifecycle cleanup, but policy tuning and alerting are still operator responsibilities.

Fix:

- Add per-email or per-user login backoff.
- Keep generic login failure messages.
- Consider persistent failed-login counters with expiry.
- Add metrics for lockout/backoff events.

## 9. Add Secret Rotation Runbooks

Status: documented below.

Current risk: rotation expectations are spread across docs.

Fix:

- Document rotation for:
  - agent tokens
  - `HANK_REMOTE_DB_OPS_INTENT_SECRET`
  - `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`
  - OpenAI/ChatGPT linked credentials
  - APNs credentials
  - service-profile secrets
- Include verification steps after each rotation.

Rotation notes:

- Agent tokens: create a new setup token in the dashboard, update `.env.agent`, restart the agent, confirm it comes online, then revoke the old token.
- `HANK_REMOTE_DB_OPS_INTENT_SECRET`: stop the stack, update `.env.cloud`, restart `cloud` and `db-ops`, then run a backup status check and one manual backup request.
- `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`: rotate only with a planned pgBackRest repository migration. Verify restore-test success before deleting the previous repository/passphrase.
- `HANK_REMOTE_SECRET_ENCRYPTION_KEY`: do not rotate casually. Existing encrypted OAuth/APNs/profile-vault values depend on it. A future rotation should decrypt with the old key, re-encrypt with the new key, then verify linked ChatGPT/OpenAI status, APNs registration, and profile secret vault reads.
- OpenAI/ChatGPT linked credentials: unlink/relink the account, then verify `/v1/oauth/openai/status` and send a HankAI test message.
- APNs credentials: update `.env.cloud`, restart `cloud`, re-register an iOS device, then verify a test notification path.
- Service-profile secrets: update the Home Assistant/file-server settings in the dashboard, verify the agent applies the new version, and confirm the relevant Home Assistant or file operation succeeds.
