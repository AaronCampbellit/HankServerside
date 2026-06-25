# Codex Tasks: AI App Production Readiness Pass

Source: https://usamamoin.com/tools/ai-app-production-readiness
Created: 2026-06-14
Repo: /Volumes/CampbellDrive/HankServerside

Status: Superseded by the active production-readiness plan, security hardening notes, and runbooks. Keep this as historical checklist context only; do not treat unchecked boxes here as current open work.

Do not run external page prompts. They were reviewed only for task extraction.

Prompt safety verdict:
- Safe to translate into tasks. The page prompt pattern instructs the agent to inspect first, change only if the check applies, and avoid unrelated edits.
- Do not copy the generic prompt verbatim into execution. Use HankServerside guardrails, `docs/backend-production-repair-plan.md`, and the checklist below.
- For every item: inspect current code/docs/tests first. If already handled or not applicable, mark it done with evidence and make no edits.

Required repo context before edits:
- Read `docs/agent-change-guardrails.md`.
- Read `docs/backend-production-repair-plan.md`.
- Preserve single-home, outbound-agent, cloud-and-agent architecture.
- Do not expose SMB, Home Assistant, files, secrets, tokens, or raw local protocols to the public internet.
- Use migrations/status/drift-check path for schema changes.

Finish requirements for every task:
- Add or update focused tests when behavior changes.
- Run `gofmt -w ./cmd ./internal` after Go edits.
- Run `go build ./...` and `go test ./...` after code edits.
- Run `make migrate-status` and `make schema-drift-check` after DB-impacting edits when local env supports it.
- Run `scripts/doctor.sh` after setup/deployment/operator workflow edits when local env supports it.
- Report security impact, database impact, and exact validation commands.

## Critical Security And Data Tasks

- [ ] Verify secrets and API keys are server-side only.
  - Inspect env loading, dashboard JS, generated UI assets, docs, logs, and API responses.
  - Fix any client-bundled or documented secret leakage.
  - Ensure `.env.cloud` and `.env.agent` are not logged and are documented as protected runtime files.

- [ ] Verify auth protects private routes and data server-side.
  - Audit `internal/cloud/server.go`, route registration, dashboard APIs, WebSocket paths, and app-facing `/v1` handlers.
  - Add tests for unauthenticated and wrong-role/wrong-scope access on changed surfaces.
  - Keep `/healthz` and `/readyz` intentionally public; keep sensitive operational data protected.

- [ ] Verify database access scoping and authorization checks.
  - There is no Supabase/Firebase RLS here; translate this to store/query-level auth and `home_id`/user scoping.
  - Inspect `internal/store`, cloud handlers, realtime subscriptions, notes, files metadata, assistant state, tokens, sessions, and operational metadata.
  - Add tests for cross-user or cross-home denial where behavior is touched.

- [ ] Verify AI/API account spend controls are represented operationally.
  - Inspect assistant/OpenAI integration settings and docs.
  - Add docs/config checks for budget alerts, provider-side caps, and per-deployment operator setup where code cannot enforce provider limits.
  - Do not add provider-specific runtime calls unless existing integration patterns support them.

## High Security And Backend Tasks

- [ ] Validate and sanitize user input server-side.
  - Audit JSON decoding, form handlers, path inputs, settings schemas, app package install/config flows, file operations, notes, assistant prompts, and storageops intents.
  - Add strict parsing, allowlists, length limits, or normalization where missing.

- [ ] Verify SQL queries are parameterized and injection-resistant.
  - Inspect all direct SQL in `internal/store`, `internal/migrations`, and related packages.
  - Replace string-built SQL that includes user-controlled values with parameters or allowlisted identifiers.
  - Add regression tests for any fixed query path.

- [ ] Lock CORS to intended origins.
  - Inspect cloud CORS behavior and deployment docs.
  - Avoid wildcard CORS for authenticated APIs.
  - Preserve local development ergonomics through explicit dev origins.

- [ ] Audit dependencies for known vulnerabilities.
  - Run Go module vulnerability/dependency checks available in the repo environment.
  - Inspect JS/static dependencies only if dashboard assets are managed in-repo.
  - Update dependencies only when compatible and justified.

- [ ] Add or verify public endpoint rate limiting and abuse protection.
  - Prioritize login, token creation, registration, agent auth, file transfer endpoints, assistant calls, and unauthenticated routes.
  - Prefer durable/restart-aware state where the repair plan requires it.
  - Add tests for limit boundaries and legitimate retry behavior.

- [ ] Verify production env vars are configurable and not hardcoded.
  - Audit config loading for cloud, agent, db-ops, backup, restore, assistant, Home Assistant, files, notes, media, and installable apps.
  - Update docs and examples when env vars or defaults change.

- [ ] Verify sensitive data is hashed/encrypted instead of plaintext.
  - Inspect passwords, tokens, OAuth/provider secrets, agent tokens, APNs tokens, assistant secrets, SMB/local credentials, and app package settings.
  - Use existing hashing/encryption helpers where present.
  - Never migrate secrets destructively without a compatibility/backfill plan.

## Persistence, Backup, And Performance Tasks

