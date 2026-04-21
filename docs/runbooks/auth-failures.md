# Runbook: Auth Failures

Use this when app login, app WebSocket auth, or agent WebSocket auth is failing.

## App Auth

Check:

1. the app is sending `Authorization: Bearer <session_token>` or `session_token` on `/ws/app`
2. the session has not expired
3. the session was not revoked by logout

Recovery:

1. log in again with `POST /v1/auth/login`
2. verify `GET /v1/me` works with the new session token
3. reconnect `/ws/app`

## Agent Auth

Check:

1. `agent_id` matches the token’s agent record
2. the token has not been revoked
3. the token has not expired
4. the agent is connecting to the correct cloud environment

Recovery:

1. issue a replacement token from `POST /v1/homes/{homeID}/agents/tokens`
2. update the agent env file
3. restart the agent
4. revoke the old token after the replacement is live

## Verify

- `/metrics` auth-failure counters stop increasing for the failing path
- the next app or agent connection succeeds
