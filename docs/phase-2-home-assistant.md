# Phase 2: Home Assistant Remote Access

## Goal

Make the Hank app remotely usable for dashboard functionality by proxying Home Assistant through the Hank Remote system.

## Why This Phase Comes Second

It gives the fastest user-visible result after identity and routing are in place. It is also simpler than remote file work.

## Scope

Build:

- agent-side Home Assistant client
- cloud relaying for Home Assistant commands
- app-facing endpoints for dashboard reads and service calls
- optional event subscription path for updates

Do not build yet:

- remote file operations
- note synchronization

## Required Agent Capabilities

The home agent should support:

- reading Home Assistant state
- invoking Home Assistant service calls
- health checking the local Home Assistant endpoint

The agent should own the local Home Assistant URL and token. The cloud should not need those secrets.

## Suggested Command Set

- `homeassistant.fetch_states`
- `homeassistant.fetch_state`
- `homeassistant.call_service`
- `homeassistant.health`

If live updates are needed early:

- `homeassistant.subscribe_events`

## Deliverables

- Home Assistant client package in the agent
- app-facing command contract for dashboard reads
- cloud relay support for Home Assistant commands
- error translation from Home Assistant failures into stable app-facing errors

## Exit Criteria

Phase 2 is complete when:

- the app can fetch the same dashboard state remotely through Hank Cloud
- service calls can be triggered remotely
- local Home Assistant credentials remain private to the agent
- failures return stable, user-readable errors

## Testing Expectations

Add tests for:

- successful state fetch
- service call forwarding
- unauthorized app access
- offline agent behavior
- Home Assistant upstream timeout handling
- malformed Home Assistant response handling

## Recommended First Tasks

1. Add agent-side Home Assistant config model.
2. Build a small Home Assistant HTTP client package.
3. Add protocol message types for Home Assistant commands.
4. Add cloud routing for those commands.
5. Add end-to-end tests using a stub Home Assistant server.
