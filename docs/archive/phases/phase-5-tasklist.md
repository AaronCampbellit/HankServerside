# Phase 5 Tasklist

This tasklist turns operations and production readiness into concrete implementation work.

## Phase 5 Objective

Make Hank Remote secure, observable, and deployable as a real service.

## Definition Of Done

Phase 5 is done when all of these are true:

1. The cloud and agent expose useful health and readiness signals.
2. Authentication and token flows can be rotated and audited.
3. Core relay paths have logs and metrics.
4. Deployment guidance and runbooks exist.
5. The service has basic abuse and failure controls.

## Recommended Implementation Order

1. structured request IDs and tracing hooks
2. metrics and health signals
3. token rotation and secret handling
4. rate limits and backpressure
5. deployment docs and runbooks

## Task Group 1: Request Correlation And Logging

### Add Correlation IDs

Every important request path should carry:

- request ID
- user ID when applicable
- home ID when applicable
- agent ID when applicable

### Logging Areas

Add structured logs for:

- auth
- home lookup
- agent connect and disconnect
- command route start and finish
- upstream error
- timeout

## Task Group 2: Metrics

### Add Metrics Package

Create:

- `internal/observability`

Track at minimum:

- online agents
- online app sessions
- command counts by type
- command latency
- route failures
- upstream Home Assistant failures
- file transfer counts and failures

### Exposure

Add:

- `GET /metrics` if using a pull model

## Task Group 3: Health And Readiness

### Add Signals

Cloud:

- `GET /healthz`
- `GET /readyz`

Agent:

- internal readiness state
- optional local debug endpoint if helpful

### Readiness Checks

Cloud should verify:

- storage available
- critical dependencies ready

Agent should verify:

- cloud connection state
- critical local adapter configuration state

## Task Group 4: Token Rotation And Secret Handling

### Agent Tokens

Add support for:

- issuing replacement tokens
- overlapping validity during rotation
- revoking old tokens

### App Sessions

Add:

- explicit expiration policy
- revocation or logout handling

### Secret Storage Rules

- avoid logging secrets
- avoid storing raw tokens where possible
- document secret handling expectations

## Task Group 5: Abuse Controls

### Add Protection For

- repeated failed login attempts
- repeated invalid agent connection attempts
- oversized command payloads
- too many in-flight requests per connection

### Suggested Controls

- rate limiting
- payload size caps
- connection caps
- request timeout caps

## Task Group 6: Deployment Support

### Add Docs

Write:

- local development setup
- single-node production deployment
- agent deployment on an always-on home machine
- required environment variables
- reverse proxy or TLS guidance

### Optional Packaging

Consider:

- Dockerfiles
- sample systemd units
- sample container configs

## Task Group 7: Runbooks

Write runbooks for:

- agent offline
- auth failures
- upstream Home Assistant failure
- file transfer failure
- storage failure
- token rotation

## Task Group 8: Testing And Verification

### Add Tests Or Checks For

- config validation
- metrics endpoint availability
- readiness behavior under degraded conditions
- token rotation flow
- rate limiting behavior
- request timeout enforcement

### Add Smoke Checklist

Create a short operator checklist for:

- starting cloud
- connecting one agent
- authenticating one app
- issuing one test command
- verifying metrics and logs

## Suggested File Additions

- `internal/observability/metrics.go`
- `internal/observability/logging.go`
- `internal/cloud/http_metrics.go`
- `internal/cloud/http_readyz.go`
- `docs/deployment.md`
- `docs/runbooks/*.md`

## Suggested Codex Prompt For This Phase

> Implement Phase 5 from `docs/phase-5-operations.md` and `docs/phase-5-tasklist.md`. Add request correlation, metrics, readiness checks, token rotation support, and practical abuse controls. Document deployment and runbooks for common failures. Run formatting and `go build ./...` before finishing.
