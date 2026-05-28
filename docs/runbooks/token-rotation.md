# Runbook: Token Rotation

Use this when rotating agent credentials without losing service.

## Agent Token Rotation

1. Issue a new token with `POST /v1/home/agent/tokens`.
2. Leave the old token valid temporarily.
3. Update the agent env file with the new token.
4. Restart the agent and verify it reconnects successfully.
5. Revoke the old token with `DELETE /v1/home/agent/tokens/{tokenID}`.

This flow provides overlapping validity during rotation, which avoids unnecessary downtime.

## App Session Rotation

App sessions already rotate naturally on fresh login and are revoked on logout. For a suspected leak:

1. log out the current session
2. log in again to receive a fresh session token
3. reconnect the app WebSocket with the new token

## Verify

- the replacement token works before the old token is revoked
- the old token no longer works after revocation
- `/metrics` shows no continued auth failures after rotation
