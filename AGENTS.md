# Hank Platform Agent Guide

## Project Authority

`HankServerside` is the primary and canonical project for the Hank ecosystem. It owns the platform contracts, server behavior, shared protocol, persistence model, operator dashboard, core services, and installable app runtime.

Other Hank projects—including mobile clients, desktop clients, standalone agents, and installable apps—are consumers or extensions of this platform. They should conform to HankServerside's stable contracts. When a client conflicts with an established platform contract, fix the client in its owning repository unless the product direction explicitly requires a platform change.

## Project Intent

HankServerside is Hank's stable platform and runtime. It provides the cloud service, outbound-connected home agent, dashboard, shared protocol, durable storage, core Home Assistant/file/notes/media/assistant services, and the runtime for optional `.hankapp` extensions.

Existing `Hank Remote`, `hank-remote-*`, and `HANK_REMOTE_*` names remain current technical identifiers.

## Repo Boundaries

This repository owns the shared Hank platform.

Do:

- build and operate the cloud services, home agent, dashboard, persistence layer, and shared protocol
- define stable app-facing and agent-facing contracts
- provide core Home Assistant, files, notes, media, assistant, backup, and restore capabilities
- maintain the generic runtime and compatibility contract for installable Hank apps
- add tests and operational tooling for platform behavior

Do not:

- implement client-specific behavior that belongs in an iOS, macOS, Windows, or other consumer repository
- weaken a stable platform contract to preserve a client-side workaround
- expose SMB, Home Assistant, local files, or other raw local protocols directly to the public internet
- design around a VPN requirement
- let optional Hank apps replace core platform services

## Product Direction

The intended system flow is:

1. A Hank client signs into the Hank platform.
2. The user registers a home agent.
3. The home agent connects outbound to HankServerside.
4. Clients use HankServerside's stable HTTPS/WebSocket APIs to access core services.
5. Optional Hank apps extend the platform through the versioned app-runtime contract.

Clients should not independently recreate networking, persistence, authorization, or local-service protocols already owned by HankServerside.

## Current System Shape

- HankServerside is the source of truth for shared product behavior and compatibility contracts.
- The first production target remains a single-home self-hosted deployment, not multi-home SaaS or multi-node clustering.
- The cloud owns authentication, authorization, routing, relay, management APIs, the dashboard, readiness, metrics, storage operations, and the installable-app runtime.
- The home agent connects outbound and owns access to local credentials, Home Assistant, files, notes, media, and other home-network resources.
- PostgreSQL is the durable store for users, homes, agents, sessions, notes, assistant state, apps, tokens, and operational metadata.
- Schema changes use the versioned migration, status, and drift-check workflow.
- Other Hank clients and services consume these platform capabilities through stable contracts.

## Current Priorities

1. Treat HankServerside as the primary project and land shared behavior here first.
2. Preserve the single-home cloud-and-agent architecture.
3. Keep authentication, authorization, database safety, file safety, and secret handling ahead of convenience.
4. Protect the shared API, protocol, and `.hankapp` compatibility contracts.
5. Complete release-readiness evidence tracked in `RELEASE.md`.
6. Strengthen core services and operator workflows with focused tests.
7. Keep setup, deployment, API, and compatibility documentation aligned with actual behavior.

## Technical Principles

- Design shared capabilities in HankServerside first; keep clients thin and contract-driven.
- Prefer one stable app-facing API over client-specific networking or protocol logic.
- Treat HankServerside as Hank's stable OS/runtime.
- Treat Hank apps as optional first-party extensions, not replacements for core services.
- Keep API, protocol, and `.hankapp` compatibility surfaces versioned; breaking changes require a new version or documented migration path.
- Keep cloud, home-agent, dashboard, protocol, persistence, and installable-app responsibilities explicit.
- Keep local credentials and local-network access inside the home agent.
- Prefer outbound-only home connectivity and never expose raw SMB or other local protocols publicly.
- Continue using the established JSON-over-HTTPS/WebSocket protocol unless a measured requirement justifies changing it.
- Do not introduce multi-home, multi-cloud-node, or SaaS assumptions unless the user explicitly changes product scope.

## Working Safely

### Plan and Milestone Completion

- When the user asks to execute a written plan or milestone, complete every task and checklist item in the active milestone—or the entire plan when it has no milestone boundary—before stopping or sending a final handoff.
- Do not treat a passing partial batch, an interim verification checkpoint, or progress on only some tasks as completion. Use commentary updates for progress while continuing the work.
- Do not suggest, begin, or ask the user to approve the next milestone while any task in the current milestone remains incomplete.
- Continue autonomously through the full active milestone, including across tool calls and context compaction. A status question from the user does not end the work unless the user explicitly asks to pause or change scope.
- Stop early only for a clear blocker that requires human review, a user decision, new authority, unavailable required access, or an external state change. Exhaust safe in-scope alternatives first, complete any independent remaining tasks, and report the exact blocked checklist item and required human action.
- A skipped validation that can be reported honestly is not by itself permission to leave the milestone's implementation tasks unfinished.

