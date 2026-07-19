# Edge-to-Edge Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Slim the desktop sidebar to `204px` and let every dashboard route use the full main workspace without an outer page-card treatment.

**Architecture:** Keep the change in the shared stylesheet rather than altering route components. Pin the presentation contract in the existing stylesheet test, then add a late desktop-only override so established mobile layouts and purposeful inner panels remain unchanged.

**Tech Stack:** React 19, TypeScript, CSS, Vitest, Vite

## Global Constraints

- Preserve unrelated uncommitted Kanban styling and test work.
- Do not commit, push, publish, or deploy.
- Keep purposeful inner cards, columns, controls, separators, and status panels.
- Keep Settings forms readable while removing outer route framing.
- Preserve the existing mobile task-focused layouts and safe-area behavior.

---

### Task 1: Pin the shared desktop workspace contract

**Files:**
- Modify: `web/dashboard/src/styles.test.ts:15-45`
- Test: `web/dashboard/src/styles.test.ts`

**Interfaces:**
- Consumes: the raw `src/styles.css` text and existing `ruleBodies()` helper.
- Produces: regression coverage for the `204px` sidebar token and desktop-only edge-to-edge route override.

- [ ] **Step 1: Add the failing edge-to-edge stylesheet test**

```ts
it("uses the full desktop workspace without an outer page card", () => {
  expect(styles).toContain("/* Productive edge-to-edge desktop workspace */");
  expect(styles).toContain("@media (min-width: 761px)");
  expect(styles).toContain("min-height: calc(100vh - 56px)");
  expect(styles).toContain("padding: 0");
  expect(styles).toContain("max-width: none");
  expect(styles).toContain("border-radius: 0");
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `npm --prefix web/dashboard run test:run -- src/styles.test.ts`

Expected: FAIL because the authoritative desktop workspace block is absent.

### Task 2: Implement the shared edge-to-edge desktop canvas

**Files:**
- Modify: `web/dashboard/src/styles.css:6975-7005`
- Test: `web/dashboard/src/styles.test.ts`

**Interfaces:**
- Consumes: shared `.dashboard-page`, `.route-cache-panel`, `.home-dashboard`, `.settings-layout`, `.settings-content`, `.notes-guide-page`, and `.notes-guide-layout` selectors.
- Produces: a desktop route canvas that fills the main pane below the `56px` top bar while retaining mobile overrides below `761px`.

- [ ] **Step 1: Add the minimal desktop-only override before the mobile responsive pass**

```css
/* Productive edge-to-edge desktop workspace */
@media (min-width: 761px) {
  .app-main > .dashboard-page,
  .app-main > .route-cache-panel > .dashboard-page,
  .app-main > .route-cache-panel > .settings-layout,
  .settings-layout {
    width: 100%;
    max-width: none;
    min-height: calc(100vh - 56px);
    padding: 0;
  }

  .home-dashboard {
    max-width: none;
    padding: 0;
  }

  .notes-guide-page {
    gap: 0;
  }

  .notes-guide-layout {
    min-height: calc(100vh - 56px);
    border: 0;
    border-radius: 0;
    background: transparent;
  }
}
```

- [ ] **Step 2: Run the focused stylesheet test and verify GREEN**

Run: `npm --prefix web/dashboard run test:run -- src/styles.test.ts`

Expected: PASS with no failed tests.

- [ ] **Step 3: Run the complete frontend gate**

Run: `npm --prefix web/dashboard run check`

Expected: all Vitest tests pass, TypeScript emits no errors, and Vite completes a production build.

- [ ] **Step 4: Verify formatting and change isolation**

Run: `git diff --check && git diff -- web/dashboard/src/styles.css web/dashboard/src/styles.test.ts`

Expected: no whitespace errors; the dashboard edits are limited to the shared sidebar/workspace contract while pre-existing Kanban changes remain intact.

### Task 3: Validate representative rendered routes

**Files:**
- Verify only: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: the authenticated dashboard routes and the Browser runtime.
- Produces: visual and computed-style evidence for desktop Notes, Home, Settings, and phone-size Notes.

- [ ] **Step 1: Validate desktop Notes**

Open `/dashboard/profile-notes` at `1212x1083`; verify the page identity, meaningful DOM, no framework overlay, relevant console health, `204px` computed sidebar width, zero outer route padding, zero Notes workspace border/radius, and a navigation interaction.

- [ ] **Step 2: Validate desktop Home and Settings**

Open `/dashboard` and `/dashboard/settings`; verify each route fills the main canvas, Home retains its internal panels, Settings retains its subnavigation rail and readable content column, and no horizontal clipping appears.

- [ ] **Step 3: Validate phone-size Notes**

Open `/dashboard/profile-notes` at a phone viewport; verify the desktop sidebar is hidden, bottom navigation is visible, and `document.documentElement.scrollWidth === document.documentElement.clientWidth`.
