# Hank Remote Project Knowledge Index

This file is the central map for project markdown that HankAI should use when answering questions about Hank Remote.

## Root Documents

- [README.md](../README.md): current scope, route summary, project layout, and quick start.
- [AGENTS.md](../AGENTS.md): Codex working rules and repo boundaries.
- [SERVER_SYNC.md](../SERVER_SYNC.md): app/server contract ledger.

These documents stay in the repo root because common tooling expects them there. They are still part of the HankAI project-docs source.

## Architecture And Setup

- [architecture.md](architecture.md)
- [deployment.md](deployment.md)
- [roadmap.md](roadmap.md)
- [backend-production-repair-plan.md](backend-production-repair-plan.md)
- [backend-architecture-audit.md](backend-architecture-audit.md): dated 2026-05-30 point-in-time audit; use the repair plan, deployment guide, and runbooks for current operator actions.
- [legacy-code-audit.md](legacy-code-audit.md): dated 2026-06-01 cleanup audit with implementation resolution status.

## Historical Phase Archive

The phase-era documents are archived under [archive/phases](archive/phases). They are retained as historical implementation context only and should not be treated as current setup, operator, or repair guidance.

- [archive/phases/phase-1-identity-and-routing.md](archive/phases/phase-1-identity-and-routing.md)
- [archive/phases/phase-1-tasklist.md](archive/phases/phase-1-tasklist.md)
- [archive/phases/phase-2-home-assistant.md](archive/phases/phase-2-home-assistant.md)
- [archive/phases/phase-2-tasklist.md](archive/phases/phase-2-tasklist.md)
- [archive/phases/phase-3-files.md](archive/phases/phase-3-files.md)
- [archive/phases/phase-3-tasklist.md](archive/phases/phase-3-tasklist.md)
- [archive/phases/phase-4-notes.md](archive/phases/phase-4-notes.md)
- [archive/phases/phase-4-tasklist.md](archive/phases/phase-4-tasklist.md)
- [archive/phases/phase-5-operations.md](archive/phases/phase-5-operations.md)
- [archive/phases/phase-5-tasklist.md](archive/phases/phase-5-tasklist.md)
- [archive/phases/phase-6-hank-assistant.md](archive/phases/phase-6-hank-assistant.md)

## Feature Notes

- [hankai-chat-tool-improvement-plan.md](hankai-chat-tool-improvement-plan.md)
- [hankai-intents-rollout.md](hankai-intents-rollout.md)
- [hermes-chat-workflow.md](hermes-chat-workflow.md)

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

Use `docs/hankai-vector-index.md` as the current inventory of HankAI vector-index sources, provider embedding behavior, and retrieval boundaries.
