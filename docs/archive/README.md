# Hank Remote Docs Archive

Historical project material, retained for implementation context and
traceability. **Nothing here is current setup, operator, or repair guidance** —
use the active docs in `docs/` (start from
[../project-knowledge-index.md](../project-knowledge-index.md)).

## Contents

- **[phases/](phases)** — the original phase plans and per-phase tasklists from
  the initial Hank Remote build. See [phases/README.md](phases/README.md).
- **[audits/](audits)** — dated, point-in-time audits whose findings were
  resolved in later cleanup passes:
  - `backend-architecture-audit.md` (2026-05-30)
  - `legacy-code-audit.md` (2026-06-01)
  - `project-cleanup-audit-2026-06-06.md` (2026-06-06)
- **[designs/](designs)** — design specs for features that are now implemented
  or superseded by the current code/docs (the active code and docs are the
  source of truth):
  - installable agent apps, invite/signup/password reset, redacted settings
    recovery, first-party app platform readiness, HankAI local-model eval harness.
- **[plans/](plans)** — completed or superseded implementation plans and one-off
  tasklists. They are not current task trackers:
  - app platform readiness, HankAI local-model eval harness, HankAI chat-tool
    improvement, HankAI intents rollout, and the Codex production-readiness task pass.

## Why keep these

They document why the system is shaped the way it is. They are still indexed by
HankAI but flagged as archived so they rank below active docs.
