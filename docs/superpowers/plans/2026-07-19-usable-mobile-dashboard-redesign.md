# Usable Mobile Dashboard Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan inline. Repository instructions prohibit subagents unless the user explicitly requests delegation.

**Goal:** Make the existing dashboard genuinely usable at phone widths by turning each high-traffic route into one focused mobile workspace while preserving desktop behavior, APIs, permissions, and user-owned changes.

**Architecture:** Keep the current React route components and desktop markup. Add small, local disclosure states where mobile needs explicit modes, then make the final `@media (max-width: 760px)` section the authoritative responsive layer. Behavioral changes are covered by component tests; layout contracts are covered by stylesheet tests and same-viewport browser validation.

**Tech Stack:** React 19, TypeScript, Vite, Vitest, Testing Library, CSS media queries.

---

### Task 1: Make Settings entry permission-aware

**Files:**
- Modify: `web/dashboard/src/ui/navConfig.ts`
- Modify: `web/dashboard/src/settings/SettingsLayout.tsx`
- Modify: `web/dashboard/src/settings/SettingsLayout.test.tsx`
- Modify: `web/dashboard/src/App.tsx`
- Modify: `web/dashboard/src/App.test.tsx`

**Step 1: Write failing tests**

- Add a `settingsLandingPath(isAdmin)` test proving admins land on `/dashboard/settings/home` and members land on the first permitted tab, `/dashboard/settings/quick-links`.
- Update the member layout test so Home is not visible and Quick Links is the active root destination.
- Add an App route test proving a member visiting `/dashboard/settings` is rendered on Quick Links without first mounting `HomeSettings`.

**Step 2: Verify RED**

Run: `npm --prefix web/dashboard test -- src/settings/SettingsLayout.test.tsx src/App.test.tsx`

Expected: tests fail because the root still resolves to Home and Home is not permission-gated in the settings tab configuration.

**Step 3: Implement minimal behavior**

- Mark Home & Connector as admin-only because its current data APIs require administrator access. Keep member-readable Quick Links, People, Connections, AI & MCP, and Join Home visible; their existing write controls continue to follow server permissions.
- Export `settingsLandingPath(isAdmin)` from `SettingsLayout.tsx`.
- Resolve the generic settings route to the landing path after bootstrap permissions load, without changing direct unauthorized URLs.
- Ensure the mobile chooser and desktop rail normalize `/dashboard/settings` against the permission-aware landing path.

**Step 4: Verify GREEN**

Run the same focused test command and confirm it passes.

### Task 2: Rebuild Home as a mobile control center

**Files:**
- Create: `web/dashboard/src/dashboard/DashboardHome.test.tsx`
- Modify: `web/dashboard/src/dashboard/DashboardHome.tsx`
- Modify: `web/dashboard/src/styles.test.ts`
- Modify: `web/dashboard/src/styles.css`

**Step 1: Write failing tests**

- Render an online healthy home and assert the mobile priority area contains one compact connector summary, does not render a disabled restart action, and separates healthy services into an `All services` disclosure.
- Render a degraded home and assert attention rows remain visible before the disclosure.
- Add stylesheet assertions for the compact two-column summary, hidden mobile-only disabled action, and mobile service disclosure.

**Step 2: Verify RED**

Run: `npm --prefix web/dashboard test -- src/dashboard/DashboardHome.test.tsx src/styles.test.ts`

Expected: tests fail because all service rows and both large actions are currently always rendered in the primary flow.

**Step 3: Implement minimal behavior**

- Derive attention and healthy service rows from the existing service model.
- Render a compact mobile status/action block and an `All services` disclosure while retaining the desktop service panel.
- Keep enabled restart/setup actions reachable, but remove giant disabled controls from the phone hierarchy.
- Restyle metrics as a compact 2×2 summary and move quick actions above historical sections on mobile with CSS ordering.

**Step 4: Verify GREEN**

Run the same focused tests and confirm they pass.

### Task 3: Make Notes a true mobile master-detail workspace

