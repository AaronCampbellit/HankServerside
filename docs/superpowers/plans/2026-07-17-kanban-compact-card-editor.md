# Compact Kanban Card Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace oversized Kanban creation and modal chrome with compact controls that prioritize card description editing.

**Architecture:** Reuse the existing Kanban state and handlers. Move only the Add task trigger in `KanbanEditor`, change the title field density in `KanbanCardModal`, and tighten the existing CSS rules without adding components or data paths.

**Tech Stack:** React 19, TypeScript, CSS, Vitest, Testing Library

## Global Constraints

- Preserve accessible labels and all existing Kanban behavior.
- Add no dependencies, routes, API changes, or schema changes.
- Keep the existing responsive modal containment behavior.

---

### Task 1: Lock the compact interaction contract

**Files:**
- Modify: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanCardModal.test.tsx`
- Modify: `web/dashboard/src/styles.test.ts`

**Interfaces:**
- Consumes: existing `KanbanEditor` and `KanbanCardModal` accessible labels.
- Produces: regression coverage for header Add task placement, one-line title, and compact CSS dimensions.

- [ ] **Step 1: Write failing tests**

Assert that `Add task to Inbox` is inside `.kanban-column-actions`, the task title textarea has `rows="1"`, and compact CSS rules cap modal/header/upload density.

- [ ] **Step 2: Verify tests fail for the current oversized layout**

Run: `npm --prefix web/dashboard test -- --run src/dashboard/KanbanEditor.test.tsx src/dashboard/KanbanCardModal.test.tsx src/styles.test.ts`

Expected: failures for header placement, title rows, and compact CSS values.

### Task 2: Implement the compact layout

**Files:**
- Modify: `web/dashboard/src/dashboard/KanbanEditor.tsx`
- Modify: `web/dashboard/src/dashboard/KanbanCardModal.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: existing `setAddingColumnID`, task modal props, upload handler, and description editor.
- Produces: compact controls with unchanged accessible names and event behavior.

- [ ] **Step 1: Move Add task into column actions**

Place the icon-only trigger immediately before the existing Delete button and render the composer only when active.

- [ ] **Step 2: Tighten modal chrome**

Use a single-row title and reduce modal minimum height, header padding, scroll gaps, metadata padding, toolbar height, and upload row height.

- [ ] **Step 3: Verify targeted tests pass**

Run: `npm --prefix web/dashboard test -- --run src/dashboard/KanbanEditor.test.tsx src/dashboard/KanbanCardModal.test.tsx src/styles.test.ts`

Expected: all targeted tests pass.

### Task 3: Validate and deploy

**Files:**
- No additional source files.

**Interfaces:**
- Consumes: demo server deployment workflow and Browser plugin.
- Produces: live desktop and narrow-width visual proof.

- [ ] **Step 1: Run broad local verification**

Run: `npm --prefix web/dashboard run check && go test ./... && go build ./... && git diff --check`

Expected: all checks pass; informational canvas and chunk-size warnings may remain.

- [ ] **Step 2: Commit and deploy the exact commit**

Commit only the scoped design, plan, tests, component, and CSS changes. Transfer the commit to the demo server without pushing GitHub and rebuild cloud/agent with `HANK_REMOTE_BUILD_VERSION` set to the exact SHA.

- [ ] **Step 3: Verify live behavior**

Confirm Add task is a compact header icon, the composer opens below the header, the modal gives the description the dominant space, screenshots remain inline, desktop and narrow widths contain the modal, browser logs are clean, `/readyz` reports the exact commit, and `scripts/doctor.sh` has zero failures.

