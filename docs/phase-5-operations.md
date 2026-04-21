# Phase 5: Operations, Security, And Production Readiness

## Goal

Make Hank Remote deployable, observable, and maintainable as a real service.

## Scope

Build:

- persistent production-grade storage choices
- token rotation and secret management
- metrics, structured logs, and tracing hooks
- rate limiting and abuse controls
- retry and backpressure strategy
- deployment guidance
- backups and recovery guidance

## Security Focus

Harden:

- app auth
- agent auth
- replay protection where needed
- secret storage
- access control by user and home
- audit logging for sensitive operations

## Operational Focus

Add:

- health checks
- readiness checks
- alertable metrics
- connection counts
- request latency metrics
- upstream failure metrics
- structured logs with request and home identifiers

## Deployment Targets

At minimum, document and support:

- local development
- single-node cloud deployment
- home agent deployment on a small always-on device

Later, optionally support:

- containerized deployment
- managed database
- multiple cloud instances

## Deliverables

- production configuration model
- observability package and dashboards
- deployment docs
- security review checklist
- backup and recovery notes

## Exit Criteria

Phase 5 is complete when:

- the service can be deployed and monitored predictably
- auth and token flows are rotatable and auditable
- core endpoints and relay paths are observable
- the system has documented operational runbooks

## Testing Expectations

Add tests or checks for:

- config validation
- token rotation paths
- rate limiting behavior
- migration or persistence startup behavior
- degraded upstream handling

## Recommended First Tasks

1. Add structured request IDs and correlation IDs everywhere.
2. Add metrics around cloud and agent connections.
3. Add token rotation support.
4. Write deployment docs for one cloud node plus one home agent.
5. Add runbooks for agent offline, upstream timeout, and auth failures.
