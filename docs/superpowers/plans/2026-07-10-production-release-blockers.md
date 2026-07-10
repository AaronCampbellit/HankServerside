# Production Release Blockers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce an immutable HankServerside release candidate whose local, PostgreSQL-backed, operational, restore, authenticated-live, and dashboard gates all pass on the Hank demo server.

**Architecture:** Preserve the single-home cloud/agent design and current dashboard work. Make file-operation job transitions monotonic at the store boundary, repair DB-backed tests according to the current API contracts and isolated database behavior, then deploy one exact Git commit to the demo host and validate it through public and authenticated surfaces.

**Tech Stack:** Go 1.26, PostgreSQL/pgvector, Docker Compose, React/Vite/Vitest, Bash validation tooling.

## Global Constraints

- Do not alter or commit `.env.cloud`, `.env.agent`, session tokens, SSH material, generated reports, or `.claude/worktrees`.
- Keep production schema changes inside `internal/migrations/sql`; this repair should require no schema migration.
- Keep `/healthz` and `/readyz` public while `/metrics` and `/v1/home` remain authenticated.
- Use synthetic demo data only for file and live validation.
- Do not deploy to production; stop at a demo-proven production candidate.

---

### Task 1: Make file-job terminal states monotonic

**Files:**
- Modify: `internal/store/production_state.go`
- Modify: `internal/cloud/file_jobs.go`
- Test: `internal/cloud/production_validation_test.go`

**Interfaces:**
- Consumes: `Store.UpdateFileOperationJob` and agent `FileOperationJobResponse`/`FileOperationJobEvent` state updates.
- Produces: an atomic store update that rejects non-terminal state writes after `completed`, `failed`, `cancelled`, `rollback_required`, or `rolled_back`.

- [ ] **Step 1: Verify the existing regression is red**

Run the DB-backed `TestManagedFileMoveFailureRetryAndCancelLifecycle`; expect `retry job status = "running", want completed`.

- [ ] **Step 2: Add an atomic non-regression store update**

Add a focused store method whose SQL updates a job only when the incoming state is terminal or the persisted state is not terminal. Return whether a row changed so callers can read the winning terminal state.

- [ ] **Step 3: Use the monotonic update for retry responses**

Replace the unconditional retry-response write with the atomic method and return the persisted job snapshot.

- [ ] **Step 4: Verify green repeatedly**

Run the focused DB-backed test at least five times and run `scripts/file-safety-validation.sh`; expect zero failures.

### Task 2: Repair the full PostgreSQL-backed test gate

**Files:**
- Modify tests and owning production files under `internal/cloud/`, `internal/store/`, and `internal/testutil/` only where the isolated DB run proves a current contract or concurrency defect.
- Test: every failing test reported by `go test -count=1 ./...` with `HANK_REMOTE_TEST_DATABASE_URL` set.

**Interfaces:**
- Consumes: `testutil.PostgreSQLTestURL`, current router/auth behavior, Notes revision/tag contracts, assistant persistence/indexing, and pgvector availability checks.
- Produces: a repeatable full suite against real PostgreSQL with no skipped DB tests and no package-global cross-test leakage.

- [ ] **Step 1: Re-run each failure individually**

For every failing test, run its exact `-run '^TestName$' -count=1` command against the demo PostgreSQL base URL. Classify it as deterministic contract drift, production defect, or package-global parallel interference.

- [ ] **Step 2: Repair deterministic contract drift**

Update stale assertions only when current route/API behavior is already documented and security-safe; otherwise repair the owning production behavior. Re-run each focused test after each change.

- [ ] **Step 3: Repair shared-state interference**

Remove or isolate package-global mutation at the source. Do not disable the whole suite's parallelism as a blanket workaround.

- [ ] **Step 4: Run package and repository DB gates**

Run `go test -count=1 ./internal/cloud`, `go test -count=1 ./internal/store`, and `go test -count=1 ./...` against isolated PostgreSQL. Expect all packages green.

### Task 3: Tidy and package the release candidate

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Preserve and include: current dashboard Home Assistant and semantic quick-link changes plus their tests.

**Interfaces:**
- Consumes: the current working-tree dashboard implementation.
- Produces: a clean dependency graph and one reviewable Git commit on a `codex/` release-candidate branch.

- [ ] **Step 1: Run dependency tidy**

Run `go mod tidy`, then `go mod tidy -diff`; expect no diff.

- [ ] **Step 2: Run local gates**

Run gofmt, `go build ./...`, `go vet ./...`, non-DB `go test -count=1 ./...`, and `npm --prefix web/dashboard run check`.

- [ ] **Step 3: Review and commit only candidate files**

Exclude `.claude/`, secrets, and generated `data/`. Inspect `git diff --check` and the staged diff, then commit the release candidate.

### Task 4: Deploy the exact candidate to the Hank demo server

**Files:**
- Server checkout: `/home/campbellservers/HankServerside`
- Private server files preserved: `.env.cloud`, `.env.agent`, `data/`, and tunnel/container state.

**Interfaces:**
- Consumes: the exact local candidate commit.
- Produces: demo containers stamped with the candidate commit and fresh React asset hashes.

- [ ] **Step 1: Transfer the Git commit without production push**

Create a Git bundle for the candidate commit, copy it to the demo host, fetch it into a `codex/` branch, and check out that exact commit while preserving private env/data files.

- [ ] **Step 2: Build and restart the Hank stack**

Run migrations and `docker compose --env-file .env.cloud --profile agent up --build -d`. Do not remove `hankdemo-cloudflared`.

- [ ] **Step 3: Prove deployment freshness**

Compare the server commit, `/readyz` runtime version, built dashboard asset names/hashes, and container content against the candidate.

### Task 5: Refresh restore and authenticated-live evidence

**Files:**
- Generated, untracked reports under server `data/`.
- Temporary session material only under server `/tmp`, removed or replaced after use.

**Interfaces:**
- Consumes: a legitimate demo admin session and the existing db-ops restore workflow.
- Produces: current restore proof, live/admin validation, and authenticated metrics evidence.

- [ ] **Step 1: Obtain a legitimate demo admin session**

Use an existing signed-in browser session or the documented demo login path. Do not weaken auth, edit password hashes, or forge session rows.

- [ ] **Step 2: Request and complete a fresh restore test**

Use the authenticated storage API, wait for success, then run `scripts/restore-proof.sh`; expect live and restore counts to match.

- [ ] **Step 3: Run authenticated validation**

Run `tools/livevalidation`, `tools/adminvalidation`, and `scripts/metrics-assert.sh` against `https://hankdemo.campbellservers.com`; expect all checks to pass.

### Task 6: Run the complete release checklist

**Files:**
- Read: `RELEASE.md`
- Generated, untracked reports under server `data/`.

**Interfaces:**
- Consumes: the deployed exact candidate.
- Produces: a production go/no-go backed by fresh evidence.

- [ ] **Step 1: Run operational gates**

Run doctor, alert-rule validation, restart validation, file safety, strict migrations, deep schema drift, migration baseline, restore proof, and metrics assertions.

- [ ] **Step 2: Run final code gates**

Run format/tidy/build/vet/dashboard checks plus the full DB-backed Go suite against an isolated PostgreSQL database.

- [ ] **Step 3: Confirm security and migration posture**

Verify strict secret storage, public endpoint policy, authenticated metrics, no schema drift, and no unexpected migration.

- [ ] **Step 4: Report readiness**

Report production-ready only if every required command exits zero and the demo serves the exact candidate. Otherwise continue repairing in scope.
