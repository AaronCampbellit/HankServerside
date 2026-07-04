# Hank Remote Project Knowledge Index

This file is the central map for project markdown. It is the one place to start
when answering questions about Hank Remote or deciding where a new doc belongs.

Layout at a glance:

- **Active reference docs** live flat in `docs/`.
- **Operator runbooks** live in `docs/runbooks/`.
- **Hank app (client) integration** docs live in `docs/app-integration/`.
- Completed or superseded plans, audits, designs, and tasklists are removed from
  the active docs tree once their durable guidance has been folded into the
  files below.

## Root Documents

- [README.md](../README.md): current scope, route summary, project layout, and quick start.
- [AGENTS.md](../AGENTS.md): Codex working rules and repo boundaries.
- [SERVER_SYNC.md](../SERVER_SYNC.md): app/server contract ledger.

These stay in the repo root because common tooling expects them there. They are
still part of the HankAI project-docs source.

## Architecture, Setup, and Operations

- [architecture.md](architecture.md): current system design and surface map.
- [deployment.md](deployment.md): setup, env, and deployment guide.
- [roadmap.md](roadmap.md): delivery order and cross-phase rules.
- [backend-production-repair-plan.md](backend-production-repair-plan.md): **active** production-readiness hardening plan (remaining items still apply).
- [security-hardening-todo.md](security-hardening-todo.md): security hardening status; implemented items are historical rationale, each section's current-risk line is the remaining work.
- [agent-change-guardrails.md](agent-change-guardrails.md): pre-change/pre-finish checklist and core invariants for coding agents.

## Feature and Interface Docs

- [mcp.md](mcp.md): optional remote MCP endpoint — routes, OAuth, scopes, the `code-reference/` source snapshot, and connecting a client.
- [notes-api.md](notes-api.md): external app guide for profile and shared Home notes over scoped, revocable Notes API tokens.
- [hank-app-platform-contract.md](hank-app-platform-contract.md): stable runtime vs installable app boundary and `.hankapp` compatibility rules.
- [hankai-vector-index.md](hankai-vector-index.md): current inventory of HankAI vector-index sources, provider embedding behavior, and retrieval boundaries.
- [hankai-local-model-evals.md](hankai-local-model-evals.md): checks to run when changing Ollama models, local prompt profiles, planner settings, or vector context packaging.
- [demo-validation.md](demo-validation.md): how demo-server validation stays separate from production code.

## Hank App Integration (`app-integration/`)

Client-side contracts and checklists for the separate Hank app repo. The durable
boundary lives in `hank-app-platform-contract.md` above; these track in-progress
app-side work.

- [app-integration/hank-app-auth-migration.md](app-integration/hank-app-auth-migration.md)
- [app-integration/hank-app-home-sync-checklist.md](app-integration/hank-app-home-sync-checklist.md)
- [app-integration/hank-app-repo-separation-checklist.md](app-integration/hank-app-repo-separation-checklist.md)
- [app-integration/hank-app-product-notes.md](app-integration/hank-app-product-notes.md): product-facing Hank app behavior extracted from historical docs and profile notes.

## Runbooks (`runbooks/`)

- [runbooks/agent-offline.md](runbooks/agent-offline.md)
- [runbooks/auth-failures.md](runbooks/auth-failures.md)
- [runbooks/file-transfer-failures.md](runbooks/file-transfer-failures.md)
- [runbooks/home-assistant-failures.md](runbooks/home-assistant-failures.md)
- [runbooks/single-host-compose.md](runbooks/single-host-compose.md)
- [runbooks/storage-failures.md](runbooks/storage-failures.md)
- [runbooks/token-rotation.md](runbooks/token-rotation.md)

## Browser Surface Scope

- [PWA/current-scope.md](PWA/current-scope.md): records that Hank Remote intentionally serves no standalone PWA, and the conditions for any future mobile-web work.

## Removed Historical Docs

Phase-era implementation plans, dated audits, completed route parity checklists,
and superseded design/task docs have been removed from the active documentation
tree. Current setup, operator, and repair guidance lives in the deployment docs,
runbooks, security hardening note, and production repair plan.

## HankAI Indexing

HankAI indexes root markdown files and every markdown file under `docs/` as the
`Project docs` source. The cloud Docker image copies these files into `/app`,
and local development defaults to the repo root. Override with
`HANK_REMOTE_PROJECT_DOCS_DIR` if the markdown lives elsewhere.
