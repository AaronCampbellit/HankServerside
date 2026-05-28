# Hank Remote Codex Guide

## Project Intent

`HankServerside` is the server-side project for Hank remote access. Its job is to let the Hank iPhone app work outside the home network without requiring a separate VPN app.

The target architecture is:

- `Hank iPhone App`
- `Hank Remote Cloud`
- `Hank Remote Agent` running inside the home network

The app should talk only to the Hank cloud service over normal HTTPS/WebSocket connections. The home agent should maintain an outbound connection to the cloud and perform local work against Home Assistant and the NAS.

## Repo Boundaries

This repo is only for the remote backend system.

Do:

- build cloud services
- build the home agent
- define and evolve the shared protocol
- add tests for cloud/agent behavior
- add adapters for Home Assistant and file operations

Do not:

- edit the Hank iOS app here
- add direct SMB exposure to the public internet
- design around a VPN requirement
- make the iPhone app speak SMB remotely once a higher-level file API exists

## Current Layout

- `cmd/hank-remote-cloud`: public cloud entrypoint
- `cmd/hank-remote-agent`: home agent entrypoint
- `internal/cloud`: cloud runtime and agent registry
- `internal/agent`: agent runtime and reconnect loop
- `internal/protocol`: shared envelope and message types
- `internal/config`: env-based config loading
- `docs/architecture.md`: architecture notes
- `README.md`: quick start and current scope

## Product Direction

The desired user experience is:

1. The Hank app signs into a Hank account.
2. The user registers a home agent.
3. The home agent connects outbound to Hank Cloud.
4. The app uses Hank Cloud to reach Home Assistant, files, and notes remotely.

This should replace app-side protocol hacks with the cloud-and-agent remote-access path.

## Near-Term Priorities

Work in this order unless the user says otherwise:

1. App auth and cloud-side user/home routing
2. Agent registration persistence and token lifecycle
3. Home Assistant proxy operations
4. File API replacing direct remote SMB from the app
5. Notes sync on top of the same relay
6. Observability, retries, and integration tests

## Technical Principles

- Prefer one stable app-facing API instead of protocol-specific app networking.
- The home agent should own local credentials and local network access.
- The cloud should relay and route, not require raw SMB credentials.
- Never expose SMB directly to the internet.
- Prefer outbound-only home connectivity.
- Keep protocol messages versioned from day one.
- Start with simple JSON over HTTPS/WebSocket unless there is a strong reason to add more complexity.

## Suggested Initial API Shape

When building the next layers, prefer commands at the level of user intent:

- `homeassistant.fetch_states`
- `homeassistant.call_service`
- `files.list`
- `files.download`
- `files.upload`
- `files.create_directory`
- `files.rename`
- `files.delete`
- `notes.sync`

Avoid remote APIs that simply tunnel raw SMB semantics into the phone app unless there is no reasonable abstraction.

## Local Development Commands

Use these first:

```bash
make tidy
make fmt
make build
make run-cloud
make run-agent
```

Equivalent direct commands:

```bash
go mod tidy
gofmt -w ./cmd ./internal
go build ./...
go run ./cmd/hank-remote-cloud
go run ./cmd/hank-remote-agent
```

## Configuration

Cloud:

- `HANK_REMOTE_CLOUD_ADDR`
- `HANK_REMOTE_CLOUD_DATABASE_URL`
- `HANK_REMOTE_DB_OPS_INTENT_SECRET`
- `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`

Agent:

- `HANK_REMOTE_AGENT_CLOUD_URL`
- `HANK_REMOTE_AGENT_ID`
- `HANK_REMOTE_AGENT_TOKEN`
- `HANK_REMOTE_AGENT_HOME_NAME`

Runtime env files now live in the repo root:

- `.env.cloud`
- `.env.agent`

Env examples live in `docs/setup-and-onboarding.md`. The `configs/` folder is for real non-env config assets such as `pgbackrest.conf`.

## Testing Expectations

When making meaningful changes:

- run `gofmt -w ./cmd ./internal`
- run `go build ./...`
- add or update tests when behavior changes

For connection-flow changes, prefer tests for:

- registration
- heartbeat handling
- reconnect behavior
- unauthorized agent rejection
- protocol decoding/encoding

## Coding Expectations

- Keep packages small and explicit.
- Prefer standard library primitives unless a dependency clearly earns its place.
- Add logs around connection lifecycle, routing decisions, and external service calls.
- Keep cloud and agent responsibilities separate.
- Avoid premature persistence layers until the routing/auth model is clear.

## Good Next Tasks For Codex

If a future session needs a concrete starting point, pick one of these:

1. Add cloud-side persistence for registered agents and homes.
2. Add authenticated app WebSocket or HTTP sessions to the cloud service.
3. Add a Home Assistant client package in the agent and expose `fetch_states`.
4. Add protocol request/response correlation for app commands.
5. Add table-driven tests for the cloud register and heartbeat flow.

## Reference Files

Read these first when starting work:

- `README.md`
- `docs/architecture.md`
- `internal/protocol/messages.go`
- `internal/cloud/server.go`
- `internal/agent/client.go`
