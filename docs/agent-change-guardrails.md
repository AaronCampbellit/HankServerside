# Agent Change Guardrails

This file is for Codex and other coding agents working in `HankServerside`.
Use it as a pre-change and pre-finish checklist for any non-trivial feature,
bug fix, cleanup, migration, or deployment change.

The goal is not to slow down development. The goal is to prevent convenient
changes from weakening the remote-access security model, damaging production
data, or leaving the repo harder to operate.

## Core Invariants

- Keep the product scoped to one self-hosted deployment for one home unless the
  user explicitly changes that scope.
- Keep the app on the Hank Remote cloud API over HTTPS/WebSocket.
- Keep local network access and local credentials inside the home agent.
- Do not expose SMB, Home Assistant, local files, or raw local protocols to the
  public internet.
- Do not add VPN requirements back into the architecture.
- Do not add app-side protocol hacks in this repo.
- Prefer stable app-facing commands such as `files.list`,
  `homeassistant.fetch_states`, and `notes.sync` over raw protocol tunneling.
- Keep cloud, agent, protocol, store, and UI responsibilities separate.

## Security Gate

Before finishing any change that touches auth, routing, files, storage,
settings, dashboard writes, agent commands, tokens, or external service calls,
verify these points:

- Every new route has authentication unless it is intentionally public.
- Every authenticated route enforces the correct user, home, role, or token
  scope server-side.
- Cookie-authenticated browser writes use CSRF protection.
- Bearer-token and WebSocket flows do not rely on secrets in query strings.
- Raw tokens, passwords, OAuth credentials, APNs tokens, service secrets, and
  file contents are not logged.
- Stored secrets are encrypted or hashed using the existing project helpers
  where appropriate.
- File paths are cleaned, symlink-resolved, and contained inside the configured
  root before read, write, stat, delete, rename, upload, or download work.
- Destructive admin or file actions are authenticated, audited, and gated by
  the existing confirmation/token pattern when impact is high.
- `/healthz` and `/readyz` may remain public; sensitive operational data such
  as `/metrics` must stay authenticated or internal-only.

## Database Gate

Treat database changes as production changes, even during feature work.

- Do not hide schema changes inside unrelated handler or store edits.
- Do not casually add `CREATE TABLE`, `ALTER TABLE`, or data backfills outside
  the migration/store path that the repo already uses.
- Any schema change must define the table/column/index/constraint behavior, the
  default or nullability story, and the backfill or compatibility story.
- Prefer database constraints for important integrity rules instead of relying
  only on handler checks.
- Do not weaken uniqueness, foreign-key, authorization, or home-scoping
  guarantees without documenting why.
- Avoid destructive migrations unless there is a staged compatibility period or
  a clear backup/restore plan.
- If a DB model changes, update store reads/writes, API payloads, tests,
  migrations/status checks, and docs together.
- After DB-impacting work, run migration status and schema drift validation when
  the local environment supports it.

## Cleanup Gate

Cleanup is useful, but it must be intentional.

- Do not remove compatibility paths, old columns, routes, env vars, or docs just
  because they look stale.
- When cleanup is requested, first identify what still reads, writes, deploys,
  links to, or documents the target.
- Remove stale behavior in one coherent pass: code, tests, docs, scripts, and
  operator instructions.
- Do not auto-clean production data or schemas. Report the cleanup need and use
  an explicit migration or operator step.

## Testing And Validation

For meaningful code changes, run the smallest set that proves the changed
surface and then broaden validation when security, persistence, routing, or
operator workflows are involved.

Default checks:

```bash
gofmt -w ./cmd ./internal
go build ./...
go test ./...
```

Database-impacting checks:

```bash
make migrate-status
make schema-drift-check
```

Deployment/setup-impacting checks:

```bash
scripts/doctor.sh
```

Use live demo/server validation only when the task requires it and access is
available. Healthy `/healthz` and `/readyz` responses alone do not prove a
frontend asset or operator workflow has been updated.

## Required Finish Note

When reporting completion, include a short summary of:

- Security impact: what auth, secret, file, or route behavior changed.
- Database impact: no schema change, migration added, or schema checks run.
- Validation: exact commands run and any checks that were skipped.

If a required check cannot run, say why and name the remaining risk.