**Files:**
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/ProfileNotesPage.tsx`
- Modify: `web/dashboard/src/styles.test.ts`
- Modify: `web/dashboard/src/styles.css`

**Step 1: Write failing tests**

- Assert the page starts in browser mode, selecting a note switches the layout to editor mode, and `Back to notes` restores browser mode.
- Assert creating a note also opens editor mode.
- Add stylesheet assertions that browser and editor panes are mutually exclusive below 760px and that lower-frequency toolbar controls live in a labeled disclosure instead of silent horizontal clipping.

**Step 2: Verify RED**

Run: `npm --prefix web/dashboard test -- src/dashboard/ProfileNotesPage.test.tsx src/styles.test.ts`

Expected: tests fail because both panes are stacked and there is no mobile pane state or back control.

**Step 3: Implement minimal behavior**

- Add local `mobilePane` state with `browser` as the initial value.
- Switch to editor after note selection, new note, or notebook creation; add a `Back to notes` button in the editor header.
- Add semantic mode classes/data attributes so desktop continues to show the split view.
- Divide formatting controls into core actions and a `More formatting` disclosure, preserving every existing action.

**Step 4: Verify GREEN**

Run the same focused tests and confirm they pass.

### Task 4: Bound and densify Home Assistant results

**Files:**
- Modify: `web/dashboard/src/dashboard/HomeAssistantPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/HomeAssistantPage.tsx`
- Modify: `web/dashboard/src/styles.test.ts`
- Modify: `web/dashboard/src/styles.css`

**Step 1: Write failing tests**

- Load more than 24 entities and assert only the first 24 rows are in the initial result set, the result summary reports the visible/total count, and `Show more` reveals the next batch.
- Assert changing the search resets the visible count.
- Add stylesheet assertions for a sub-80px mobile row contract and a sticky search toolbar.

**Step 2: Verify RED**

Run: `npm --prefix web/dashboard test -- src/dashboard/HomeAssistantPage.test.tsx src/styles.test.ts`

Expected: tests fail because the page renders up to 80 oversized card-like rows at once.

**Step 3: Implement minimal behavior**

- Keep the existing 80-result query cap but paginate the rendered result set in batches of 24.
- Add a compact result summary and explicit Show more control.
- Reset the rendered batch when the query changes.
- Restyle mobile rows as name/state-first rows with compact metadata and trailing actions; preserve table semantics and desktop table styling.

**Step 4: Verify GREEN**

Run the focused tests and confirm they pass.

### Task 5: Turn Files into one browser with separate activity

**Files:**
- Modify: `web/dashboard/src/dashboard/FileServerPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/FileServerPage.tsx`
- Modify: `web/dashboard/src/styles.test.ts`
- Modify: `web/dashboard/src/styles.css`

**Step 1: Write failing tests**

- Assert the page exposes an `Activity` toggle with an active-transfer count and keeps the transfer region collapsed until opened.
- Assert the unified file result list remains the only mobile folder collection through semantic mobile classes.
- Add stylesheet assertions that the folder rail is hidden on mobile and list rows use one compact metadata line under 80px.

**Step 2: Verify RED**

Run: `npm --prefix web/dashboard test -- src/dashboard/FileServerPage.test.tsx src/styles.test.ts`

Expected: tests fail because Transfers are always expanded and the folder rail remains stacked above duplicate folder rows.

**Step 3: Implement minimal behavior**

- Add local `activityOpen` state and an Activity trigger near the browser controls.
- Keep transfer content visible on desktop and conditionally exposed on mobile via semantic classes and ARIA state.
- Hide the separate folder rail on mobile while preserving the combined folder/file result list.
- Collapse size/type/modified into one concise mobile metadata line without removing desktop cells or actions.

**Step 4: Verify GREEN**

Run the focused tests and confirm they pass.

### Task 6: Make Hank a full-height conversation workspace

**Files:**
- Modify: `web/dashboard/src/dashboard/HankAIPage.test.tsx`
- Modify: `web/dashboard/src/dashboard/HankAIPage.tsx`
- Modify: `web/dashboard/src/styles.test.ts`
- Modify: `web/dashboard/src/styles.css`

**Step 1: Write failing tests**

- Assert provider/readiness are presented as one compact status line and the conversation header has no duplicated Ready pill.
- Add stylesheet assertions for a dynamic-viewport workspace and a composer that stays inside the conversation surface above the bottom navigation.

**Step 2: Verify RED**

Run: `npm --prefix web/dashboard test -- src/dashboard/HankAIPage.test.tsx src/styles.test.ts`

Expected: tests fail because readiness is repeated and the mobile page retains nested dashboard framing.

**Step 3: Implement minimal behavior**

- Consolidate provider/readiness into one status line.
- Remove duplicated framing and the accidental nested conversation-row wrapper.
- Use mobile CSS to make the conversation panel the route workspace and keep the composer at its bottom.

**Step 4: Verify GREEN**

Run the focused tests and confirm they pass.

### Task 7: Integrated verification and visual correction

**Files:**
- Modify only files needed to correct failures found during validation.

**Step 1: Run the dashboard quality gate**

Run: `npm --prefix web/dashboard run check`

Fix only regressions attributable to this work, then rerun until clean.

**Step 2: Run repository-relevant build checks**

Run: `go build ./...`

Run: `go test ./...`

If pre-existing failures occur, record exact evidence and keep the frontend result independently verified.

**Step 3: Start a local preview and capture the same states**

- Build/serve the dashboard using the repository's existing Vite workflow.
- Use the selected Codex in-app browser at 430×932.
- Capture Home, Hank, Notes browser/editor, Home Assistant, Files/activity, Menu, and Settings as fresh local screenshots.

**Step 4: Compare and correct**

- Place each live audit reference and corresponding local screenshot into the same comparison input.
- Correct visible overflow, clipped controls, oversized rows, bad hierarchy, spacing, borders, and viewport/composer issues.
- Repeat at 320, 390, 430, 760, and 768px.
- Confirm `document.documentElement.scrollWidth === document.documentElement.clientWidth` for every audited route.

**Step 5: Review the final diff**

Run: `git diff --check`

Run: `git status --short`

Verify user-owned `AGENTS.md`, Kanban typography changes, and `.codex/` audit artifacts remain preserved and unstaged unless explicitly selected.
