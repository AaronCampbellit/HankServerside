# Runbook: File Transfer Failure

Use this when `POST /v1/home/files/uploads`, `POST /v1/home/files/downloads`, `GET /v1/file-transfers/{id}`, or `PUT /v1/file-transfers/{id}` fail.

## Check

1. verify the home agent is online
2. verify the configured file root exists on the agent host
3. confirm the transfer URL still has a valid transfer token
4. if resuming, confirm the client is using the correct `offset`

## Common Causes

- target path is outside the configured file root
- transfer expired before retry
- agent disconnected mid-transfer
- client resumed with the wrong offset

## Recovery

1. retry the transfer if the transfer session is still valid
2. if resume fails repeatedly, create a fresh upload or download session
3. if the agent disconnected, restore the agent connection first
4. for persistent path failures, verify the local file root and permissions on the agent machine

## Verify

- the transfer completes with `200`
- resumed transfers continue from the requested byte offset
- the resulting file content matches the expected source
