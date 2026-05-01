# Phase 2 Tasklist

This tasklist turns the Home Assistant phase into concrete implementation work.

## Phase 2 Objective

Let the Hank app remotely fetch dashboard data and call Home Assistant services through Hank Cloud and the home agent.

## Definition Of Done

Phase 2 is done when all of these are true:

1. The home agent can talk to the local Home Assistant instance.
2. The cloud can route Home Assistant commands to the correct online agent.
3. The app can fetch Home Assistant states remotely.
4. The app can invoke Home Assistant service calls remotely.
5. Local Home Assistant secrets stay on the agent.

## Recommended Implementation Order

1. Agent config for Home Assistant
2. Agent-side Home Assistant client
3. Protocol messages for Home Assistant commands
4. Cloud routing for those commands
5. End-to-end tests with a stub Home Assistant server

## Task Group 1: Agent Configuration

### Add Config Support

Extend agent config with:

- `HANK_REMOTE_HA_BASE_URL`
- `HANK_REMOTE_HA_TOKEN`
- `HANK_REMOTE_HA_TIMEOUT_SECONDS`

Add env examples and validation.

### Suggested Files

- `internal/config/config.go`
- `docs/setup-and-onboarding.md`

## Task Group 2: Home Assistant Client

### Create Package

Add:

- `internal/agent/homeassistant`

### Implement Capabilities

Build:

- fetch all states
- fetch one state
- call service
- health check

### Suggested Interface

```go
type Client interface {
    Health(ctx context.Context) error
    FetchStates(ctx context.Context) ([]State, error)
    FetchState(ctx context.Context, entityID string) (State, error)
    CallService(ctx context.Context, domain string, service string, body json.RawMessage) error
}
```

### Rules

- use agent-local secrets only
- add request timeout handling
- map Home Assistant failures into stable internal errors

## Task Group 3: Protocol Messages

### Extend Shared Protocol

Add commands:

- `homeassistant.health`
- `homeassistant.fetch_states`
- `homeassistant.fetch_state`
- `homeassistant.call_service`

Add typed payloads and typed responses for each.

### Suggested Files

- `internal/protocol/messages.go`
- `internal/protocol/homeassistant.go`

## Task Group 4: Agent Command Handling

### Add Command Dispatcher

Create or extend:

- `internal/agent/commands.go`

Handle:

- route command by type
- invoke Home Assistant client
- build typed protocol responses
- return stable error envelopes

### First Commands To Implement

1. `homeassistant.health`
2. `homeassistant.fetch_states`
3. `homeassistant.call_service`

## Task Group 5: Cloud Routing

### Extend Cloud Router

Ensure the cloud can:

- accept Home Assistant commands from the app
- route them to the correct agent
- correlate the response by `request_id`
- return a typed error if the agent is offline or the request times out

### Required Error Cases

- `agent_offline`
- `request_timeout`
- `unsupported_command`
- `upstream_error`

## Task Group 6: App-Facing HTTP Or WebSocket Contract

Choose one temporary app-facing path for early delivery:

- WebSocket command messages over `/ws/app`
- or HTTP endpoints that internally relay to the agent

Prefer reusing `/ws/app` if Phase 1 already built request routing there.

### Suggested Commands

- request all states
- request one state
- call service

## Task Group 7: Testing

### Unit Tests

Add tests for:

- Home Assistant response decoding
- Home Assistant timeout handling
- protocol payload encode/decode
- agent command dispatch

### Integration Tests

Required scenarios:

1. app requests all states
2. cloud routes command to the correct agent
3. agent fetches from stub Home Assistant
4. app receives response
5. app calls a Home Assistant service successfully

Failure scenarios:

1. Home Assistant upstream timeout
2. malformed Home Assistant response
3. offline agent
4. unauthorized app session

## Task Group 8: Logging And Errors

Add structured logs for:

- Home Assistant request start
- Home Assistant request success
- Home Assistant request failure
- cloud relay start and finish

Keep errors stable and app-facing. Do not leak raw secrets or tokens.

## Suggested File Additions

- `internal/agent/homeassistant/client.go`
- `internal/agent/homeassistant/types.go`
- `internal/agent/commands_homeassistant.go`
- `internal/protocol/homeassistant.go`
- `internal/cloud/relay_homeassistant.go`
- `internal/agent/homeassistant/client_test.go`

## Suggested Codex Prompt For This Phase

> Implement Phase 2 from `docs/phase-2-home-assistant.md` and `docs/phase-2-tasklist.md`. Add agent-side Home Assistant config and client support, protocol messages for Home Assistant commands, agent command dispatch, and cloud routing for `homeassistant.health`, `homeassistant.fetch_states`, and `homeassistant.call_service`. Add tests using a stub Home Assistant server. Run formatting and `go build ./...` before finishing.
