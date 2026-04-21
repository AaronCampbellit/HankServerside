# Architecture

## Goal

Allow Hank to work off the home network without requiring a separate VPN app.

## High-Level Design

```text
Hank iPhone App
    |
    | HTTPS / WebSocket
    v
Hank Remote Cloud
    |
    | persistent outbound WebSocket
    v
Hank Remote Agent (inside home network)
    |                |
    | local HTTP     | local SMB / filesystem adapters
    v                v
Home Assistant     NAS / TrueNAS
```

## Why This Direction

- The iPhone app gets one remote API surface.
- The home network needs only an outbound connection from the agent.
- SMB and Home Assistant stay private to the home LAN.
- We avoid protocol-specific networking tricks in the iOS app.

## Planned Boundaries

### Cloud

- authenticate app clients
- authenticate agents
- maintain routing from users/homes to active agents
- relay commands, streams, and events

### Agent

- connect outbound to cloud
- expose local capabilities
- translate cloud commands into local Home Assistant and filesystem operations
- enforce local policy and capability checks

### Protocol

- versioned envelope
- request/response correlation
- command and event schemas
- error format

## Near-Term Feature Plan

1. `homeassistant.fetch_states`
2. `homeassistant.call_service`
3. `files.list`
4. `files.download`
5. `files.upload`
6. `notes.sync`

## Security Notes

- Never expose SMB directly to the internet.
- The agent should only connect outbound to cloud.
- Agent registration tokens should be rotated and revocable.
- App auth should be separate from agent auth.
- Cloud should not need raw SMB credentials if the agent owns local access.
