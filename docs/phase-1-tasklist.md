# Phase 1 Tasklist

This tasklist turns Phase 1 into concrete implementation work for Codex.

## Phase 1 Objective

Build the first real version of identity and routing so:

- authenticated app clients can connect to Hank Cloud
- home agents can register against a specific home
- the cloud can route an app command to the correct online agent
- the cloud can return a response back to the app

## Definition Of Done

Phase 1 is done when all of these are true:

1. The cloud stores users, homes, agents, and agent tokens persistently.
2. The app can authenticate and obtain a session token.
3. The app can open a cloud connection tied to a user session.
4. The cloud can route a command to a connected agent by `home_id`.
5. The agent can return a correlated response to the app.
6. Offline-agent, unauthorized-agent, and revoked-token cases are covered.

## Recommended Implementation Order

1. Persistence layer
2. Domain models
3. Agent token lifecycle
4. App auth
5. App WebSocket session
6. Command routing
7. End-to-end tests

## Task Group 1: Persistence And Domain Models

### Create Packages

Add:

- `internal/store`
- `internal/domain`

### Add Domain Models

Define at minimum:

- `User`
- `Home`
- `Agent`
- `AgentToken`
- `AppSession`

Suggested fields:

#### User

- `ID`
- `Email`
- `PasswordHash`
- `CreatedAt`
- `UpdatedAt`

#### Home

- `ID`
- `UserID`
- `Name`
- `CreatedAt`
- `UpdatedAt`

#### Agent

- `ID`
- `HomeID`
- `Name`
- `Status`
- `LastSeenAt`
- `CreatedAt`
- `UpdatedAt`

#### AgentToken

- `ID`
- `HomeID`
- `AgentID` optional if you want per-agent binding
- `TokenHash`
- `RevokedAt`
- `ExpiresAt`
- `CreatedAt`

#### AppSession

- `ID`
- `UserID`
- `TokenHash`
- `ExpiresAt`
- `CreatedAt`

### Storage Direction

Use PostgreSQL first.

Keep the interface narrow enough that storage can be swapped later.

Suggested interfaces:

- `UserStore`
- `HomeStore`
- `AgentStore`
- `AgentTokenStore`
- `SessionStore`

### Concrete Tasks

- create schema migrations
- add store interfaces
- add PostgreSQL implementations
- add startup migration hook in cloud main

## Task Group 2: Agent Token Lifecycle

### Build Cloud-Side Token Management

Add support for:

- issuing an agent token for a home
- hashing token values before persistence
- revoking a token
- validating a presented token from the WebSocket agent connection

### Required Endpoints

- `POST /v1/homes/{homeID}/agents/tokens`
- `DELETE /v1/homes/{homeID}/agents/tokens/{tokenID}`

### Rules

- never store raw agent tokens
- return the raw token only once on creation
- rejected or revoked tokens must not allow WebSocket registration

## Task Group 3: App Authentication

### Build A Minimal Auth Model

Start simple:

- email + password login
- signed or random session token

Suggested endpoints:

- `POST /v1/auth/register`
- `POST /v1/auth/login`
- `POST /v1/auth/logout`
- `GET /v1/me`

### Implementation Notes

- password hashing: use a strong password hash
- session tokens: opaque random tokens are fine for the first pass
- store only token hashes if practical

### Middleware

Add auth middleware for app-only routes.

Protected routes should require a valid app session.

## Task Group 4: Home Registration And Ownership

### Build Home Management

Add endpoints:

- `POST /v1/homes`
- `GET /v1/homes`
- `GET /v1/homes/{homeID}`

Rules:

- a user can only see their own homes
- a user can only issue agent tokens for homes they own

### Suggested Data Validation

- home name required
- home must belong to authenticated user

## Task Group 5: Protocol And Routing

### Extend The Shared Protocol

Update `internal/protocol/messages.go` to support:

- `home_id` on routed messages
- `request_id` everywhere routing matters
- command and response types for app-to-agent traffic
- standard error envelopes for cloud routing failures

Suggested message types:

- `app.command`
- `app.response`
- `app.error`
- `cloud.command`
- `cloud.response`

### Required Behavior

- an app command must target a specific `home_id`
- cloud resolves that `home_id` to an active agent connection
- cloud forwards the request to the active agent
- agent returns a response with the same `request_id`
- cloud sends the response back to the original app connection

### First Command To Support

Implement a simple health or ping command:

- `system.ping`

Example flow:

1. app sends `system.ping`
2. cloud routes it to the agent for the requested home
3. agent replies with `pong`
4. cloud returns `pong` to the app

## Task Group 6: App WebSocket Session

### Add App Connection Endpoint

Create:

- `GET /ws/app`

Requirements:

- requires valid app auth
- associates the connection with a user
- supports sending commands with `home_id`
- receives routed responses from the cloud

### Connection Tracking

Cloud should maintain:

- online app connections by session or user
- online agent connections by home
- pending request routing by `request_id`

Suggested package:

- `internal/router`

## Task Group 7: Agent Registration Upgrade

Update the current agent register flow so registration is tied to stored cloud records.

### Required Behavior

- token validation resolves the home and allowed agent identity
- register message confirms or updates agent metadata
- cloud marks the agent online
- disconnect marks it offline or stale

### Agent Data To Capture

- home name
- agent version
- capabilities
- last seen

## Task Group 8: Tests

### Add Unit Tests

Suggested files:

- `internal/config/config_test.go`
- `internal/protocol/messages_test.go`
- `internal/store/...`
- `internal/cloud/server_test.go`

### Add Integration Tests

Required scenarios:

1. register user
2. login user
3. create home
4. issue agent token
5. connect agent with valid token
6. connect app with valid session
7. app sends `system.ping`
8. agent returns response
9. app receives correlated response

Failure scenarios:

1. invalid agent token
2. revoked token
3. user tries to access another user’s home
4. app targets a home with no online agent

## Task Group 9: Logging And Errors

### Add Structured Logs For

- app login
- home creation
- token issuance
- agent connect and disconnect
- app connect and disconnect
- route success and route failure

### Standard Cloud Errors

Define stable error codes such as:

- `unauthorized`
- `forbidden`
- `home_not_found`
- `agent_offline`
- `request_timeout`
- `invalid_request`

## Suggested File Additions

These are likely new files for the first real pass:

- `internal/domain/models.go`
- `internal/store/interfaces.go`
- `internal/store/*.go`
- `internal/cloud/auth.go`
- `internal/cloud/app_ws.go`
- `internal/cloud/agent_ws.go`
- `internal/cloud/http_homes.go`
- `internal/cloud/http_auth.go`
- `internal/cloud/http_agent_tokens.go`
- `internal/router/router.go`
- `internal/router/pending_requests.go`

## Suggested Codex Prompt For This Phase

Use something like:

> Implement Phase 1 from `docs/phase-1-identity-and-routing.md` and `docs/phase-1-tasklist.md`. Start with PostgreSQL-backed persistence, app auth, home creation, agent token issuance, `/ws/app`, and app-to-agent `system.ping` routing. Add tests for valid routing, offline agents, revoked agent tokens, and cross-user access rejection. Run formatting and `go build ./...` before finishing.

## Final Check Before Calling Phase 1 Complete

Make sure all of the following are demonstrated:

- persistent home and agent data
- valid app login
- valid agent registration
- correlated request/response routing
- offline-agent error behavior
- revoked-token rejection
- ownership enforcement across homes
