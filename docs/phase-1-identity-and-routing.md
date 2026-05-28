# Phase 1: Identity And Routing

## Goal

Turn the current scaffold into a real relay foundation with authenticated agents, authenticated app sessions, and cloud-side routing between users, homes, and connected agents.

## Why This Phase Comes First

Everything else depends on it. Until the cloud can identify:

- which user is calling
- which home they are trying to reach
- which agent is online for that home

the remote feature cannot safely proxy anything.

## Scope

Build:

- app authentication in the cloud service
- cloud-side home and agent registration model
- persistent agent records
- token lifecycle for agents
- request/response correlation for app commands
- routing from app session to target agent

Do not build yet:

- Home Assistant logic
- file or SMB adapters
- notes sync behavior

## Suggested Data Model

At minimum, define these concepts:

- `User`
- `Home`
- `Agent`
- `AgentToken`
- `AppSession`

Use PostgreSQL from the start and keep the persistence layer thin enough to evolve without rewriting cloud routing.

## Suggested API Surface

Cloud HTTP:

- `POST /v1/auth/register`
- `POST /v1/auth/login`
- `POST /v1/auth/logout`
- `GET /v1/me`
- `GET /v1/home`
- `PUT /v1/home`
- `GET /v1/home/agent`
- `GET /v1/home/agent/tokens`
- `POST /v1/home/agent/tokens`
- `DELETE /v1/home/agent/tokens/{tokenID}`
- `POST /v1/ws/app-ticket`

Cloud WebSocket:

- `GET /ws/app`
- `GET /ws/agent`

## Protocol Work

Extend the protocol envelope to support:

- `request_id`
- app commands without requiring `home_id`; the cloud resolves the singleton Home from the authenticated user
- command routing
- typed error responses
- app-side command and response flow

## Deliverables

- persistent cloud storage for homes and agents
- agent registration tied to a home
- revocable agent token model
- app session authentication
- an app-to-agent relay path that can carry a simple ping command

## Exit Criteria

Phase 1 is complete when:

- an authenticated app session can target a specific home
- the cloud can determine whether that home’s agent is online
- the cloud can send a simple request to the agent and receive a response
- unauthorized agents are rejected
- revoked agent tokens stop working

## Testing Expectations

Add tests for:

- agent token validation
- revoked token rejection
- app auth guardrails
- route lookup by home
- app request to agent round-trip
- offline-agent error responses

## Recommended First Tasks

1. Add storage interfaces and a PostgreSQL-backed implementation.
2. Add app auth middleware.
3. Add persistent `Home` and `Agent` registration.
4. Add request correlation in the protocol.
5. Add end-to-end tests for online and offline routing.