Before editing:

- inspect the current branch, worktree status, and relevant diff
- preserve unrelated or uncommitted user work
- identify the owning layer before changing a shared contract
- read `docs/agent-change-guardrails.md` for non-trivial code, database, security, cleanup, or deployment changes
- use the repository migration workflow for schema changes; never hide schema mutation in startup code
- do not modify another Hank repository unless the user explicitly includes it in scope
- do not commit, push, tag, publish, or deploy unless the user explicitly asks

## Project Layout

- `cmd/`: cloud, home-agent, and database-operations entry points
- `internal/cloud`: HTTP/WebSocket APIs, auth, routing, relay, dashboard serving, and app runtime
- `internal/agent`: home-agent connection lifecycle and local capability adapters
- `internal/protocol`: shared versioned cloud/agent wire contract
- `internal/store`: PostgreSQL persistence
- `internal/migrations`: versioned schema and migration checks
- `internal/domain`, `internal/config`, and `internal/maintenance`: shared platform models, configuration, and lifecycle work
- `internal/storageops` and `internal/observability`: backup/restore coordination and metrics
- `web/dashboard`: React/Vite/TypeScript operator dashboard
- `schemas`: versioned external compatibility schemas
- `scripts`, `tools`, and `ops`: setup, validation, administration, release, and monitoring tooling
- `docs`: architecture, contracts, deployment guidance, runbooks, and plans

## Development Commands

Whole-platform checks:

```bash
make tidy
make fmt
go test ./...
make build
```

Frontend-specific checks:

```bash
make frontend-test
make frontend-check
make frontend-build
```

Local services:

```bash
make run-cloud
make run-agent
make run-db-ops
```

Database and operator checks:

```bash
make migrate-status
make schema-drift-check
scripts/doctor.sh
```

Use the smallest relevant checks during development. Use the complete gate in `RELEASE.md` for release work.

## Configuration

Runtime environment files are `.env.cloud` and `.env.agent` in the repository root. Treat them as sensitive and never commit, print, or copy their secret values into logs or documentation.

Use `docs/deployment.md` as the source of truth for supported environment variables and setup. Keep it synchronized when configuration behavior changes.

## Validation Expectations

Match validation to the changed surface, then broaden it when risk justifies it.

- Go or protocol changes: run `make fmt`, targeted tests, `go test ./...`, and `make build`
- Dashboard changes: run targeted frontend tests, `make frontend-test`, `make frontend-check`, and `make frontend-build`
- Database changes: run relevant store tests, `make migrate-status`, and `make schema-drift-check`
- Auth, routing, files, storage, agent-command, or secret-handling changes: add focused security and failure-path coverage
- Deployment or release changes: use `scripts/doctor.sh` and the applicable `RELEASE.md` gate
- Documentation-only changes: verify referenced paths, commands, links, and `git diff --check`

PostgreSQL-backed tests that skip without `HANK_REMOTE_TEST_DATABASE_URL` do not count as full database validation.

Before reporting completion, state:

- security impact
- database or migration impact
- validation performed and anything skipped

## Coding Expectations

- Put shared product behavior in the platform layer that owns it.
- Keep packages and interfaces small, explicit, and testable.
- Prefer standard-library primitives unless a dependency clearly earns its place.
- Preserve stable client-facing APIs when changing internal implementations.
- Add useful lifecycle, routing, and external-call logs without logging secrets or private file contents.
- Update tests and documentation when behavior, configuration, routes, schemas, setup, or operator workflows change.
- Avoid duplicating platform behavior in the dashboard, agent, or consuming clients.

## Reference Routing

Read the files relevant to the task instead of loading every reference for every change.

- Platform architecture: `README.md` and `docs/architecture.md`
- Security, database, cleanup, or deployment changes: `docs/agent-change-guardrails.md`
- App-runtime or package compatibility: `docs/hank-app-platform-contract.md`
- Release work: `RELEASE.md` and `docs/demo-validation.md`
- Protocol work: `internal/protocol/messages.go`
- Cloud/API work: `internal/cloud/server.go` and the owning handler
- Agent work: `internal/agent/client.go` and the owning adapter
- Persistence work: `internal/store` and `internal/migrations`
- Dashboard work: `web/dashboard` plus the corresponding server API
