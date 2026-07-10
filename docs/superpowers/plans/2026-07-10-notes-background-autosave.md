# Notes Background Autosave Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Save Notes automatically in the background without interrupting editing or losing newer work.

**Architecture:** Add a 750 ms debounced autosave scheduler inside `ProfileNotesPage`. Reuse the existing serialized `saveNote(Editor)` queue, suppress success toasts for background saves, and flush pending snapshots before note navigation.

**Tech Stack:** React 19, TypeScript, Vitest fake timers, existing Profile Notes HTTP client.

## Global Constraints

- Preserve the existing revision-conflict contract.
- Never replace newer editor content with an older save response.
- Background saves must not steal focus, block typing, or show success toasts.
- Keep the manual Save action.

---

### Task 1: Debounced background autosave

**Files:**
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Test: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`

**Interfaces:**
- Consumes: existing `saveNote(editor: Editor)` serialized save path.
- Produces: a 750 ms timer that sends the latest dirty editor snapshot and a navigation flush helper.

- [ ] **Step 1: Write failing tests**

Add Vitest coverage proving no request before 750 ms, one request after 750 ms with the latest content, no success toast interruption, and a pending edit flush before selecting another note.

- [ ] **Step 2: Verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/ProfileNotesPage.test.tsx`

Expected: autosave expectations fail because only manual/blur saves exist.

- [ ] **Step 3: Implement minimal autosave scheduler**

Add one timer ref, one latest-editor ref, and an effect keyed to dirty editor content. Clear/reschedule the timer on edits, call the existing queue after 750 ms, and flush before navigation. Add a background flag so successful autosaves do not emit toasts.

- [ ] **Step 4: Verify GREEN and broad compatibility**

Run:

```bash
npm --prefix web/dashboard run test:run -- src/dashboard/ProfileNotesPage.test.tsx
npm --prefix web/dashboard run test:run
npm --prefix web/dashboard run build
git diff --check
```

Expected: all commands exit successfully.
