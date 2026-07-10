# Search, Conversations, and Notes Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore whole-share File Server search, add Hank conversation deletion, and make Notes persistence, notebook moves, and previews reliable.

**Architecture:** Use the existing authenticated `files.search` command from the dashboard, harden the agent search traversal and add a short-lived source-scoped in-memory index cache invalidated by file mutations. Add the already-supported conversation DELETE action to the existing React list. Make Notes save state explicit, save notebook selection through the existing PUT contract, and constrain previews with CSS.

**Tech Stack:** Go agent/cloud services, React 19 + TypeScript, Vitest, PostgreSQL-backed Notes API.

## Global Constraints

- Preserve authenticated source scoping and do not expose SMB directly.
- Preserve Notes revision conflict handling and existing migration/store paths.
- Keep unrelated worktree changes untouched.
- Verify frontend with focused Vitest and `npm --prefix web/dashboard run build`; verify Go with `gofmt`, `go build ./...`, and `go test ./...` when available.

### Task 1: Whole-share File Server search

**Files:**
- Modify: `internal/agent/files/service.go`
- Test: `internal/agent/files/service_test.go`
- Modify: `web/dashboard/src/dashboard/FileServerPage.tsx`
- Test: `web/dashboard/src/dashboard/FileServerPage.test.tsx`

- [ ] Add failing tests proving a search request returns nested results and stale results cannot overwrite a newer query.
- [ ] Run focused tests and confirm failure.
- [ ] Implement source-scoped cached recursive search with bounded TTL, invalidation after mutations, and depth/visit limits that do not truncate normal shares.
- [ ] Wire the search results into the existing dashboard search state while preserving the current folder view when the query is empty.
- [ ] Run focused tests and confirm pass.

### Task 2: Hank conversation deletion

**Files:**
- Modify: `web/dashboard/src/dashboard/HankAIPage.tsx`
- Test: `web/dashboard/src/dashboard/HankAIPage.test.tsx`

- [ ] Add a failing test for a visible per-conversation Delete action that calls the existing client DELETE method and removes the deleted session.
- [ ] Run the focused test and confirm failure.
- [ ] Add an accessible hover/swipe-compatible delete action with confirmation and selection fallback.
- [ ] Run the focused test and confirm pass.

### Task 3: Notes persistence and preview reliability

**Files:**
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Modify: `web/dashboard/src/styles.css`
- Test: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`

- [ ] Add failing tests for notebook selector persistence, real dirty/saved state, and long preview clipping.
- [ ] Run focused tests and confirm failure.
- [ ] Save notebook changes through the existing `saveNote` PUT path, track dirty/saving/saved/error states, and keep revision values synchronized after save.
- [ ] Add min-width/overflow/ellipsis rules to notebook cards and previews.
- [ ] Run focused tests and confirm pass.

### Task 4: Broad validation

- [ ] Run `gofmt -w ./cmd ./internal`.
- [ ] Run `go build ./...` and `go test ./...`.
- [ ] Run `npm --prefix web/dashboard run test:run` and `npm --prefix web/dashboard run build`.
- [ ] Review `git diff --check` and preserve unrelated changes.
