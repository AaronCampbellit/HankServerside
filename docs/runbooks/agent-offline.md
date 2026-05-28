# Runbook: Agent Offline

Use this when app requests fail with `agent_offline` or the cloud shows a home with no online agent.

## Check

1. Verify `GET /readyz` still reports storage as ready.
2. Check `/metrics` for the current `hank_remote_online_agents` value.
3. Confirm the agent process is running on the home machine.
4. Confirm the agent can reach the cloud `wss://.../ws/agent` URL outbound.

## Common Causes

- invalid or revoked agent token
- expired agent token
- cloud URL mismatch
- local network outage
- agent process crash

## Recovery

1. Restart the agent process.
2. If auth is failing, issue a replacement token from `POST /v1/home/agent/tokens`.
3. Update the agent env file with the replacement token.
4. Restart the agent and verify the cloud reports it online.
5. Revoke the old token once the new token is confirmed working.

## Verify

- `/metrics` shows `hank_remote_online_agents 1` for the active test environment
- the app can route `system.ping`
- Home Assistant or file commands succeed again
