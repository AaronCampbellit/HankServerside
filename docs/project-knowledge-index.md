# Hank Remote Project Knowledge Index

This file is the central map for project markdown that HankAI should use when answering questions about Hank Remote.

## Root Documents

- [README.md](../README.md): current scope, route summary, project layout, and quick start.
- [AGENTS.md](../AGENTS.md): Codex working rules and repo boundaries.
- [SERVER_SYNC.md](../SERVER_SYNC.md): app/server contract ledger.

These documents stay in the repo root because common tooling expects them there. They are still part of the HankAI project-docs source.

## Architecture And Setup

- [architecture.md](architecture.md)
- [setup-and-onboarding.md](setup-and-onboarding.md)
- [deployment.md](deployment.md)
- [first-time-deployment.md](first-time-deployment.md)
- [roadmap.md](roadmap.md)

## Feature Phases

- [phase-1-identity-and-routing.md](phase-1-identity-and-routing.md)
- [phase-1-tasklist.md](phase-1-tasklist.md)
- [phase-2-home-assistant.md](phase-2-home-assistant.md)
- [phase-2-tasklist.md](phase-2-tasklist.md)
- [phase-3-files.md](phase-3-files.md)
- [phase-3-tasklist.md](phase-3-tasklist.md)
- [phase-4-notes.md](phase-4-notes.md)
- [phase-4-tasklist.md](phase-4-tasklist.md)
- [phase-5-operations.md](phase-5-operations.md)
- [phase-5-tasklist.md](phase-5-tasklist.md)
- [phase-6-hank-assistant.md](phase-6-hank-assistant.md)
- [hankai-intents-rollout.md](hankai-intents-rollout.md)

## App Contract Notes

- [hank-app-auth-migration.md](hank-app-auth-migration.md)
- [hank-app-home-sync-checklist.md](hank-app-home-sync-checklist.md)

## Runbooks

- [runbooks/agent-offline.md](runbooks/agent-offline.md)
- [runbooks/auth-failures.md](runbooks/auth-failures.md)
- [runbooks/file-transfer-failures.md](runbooks/file-transfer-failures.md)
- [runbooks/home-assistant-failures.md](runbooks/home-assistant-failures.md)
- [runbooks/single-host-compose.md](runbooks/single-host-compose.md)
- [runbooks/storage-failures.md](runbooks/storage-failures.md)
- [runbooks/token-rotation.md](runbooks/token-rotation.md)

## HankAI Indexing

HankAI indexes root markdown files and every markdown file under `docs/` as the `Project docs` source. The cloud Docker image copies those files into `/app`, and local development defaults to the repo root. Override with `HANK_REMOTE_PROJECT_DOCS_DIR` if the markdown lives somewhere else.
