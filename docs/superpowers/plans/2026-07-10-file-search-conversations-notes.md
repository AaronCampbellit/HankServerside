# File Search, Conversation Deletion, and Notes Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Restore whole-share file search, make Hank conversations deletable, and make dashboard Notes reliably save, move, and display previews.

**Architecture:** The dashboard will query the existing authenticated files.search command rather than filtering the open directory. The agent will keep a source-scoped, bounded in-memory file index so searches are fast after the first scan and mutations invalidate stale results. Dashboard-only changes will surface the existing session deletion route and make Notes persistence state truthful.

**Tech Stack:** Go standard library, React 19, TypeScript, Vitest, existing WebSocket command protocol.

## Global Constraints

- File access stays agent-owned and authorization continues through ListSource and source policies.
- Search results remain scoped to the selected file source; no cross-share mixing.
- No schema or migration change.
- Tests precede production changes and prove the reported interaction.

---

### Task 1: Cached source-scoped file search

**Files:**
- Modify: internal/agent/files/service.go, internal/agent/files/service_test.go
- Modify: web/dashboard/src/dashboard/FileServerPage.tsx, web/dashboard/src/dashboard/FileServerPage.test.tsx

- [ ] Write a failing Go test that creates a file deeper than the current directory and verifies search returns it after the first index build, then add a test proving a mutation invalidates the relevant source index.
- [ ] Run go test ./internal/agent/files -run TestSearch and confirm the new cache expectation fails.
- [ ] Add a mutex-protected source index with a bounded TTL, single-flight refresh per source, path-safe recursive discovery, stable fuzzy score sorting, and invalidation after create, rename, move, upload, and delete.
- [ ] Run go test ./internal/agent/files -run TestSearch and confirm it passes.
- [ ] Write a failing dashboard test that types a query and expects fileServerClient.search(query, selectedSource) results, rather than only local folder filtering.
- [ ] Run npm --prefix web/dashboard run test:run -- FileServerPage.test.tsx and confirm it fails for the missing request.
- [ ] Debounce source-scoped requests, discard stale responses, render search results in the existing file list, and retain normal folder listing when the query is blank.
- [ ] Run the focused dashboard test and confirm it passes.

### Task 2: Conversation deletion affordance

**Files:**
- Modify: web/dashboard/src/dashboard/HankAIPage.tsx, web/dashboard/src/dashboard/HankAIPage.test.tsx, web/dashboard/src/styles.css

- [ ] Write a failing component test that opens a conversation action, confirms deletion, calls hankAIClient.deleteSession, removes the session, and selects a safe remaining conversation.
- [ ] Run npm --prefix web/dashboard run test:run -- HankAIPage.test.tsx and confirm it fails.
- [ ] Implement an accessible hover/swipe-revealed Delete action with the existing confirmation dialog; remove the deleted session and clear its messages when none remain.
- [ ] Run the focused test and confirm it passes.

### Task 3: Notes persistence, notebook moves, and preview clipping

**Files:**
- Modify: web/dashboard/src/dashboard/ProfileNotesPage.tsx, web/dashboard/src/dashboard/ProfileNotesPage.test.tsx, web/dashboard/src/styles.css

- [ ] Write failing tests proving selecting a notebook persists parent_id, body/title blur saves edits, and the status shows unsaved while edits are pending.
- [ ] Run npm --prefix web/dashboard run test:run -- ProfileNotesPage.test.tsx and confirm the tests fail.
- [ ] Track saved editor content/revision separately from draft state; save safely on blur and notebook selection, prevent duplicate concurrent writes, preserve server conflict errors, and make the status reflect saved/saving/unsaved/error.
- [ ] Add clipping and ellipsis rules for notebook-card previews without changing stored content.
- [ ] Run the focused Notes tests and confirm they pass.

### Task 4: Integration verification

**Files:**
- Modify: no additional files expected

- [ ] Run gofmt -w internal/agent/files/service.go internal/agent/files/service_test.go.
- [ ] Run go test ./internal/agent/files ./internal/agent, go test ./internal/cloud, npm --prefix web/dashboard run test:run, npm --prefix web/dashboard run build, and go test ./....
- [ ] Inspect the final diff for source scoping, auth preservation, and unrelated changes; run git diff --check.
