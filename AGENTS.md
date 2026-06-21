# Hank Remote Agent Guide

## Project Intent

`HankServerside` is the server-side project for Hank remote access. Its job is to let the Hank iPhone app work outside the home network without requiring a separate VPN app.

The target architecture is:

- `Hank iPhone App`
- `Hank Remote Cloud`
- `Hank Remote Agent` running inside the home network

The app should talk only to the Hank cloud service over normal HTTPS/WebSocket connections. The home agent maintains an outbound connection to the cloud and performs local work against Home Assistant, files, notes, and media sources.

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

## Current System Shape

- The first production target is a single-home self-hosted deployment, not multi-home SaaS or multi-node cloud clustering.
- The cloud serves app/dashboard auth, home management, routing, relay, management UI, readiness, metrics, and storage operations.
- The agent connects outbound to the cloud and owns local Home Assistant, file, notes, and media access.
- PostgreSQL is the durable store for users, homes, agents, sessions, notes, assistant state, tokens, and operational metadata.
- Schema work must use the repo's migration/status/drift-check path instead of hidden startup mutations.
- The dashboard is an operator surface for setup, tokens, settings, storage, assistant controls, and troubleshooting.

## Product Direction

The desired user experience is:

1. The Hank app signs into a Hank account.
2. The user registers a home agent.
3. The home agent connects outbound to Hank Cloud.
4. The app uses Hank Cloud to reach Home Assistant, files, and notes remotely.

This should replace app-side protocol hacks with the cloud-and-agent remote-access path.

## Current Priorities

Use the current task docs and repair plan rather than the old initial-build order.

1. Preserve the single-home cloud-and-agent model.
2. Keep auth, authorization, database safety, and secret handling ahead of feature convenience.
3. Continue the production-readiness work in `docs/backend-production-repair-plan.md`.
4. Strengthen Home Assistant, file, notes, media, assistant, backup, and restore flows with tests.
5. Keep operator setup and deployment docs aligned with real scripts and current behavior.

## Technical Principles

- Prefer one stable app-facing API instead of protocol-specific app networking.
- Treat `HankServerside` like Hank's stable OS/runtime.
- Treat Hank apps as installable first-party extensions for optional workflows, not as replacements for core Hank services.
- Keep the `.hankapp` package format and app runtime APIs as a strict compatibility contract; breaking changes need a new schema version or migration path.
- The home agent should own local credentials and local network access.
- The cloud should relay and route, not require raw SMB credentials.
- Never expose SMB directly to the internet.
- Prefer outbound-only home connectivity.
- Keep protocol messages versioned from day one.
- Start with simple JSON over HTTPS/WebSocket unless there is a strong reason to add more complexity.
- Do not add multi-home, multi-cloud-node, or SaaS assumptions unless the user explicitly changes product scope.

## Agent Change Guardrails

For non-trivial features, bug fixes, cleanup, database work, security changes, or deployment changes, read `docs/agent-change-guardrails.md` before editing and use its security, database, cleanup, and validation checklist before finishing.

## Project Layout

- `cmd/hank-remote-cloud`: public cloud service
- `cmd/hank-remote-agent`: local home agent service
- `cmd/hank-db-ops`: backup, restore-test, and storage operation worker
- `internal/cloud`: auth, routing, relay, dashboard, readiness, metrics, and cloud handlers
- `internal/agent`: reconnect loop, command dispatch, and local capability adapters
- `internal/protocol`: shared wire contract, envelopes, command names, and payloads
- `internal/store`: PostgreSQL persistence
- `internal/migrations`: migration status and checksum checks
- `internal/storageops`: backup/restore operation coordination
- `internal/observability`: metrics aggregation
- `docs/architecture.md`: system design notes
- `docs/hank-app-platform-contract.md`: stable runtime vs installable app boundary and `.hankapp` compatibility rules
- `docs/project-knowledge-index.md`: markdown index used by HankAI

## Local Development Commands

Use these first:

```bash
make tidy
make fmt
make build
make run-cloud
make run-agent
make run-db-ops
```

Equivalent direct commands:

```bash
go mod tidy
gofmt -w ./cmd ./internal
go build ./...
go run ./cmd/hank-remote-cloud
go run ./cmd/hank-remote-agent
go run ./cmd/hank-db-ops
```

Database and deployment checks:

```bash
make migrate-status
make schema-drift-check
scripts/doctor.sh
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

Env examples live in `docs/deployment.md`. The `configs/` folder is for real non-env config assets such as `pgbackrest.conf`.

## Testing Expectations

When making meaningful changes:

- run `gofmt -w ./cmd ./internal`
- run `go build ./...`
- run `go test ./...`
- add or update tests when behavior changes

For connection, auth, storage, or database changes, prefer tests for:

- registration
- heartbeat handling
- reconnect behavior
- unauthorized agent rejection
- protocol decoding/encoding
- cookie, bearer, CSRF, and role authorization behavior
- migration status, schema drift, and store read/write behavior
- file traversal, symlink containment, transfer retry, and destructive operation safety

## Coding Expectations

- Keep packages small and explicit.
- Prefer standard library primitives unless a dependency clearly earns its place.
- Add logs around connection lifecycle, routing decisions, and external service calls.
- Keep cloud and agent responsibilities separate.
- Keep user-facing APIs stable when changing cloud/agent internals.
- Update docs when env vars, setup steps, routes, migrations, or operator workflows change.

## Reference Files

Read these first when starting work:

- `README.md`
- `docs/architecture.md`
- `docs/hank-app-platform-contract.md`
- `docs/agent-change-guardrails.md`
- `docs/backend-production-repair-plan.md`
- `internal/protocol/messages.go`
- `internal/cloud/server.go`
- `internal/agent/client.go`
- `internal/store/store.go`
- `internal/migrations/migrations.go`