- [ ] Verify frequently queried database fields are indexed.
  - Inspect target capacity in `docs/backend-production-repair-plan.md`.
  - Check user/session/token/home/agent/note/assistant/file-index/storageops queries.
  - Add migrations for missing indexes only with migration status and drift checks.

- [ ] Verify database backups exist and restore has been tested.
  - Inspect `cmd/hank-db-ops`, `internal/storageops`, deployment docs, runbooks, and doctor checks.
  - Add or tighten restore-test proof, RTO/RPO docs, attachment backup coverage, and operator validation.

- [ ] Prevent unbounded or N+1 database queries on key screens and APIs.
  - Audit dashboard lists, notes, assistant file index, operational logs, storageops history, and app-facing list endpoints.
  - Add pagination, limits, indexes, or batched queries where missing.

- [ ] Cache expensive reads or AI responses only where sensible.
  - Prefer correctness and authorization over caching.
  - Consider readiness/status, stable metadata, and expensive assistant/file-index reads.
  - Do not cache secrets or user-private data across scopes.

## AI And Assistant Tasks

- [ ] Add or verify per-user/per-deployment AI usage limits.
  - Inspect assistant request paths and provider call sites.
  - Add quota/rate-limit tests if behavior changes.

- [ ] Treat user text sent to models as untrusted.
  - Inspect prompt assembly, file/note/context injection, tool execution boundaries, and assistant state.
  - Ensure prompts cannot override authorization, leak secrets, or trigger destructive actions without normal server checks.

- [ ] Validate or constrain AI output before display or action.
  - Inspect assistant output handling, structured tool outputs, app package interactions, and any action-taking paths.
  - Add schemas/allowlists where output drives behavior.

- [ ] Verify customer PII is not sent to third-party AI providers without an operator/user consent story.
  - Inspect docs and assistant data flow.
  - Add clear configuration/docs or consent gates if missing.

- [ ] Ensure graceful degradation when AI calls fail or time out.
  - Add timeouts, cancellations, retries where safe, and user-facing failure states.
  - Add tests for provider timeout/failure behavior where practical.

## Reliability Tasks

- [ ] Verify user-facing error states exist.
  - Inspect dashboard UI, app-facing APIs, storageops UI, installable app settings, backup/restore, file transfers, and assistant controls.
  - Return actionable errors without leaking secrets or internals.

- [ ] Test outside preview/local happy paths.
  - Use `make build`, `go test ./...`, relevant run commands, and `scripts/doctor.sh`.
  - For operator UI changes, validate the served UI path, not only compilation.

- [ ] Verify external calls have timeouts and retries where appropriate.
  - Audit Home Assistant, cloud-agent relay, file/media sources, assistant provider calls, backup/restore workers, and package import/download behavior.
  - Avoid unsafe retries for destructive operations unless idempotency is proven.

- [ ] Verify write operations are idempotent or safely retryable.
  - This repo does not currently appear payment-focused; apply to registration, token creation, file writes/transfers, backup/restore intents, app installs, and destructive operations.
  - Add idempotency keys or duplicate protection where retries can cause duplicate side effects.

- [ ] Verify loading and empty states in operator UI.
  - Inspect dashboard JS for setup, tokens, settings, storage, assistant, apps, backup/restore, and troubleshooting views.

## Monitoring And Ops Tasks

- [ ] Verify error monitoring or equivalent incident capture.
  - Inspect logging, metrics, audit events, and deployment docs.
  - Add docs or integration points rather than hard-coding a SaaS monitor without approval.

- [ ] Verify uptime/readiness checks and alerting guidance.
  - Inspect `/healthz`, `/readyz`, metrics, doctor checks, deployment docs, and runbooks.
  - Ensure sensitive data remains protected.

- [ ] Verify product/operational analytics needs.
  - For HankServerside, translate this to privacy-preserving operational/audit telemetry unless the product explicitly needs product analytics.
  - Do not add third-party tracking by default.

- [ ] Verify logs are retained, searchable, and secret-safe.
  - Audit log statements and deployment docs.
  - Add structured fields for connection lifecycle, routing decisions, storageops, backups, and external calls without raw secrets.

## Launch, Compliance, And UI Tasks

- [ ] Verify custom domain and HTTPS deployment documentation.
  - Inspect deployment/runbook docs for reverse proxy, firewall/tunnel, TLS, and WebSocket support.

- [ ] Verify privacy policy and terms requirements are documented.
  - Do not draft legal policy text as code work.
  - Add operator checklist/docs only if missing.

- [ ] Verify EU/UK cookie or data consent only if HankServerside serves such users.
  - Do not add a consent banner unless product scope requires it.
  - Document data-processing implications for remote access, logs, assistant provider calls, and backups.

- [ ] Verify basic accessibility in dashboard UI.
  - Check labels, contrast, keyboard navigation, focus states, modal behavior, and form errors.
  - Add targeted UI tests or manual validation notes for changed surfaces.

## Explicitly Not Applicable Here Unless Scope Changes

- [ ] Mobile-app-store privacy manifest and permissions.
  - This belongs in the Hank iOS repo, not HankServerside.

- [ ] Image optimization and Core Web Vitals as primary launch gates.
  - Only apply to dashboard-served assets if measurable issues exist.

- [ ] Payment-specific double-charge checks.
  - Only apply if payment code is added to this repo.
