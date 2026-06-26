# Hank Remote Roadmap

This roadmap records the current delivery priorities for `Hank Remote`. The original phase plans have been completed or superseded and removed from the active documentation tree. Current work should follow [backend-production-repair-plan.md](backend-production-repair-plan.md), [deployment.md](deployment.md), and the runbooks.

## Removed Historical Phase Plans

The phase-era documents were useful during the initial build, but they should not be used as current setup, operator, or repair guidance.

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
