# Hank Remote Roadmap

This roadmap records the original implementation shape for `Hank Remote`. The phase plans have been completed or superseded and are now archived as historical context. Current work should follow [backend-production-repair-plan.md](backend-production-repair-plan.md), [deployment.md](deployment.md), and the runbooks.

## Archived Phase List

1. [Phase 1: Identity And Routing](./archive/phases/phase-1-identity-and-routing.md)
2. [Phase 2: Home Assistant Remote Access](./archive/phases/phase-2-home-assistant.md)
3. [Phase 3: File API And NAS Access](./archive/phases/phase-3-files.md)
4. [Phase 4: Notes Sync And Background Flows](./archive/phases/phase-4-notes.md)
5. [Phase 5: Operations, Security, And Production Readiness](./archive/phases/phase-5-operations.md)
6. [Phase 6: Hank Assistant](./archive/phases/phase-6-hank-assistant.md)

## Current Delivery Order

1. Preserve the single-home cloud-and-agent deployment model.
2. Keep schema changes in embedded versioned migrations with status and drift checks.
3. Keep Home Assistant, files, notes, media, assistant, backup, and restore flows aligned with the dashboard Settings and deployment docs.
4. Use the runbooks for operator troubleshooting.
5. Treat multi-home SaaS and multi-node cloud assumptions as out of scope unless the product scope changes.

## Rules Across Every Phase

- Keep the app-facing API high-level.
- Keep the home agent outbound-only.
- Do not expose SMB directly to the internet.
- Prefer simple JSON over HTTPS/WebSocket unless streaming or performance forces a change.
- Add tests when protocol or routing behavior changes.
- Keep cloud concerns and agent concerns separated in code.
