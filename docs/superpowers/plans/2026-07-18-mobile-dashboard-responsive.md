# Mobile Dashboard Responsive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every existing Hank Remote browser route usable on phones while preserving desktop behavior and every backend contract.

**Architecture:** Keep one React route implementation and adapt the current shell at the existing 760px breakpoint. The shell gains a compact mobile header, five-destination bottom navigation, and an overflow menu; route components add only semantic hooks required for card/table transformations, and one final mobile CSS section owns the responsive cascade.

**Tech Stack:** React 19, TypeScript 6, Vite 8, CSS, Vitest, Testing Library, existing dependency-free SVG icon treatment.

## Global Constraints

- Do not add PWA routes, manifests, service workers, offline caching, install prompts, dependencies, backend routes, migrations, permissions, or protocol changes.
- Preserve the desktop dashboard above 760px and the existing visual tokens, icons, typography, and component language.
- Preserve the user's uncommitted Kanban weight changes already in `web/dashboard/src/styles.css` and `web/dashboard/src/styles.test.ts`.
- Do not use subagents unless the user explicitly requests delegation.
- Mobile controls are at least 44 by 44 CSS pixels; mobile inputs use at least 16px type.
- The document does not horizontally overflow at 320, 390, 430, or 768 CSS pixels.
- Work test-first and independently verify each task. Because the two shared style files already contain user-owned changes, leave implementation changes uncommitted unless the responsive hunks can be staged without staging those existing Kanban hunks; never stage either style file wholesale.

---

### Task 1: Mobile Shell Navigation and State

**Files:**
- Modify: `web/dashboard/src/ui/Shell.tsx:1-371`
- Test: `web/dashboard/src/ui/Shell.test.tsx:1-96`

**Interfaces:**
- Consumes: the existing `GroupedNavItem[]`, `currentPath`, `onNavigate`, `onPrefetch`, `onLogout`, session, and connector props.
- Produces: landmarks `Mobile primary` and `Mobile menu`, controls `Open search`, `Close search`, and `Open menu`, plus root attributes `data-mobile-search-open` and `data-mobile-menu-open`.

- [ ] **Step 1: Write failing navigation tests**

Append inside the existing `describe("Shell", ...)`:

```tsx
  it("partitions daily mobile routes from the overflow menu", () => {
    render(
      <Shell
        navItems={[
          { href: "/dashboard", label: "Home" },
          { href: "/dashboard/hank", label: "Hank" },
          { href: "/dashboard/profile-notes", label: "Notes" },
          { href: "/dashboard/home-assistant", label: "Home Assistant" },
          { href: "/dashboard/file-server", label: "File Server" },
          { href: "/dashboard/agents", label: "Agents" },
          { href: "/dashboard/settings", label: "Settings" },
          { href: "/docs/deployment", label: "Setup Guide" },
        ]}
        currentPath="/dashboard/profile-notes"
        onNavigate={vi.fn()}
        onLogout={vi.fn()}
      ><div>Notes content</div></Shell>,
    );
    const primary = screen.getByRole("navigation", { name: "Mobile primary" });
    expect(within(primary).getByRole("link", { name: "Notes" })).toHaveAttribute("aria-current", "page");
    expect(within(primary).getAllByRole("link")).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: "Open menu" }));
    const menu = screen.getByRole("dialog", { name: "Mobile menu" });
    expect(within(menu).getByRole("link", { name: "Agents" })).toBeInTheDocument();
    expect(within(menu).getByRole("link", { name: "Settings" })).toBeInTheDocument();
    expect(within(menu).getByRole("link", { name: "Setup Guide" })).toBeInTheDocument();
  });

  it("dismisses mobile overlays with Escape and restores focus", () => {
    render(<Shell navItems={[{ href: "/dashboard", label: "Home" }]} currentPath="/dashboard" onNavigate={vi.fn()} onLogout={vi.fn()}><div>Home</div></Shell>);
    const menuButton = screen.getByRole("button", { name: "Open menu" });
    fireEvent.click(menuButton);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Mobile menu" })).not.toBeInTheDocument();
    expect(menuButton).toHaveFocus();
    const searchButton = screen.getByRole("button", { name: "Open search" });
    fireEvent.click(searchButton);
    expect(screen.getByRole("button", { name: "Close search" })).toBeInTheDocument();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(searchButton).toHaveFocus();
  });
```

- [ ] **Step 2: Run the tests to prove failure**

Run: `npm --prefix web/dashboard run test:run -- src/ui/Shell.test.tsx`

Expected: FAIL because the mobile landmarks and triggers do not exist.

- [ ] **Step 3: Add partitioning, state, and focus restoration**

Add near `COLLAPSE_KEY`:

```tsx
const MOBILE_PRIMARY_HREFS = new Set([
  "/dashboard", "/dashboard/hank", "/dashboard/profile-notes",
  "/dashboard/home-assistant", "/dashboard/file-server",
]);
```

Add inside `Shell`:

```tsx
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);
  const mobileMenuButtonRef = useRef<HTMLButtonElement>(null);
  const mobileSearchButtonRef = useRef<HTMLButtonElement>(null);
  const mobilePrimaryItems = navItems.filter((item) => MOBILE_PRIMARY_HREFS.has(item.href));
  const mobileOverflowItems = navItems.filter((item) => !MOBILE_PRIMARY_HREFS.has(item.href));
```

Extend the document keyboard effect with these exact Escape branches:

```tsx
      if (e.key === "Escape" && mobileMenuOpen) {
        setMobileMenuOpen(false);
        window.requestAnimationFrame(() => mobileMenuButtonRef.current?.focus());
      }
      if (e.key === "Escape" && mobileSearchOpen) {
        setMobileSearchOpen(false);
        window.requestAnimationFrame(() => mobileSearchButtonRef.current?.focus());
      }
```

Include `mobileMenuOpen` and `mobileSearchOpen` in that effect's dependency list.

- [ ] **Step 4: Render the mobile controls**

Add `data-mobile-search-open={mobileSearchOpen ? "true" : "false"}` and `data-mobile-menu-open={mobileMenuOpen ? "true" : "false"}` to `.app-shell`. Add these controls to `.app-topbar` before the existing breadcrumb:

```tsx
<div className="mobile-topbar-title">
  <img src="/assets/hank-icon-192.png" alt="" />
  <strong>{current?.label || "Hank Remote"}</strong>
</div>
<button ref={mobileSearchButtonRef} className="mobile-topbar-action" type="button" aria-label="Open search"
  onClick={() => {
    setMobileMenuOpen(false);
    setMobileSearchOpen(true);
    window.requestAnimationFrame(() => searchInputRef.current?.focus());
  }}>
  <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m20 20-3-3" /></svg>
</button>
<button ref={mobileMenuButtonRef} className="mobile-topbar-action" type="button" aria-label="Open menu"
  aria-expanded={mobileMenuOpen}
  onClick={() => {
    setNotifOpen(false);
    setMobileSearchOpen(false);
    setMobileMenuOpen((open) => !open);
  }}>
  <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 7h14M5 12h14M5 17h14" /></svg>
</button>
```

Add this inside `.topbar-search-wrap` after the search label:

```tsx
{mobileSearchOpen ? (
  <button className="mobile-search-close" type="button" aria-label="Close search"
    onClick={() => {
      setMobileSearchOpen(false);
      setSearchOpen(false);
      window.requestAnimationFrame(() => mobileSearchButtonRef.current?.focus());
    }}>×</button>
) : null}
```

Add these siblings after `.app-content`:

```tsx
      <nav className="mobile-bottom-nav" aria-label="Mobile primary">
        {mobilePrimaryItems.map((item) => (
          <a key={item.href} href={item.href} aria-label={item.label} aria-current={isActive(item.href) ? "page" : undefined}>
            <NavIcon href={item.href} />
            <span>{item.label === "Home Assistant" ? "HA" : item.label}</span>
          </a>
        ))}
      </nav>
      {mobileMenuOpen ? (
        <div className="mobile-menu-scrim" role="presentation" onPointerDown={() => setMobileMenuOpen(false)}>
          <section className="mobile-menu" role="dialog" aria-modal="true" aria-label="Mobile menu" onPointerDown={(event) => event.stopPropagation()}>
            <header><strong>{footerName}</strong><button type="button" aria-label="Close menu" onClick={() => setMobileMenuOpen(false)}>×</button></header>
            <p>{connectorOnline ? "Connector online" : "Connector offline"} · {roleLabel}</p>
            <nav aria-label="More destinations">
              {mobileOverflowItems.map((item) => <a key={item.href} href={item.href}><NavIcon href={item.href} /><span>{item.label}</span></a>)}
            </nav>
            <button type="button" onClick={onLogout}>Sign out</button>
          </section>
        </div>
      ) : null}
```

- [ ] **Step 5: Run and inspect**

Run: `npm --prefix web/dashboard run test:run -- src/ui/Shell.test.tsx`

Expected: PASS.

Run `git diff -- web/dashboard/src/ui/Shell.tsx web/dashboard/src/ui/Shell.test.tsx` and keep the verified task changes in the current worktree for the final combined review.

---

### Task 2: Authoritative Mobile Shell CSS

**Files:**
- Modify: `web/dashboard/src/styles.css:1-6977`
- Test: `web/dashboard/src/styles.test.ts:1-66`

**Interfaces:**
- Consumes: Task 1 mobile classes and data attributes.
- Produces: `--mobile-topbar-height`, `--mobile-bottom-nav-height`, safe-area spacing, one scroll surface, touch-size controls, and contained overlays/dialogs.

- [ ] **Step 1: Write the failing CSS contract**

```ts
  it("defines the authoritative safe-area mobile shell", () => {
    expect(styles).toContain("/* Mobile responsive pass */");
    expect(styles).toContain("--mobile-bottom-nav-height: 68px");
    expect(styles).toContain("grid-template-rows: auto minmax(0, 1fr)");
    expect(styles).toContain("env(safe-area-inset-bottom)");
    expect(styles).toContain("min-height: 100dvh");
    expect(styles).toContain("max-height: calc(100dvh - 16px)");
  });
```

- [ ] **Step 2: Prove failure**

Run: `npm --prefix web/dashboard run test:run -- src/styles.test.ts`

Expected: FAIL because the marker and variables are absent.

- [ ] **Step 3: Append the authoritative mobile section**

Append after the existing reduced-motion query:

```css
/* Mobile responsive pass */
.mobile-topbar-title, .mobile-topbar-action, .mobile-search-close,
.mobile-bottom-nav, .mobile-menu-scrim { display: none; }

@media (max-width: 760px) {
  :root { --mobile-topbar-height: 56px; --mobile-bottom-nav-height: 68px; --route-page-x-padding: 14px; }
  html, body, #root { width: 100%; min-width: 320px; min-height: 100%; overflow-x: hidden; }
  body { overflow-y: auto; }
  .app-shell, .app-shell[data-nav-collapsed="true"] {
    width: 100%; height: auto; min-height: 100dvh;
    grid-template-columns: minmax(0, 1fr); grid-template-rows: auto minmax(0, 1fr);
  }
  .app-nav, .app-shell[data-nav-collapsed="true"] .app-nav { display: none; }
  .app-content { grid-row: 2; width: 100%; height: auto; min-height: 100dvh; overflow: visible; }
  .app-topbar {
    position: sticky; top: 0; z-index: 30; height: auto;
    min-height: calc(var(--mobile-topbar-height) + env(safe-area-inset-top));
    padding: env(safe-area-inset-top) 10px 0;
  }
  .topbar-crumbs, .operational-pill { display: none; }
  .mobile-topbar-title { min-width: 0; display: flex; align-items: center; gap: 9px; }
  .mobile-topbar-action, .mobile-search-close, .topbar-icon-btn {
    width: 44px; min-width: 44px; min-height: 44px; display: inline-flex; align-items: center; justify-content: center;
  }
  .topbar-search-wrap { display: none; }
  .app-shell[data-mobile-search-open="true"] .topbar-search-wrap {
    position: fixed; z-index: 50; top: calc(env(safe-area-inset-top) + 6px);
    left: 10px; right: 10px; display: flex; gap: 8px;
  }
  .app-shell[data-mobile-search-open="true"] .topbar-search { width: 100%; min-height: 44px; }
  .app-main { overflow: visible; padding-bottom: calc(var(--mobile-bottom-nav-height) + env(safe-area-inset-bottom)); }
  .mobile-bottom-nav {
    position: fixed; z-index: 35; left: 0; right: 0; bottom: 0;
    min-height: calc(var(--mobile-bottom-nav-height) + env(safe-area-inset-bottom));
    display: grid; grid-template-columns: repeat(5, minmax(0, 1fr));
    padding: 5px 6px env(safe-area-inset-bottom); border-top: 1px solid var(--line);
    background: color-mix(in srgb, var(--side) 94%, transparent); backdrop-filter: blur(18px);
  }
  .mobile-bottom-nav a {
    min-width: 0; min-height: 56px; display: grid; place-items: center; align-content: center;
    gap: 3px; border-radius: 10px; color: var(--muted); font-size: 10px; font-weight: 700; text-decoration: none;
  }
  .mobile-bottom-nav a[aria-current="page"] { background: var(--acc-soft); color: var(--brand-dark); }
  .mobile-menu-scrim {
    position: fixed; z-index: 60; inset: 0; display: grid; align-items: end;
    padding: 16px 10px calc(10px + env(safe-area-inset-bottom)); background: rgba(0,0,0,.58);
  }
  .mobile-menu {
    width: 100%; max-height: calc(100dvh - 16px); display: grid; gap: 12px;
    padding: 16px; overflow-y: auto; border: 1px solid var(--line-strong);
    border-radius: 16px; background: var(--surface); box-shadow: var(--shadow);
  }
  .mobile-menu header, .mobile-menu nav a { min-height: 44px; display: flex; align-items: center; gap: 10px; }
  .mobile-menu nav { display: grid; gap: 6px; }
  .mobile-menu nav a { padding: 0 12px; border-radius: 10px; background: var(--surface-strong); text-decoration: none; }
  button, input, select, textarea { min-height: 44px; }
  input, select, textarea { font-size: 16px; }
  .confirm-card, .guide-dialog, .kanban-card-modal { width: calc(100vw - 20px); max-height: calc(100dvh - 16px); }
}
```

- [ ] **Step 4: Run and inspect**

Run: `npm --prefix web/dashboard run test:run -- src/styles.test.ts src/ui/Shell.test.tsx`

Expected: PASS.

Run `git diff -- web/dashboard/src/styles.css web/dashboard/src/styles.test.ts` and confirm the pre-existing Kanban weight hunks remain intact. Do not stage either shared style file wholesale.

---

### Task 3: Home and HankAI Mobile Workflows

**Files:**
- Modify: `web/dashboard/src/dashboard/HankAIPage.tsx:35-332`
- Test: `web/dashboard/src/dashboard/HankAIPage.test.tsx:35-222`
- Modify: `web/dashboard/src/styles.css`
- Test: `web/dashboard/src/styles.test.ts`

**Interfaces:**
- Produces: state `mobileConversationsOpen`, control `Show conversations`, class `mobile-conversations-open`, one-column Home/Hank layouts, and a keyboard-safe composer.

- [ ] **Step 1: Write the failing toggle test**

```tsx
  it("toggles the mobile conversation list", async () => {
    render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);
    expect(await screen.findByRole("heading", { name: "HankAI" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show conversations" }));
    expect(screen.getByRole("region", { name: "Conversations" })).toHaveClass("mobile-conversations-open");
    expect(screen.getByRole("button", { name: "Hide conversations" })).toBeInTheDocument();
  });
```

- [ ] **Step 2: Prove failure**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/HankAIPage.test.tsx`

Expected: FAIL because the toggle is absent.

- [ ] **Step 3: Add the toggle and state class**

Add `const [mobileConversationsOpen, setMobileConversationsOpen] = useState(false);`, render this header control, and add the state class to the existing Conversations region:

```tsx
<button className="secondary hank-mobile-conversations-toggle" type="button"
  aria-expanded={mobileConversationsOpen}
  aria-label={mobileConversationsOpen ? "Hide conversations" : "Show conversations"}
  onClick={() => setMobileConversationsOpen((open) => !open)}>Conversations</button>

<section className={`settings-panel conversations-panel${mobileConversationsOpen ? " mobile-conversations-open" : ""}`} aria-label="Conversations">
```

Close the mobile panel after a successful session selection with `setMobileConversationsOpen(false);`.

- [ ] **Step 4: Add Home/Hank mobile CSS**

Add the desktop default `.hank-mobile-conversations-toggle { display: none; }`, then append inside the authoritative mobile query:

```css
  .home-dashboard, .app-main > .dashboard-page:not(.home-dashboard),
  .app-main > .route-cache-panel > .dashboard-page:not(.home-dashboard) { width: 100%; padding: 18px 14px 28px; }
  .home-hero, .home-hero-actions, .dashboard-header, .chat-panel-header, .composer-bar {
    align-items: stretch; flex-direction: column;
  }
  .home-metrics, .home-board, .home-side-column, .home-quick-grid, .hank-layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .service-row { grid-template-columns: 36px minmax(0, 1fr); }
  .service-chip { grid-column: 2; justify-self: start; }
  .hank-mobile-conversations-toggle { display: inline-flex; }
  .conversations-panel { display: none; }
  .conversations-panel.mobile-conversations-open { display: block; }
  .chat-panel { min-height: calc(100dvh - var(--mobile-topbar-height) - var(--mobile-bottom-nav-height) - 116px); }
  .chat-thread { min-height: 42dvh; }
  .chat-composer { position: sticky; bottom: calc(var(--mobile-bottom-nav-height) + env(safe-area-inset-bottom)); z-index: 4; }
  .composer-send { width: 100%; }
```

- [ ] **Step 5: Run and inspect**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/HankAIPage.test.tsx src/styles.test.ts`

Expected: PASS.

Inspect the component, test, and style diffs together. Leave them uncommitted in the current dirty worktree so the user-owned style hunks cannot be swept into an implementation commit.

---

### Task 4: Notes and Kanban Mobile Editing

**Files:**
- Modify: `web/dashboard/src/styles.css`
- Test: `web/dashboard/src/styles.test.ts`
- Verify: `web/dashboard/src/dashboard/ProfileNotesPage.test.tsx`
- Verify: `web/dashboard/src/dashboard/KanbanEditor.test.tsx`
- Verify: `web/dashboard/src/dashboard/KanbanCardModal.test.tsx`

**Interfaces:**
- Consumes: existing rail state, editor toolbar, Kanban workbar/board, and card modal.
- Produces: touch-size editor tools, stacked rail/editor, snapping columns, and a full-screen mobile card editor.

- [ ] **Step 1: Write the failing CSS contract**

```ts
  it("defines touch-sized Notes and snapping Kanban behavior", () => {
    expect(styles).toContain("scroll-snap-type: x mandatory");
    expect(styles).toContain("grid-auto-columns: min(86vw, 340px)");
    expect(styles).toContain(".notes-toolbar .icon-button");
    expect(styles).toContain(".kanban-card-modal-scroll");
  });
```

- [ ] **Step 2: Prove failure**

Run: `npm --prefix web/dashboard run test:run -- src/styles.test.ts`

Expected: FAIL because the snap rules do not exist.

- [ ] **Step 3: Add Notes/Kanban mobile CSS**

Append inside the authoritative mobile query:

```css
  .notes-guide-layout, .notes-guide-layout.rail-closed { min-height: 0; grid-template-columns: minmax(0, 1fr); overflow: visible; }
  .notes-guide-rail, .notes-rail-collapsed {
    max-height: min(48dvh, 520px); overflow-y: auto; border-right: 0; border-bottom: 1px solid var(--line);
  }
  .notes-guide-editor, .notes-content-scroll { min-width: 0; overflow-x: hidden; }
  .notes-editor-header { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; }
  .notes-title-input, .notes-editor-notebook { grid-column: 1 / -1; width: 100%; }
  .notes-toolbar { overflow-x: auto; flex-wrap: nowrap; overscroll-behavior-x: contain; }
  .notes-toolbar .icon-button, .kanban-formatbar button, .kanban-column-actions button, .kanban-card-move {
    min-width: 44px; min-height: 44px;
  }
  .kanban-workbar { align-items: stretch; flex-wrap: wrap; }
  .kanban-search { width: 100%; }
  .kanban-board {
    grid-template-columns: none; grid-auto-flow: column; grid-auto-columns: min(86vw, 340px);
    overflow-x: auto; scroll-snap-type: x mandatory; overscroll-behavior-x: contain;
  }
  .kanban-column { scroll-snap-align: start; }
  .kanban-card-modal-backdrop { padding: 0; }
  .kanban-card-modal { width: 100vw; min-height: 100dvh; max-height: 100dvh; border-radius: 0; }
  .kanban-card-modal-scroll { overflow-y: auto; padding-bottom: calc(20px + env(safe-area-inset-bottom)); }
```

- [ ] **Step 4: Run and inspect**

Run: `npm --prefix web/dashboard run test:run -- src/styles.test.ts src/dashboard/ProfileNotesPage.test.tsx src/dashboard/KanbanEditor.test.tsx src/dashboard/KanbanCardModal.test.tsx`

Expected: PASS, including the pre-existing bold-weight assertions.

Inspect the style diff and confirm both the new mobile contracts and the pre-existing bold-weight assertions are present. Do not stage the shared style files wholesale.

---

### Task 5: Home Assistant Mobile Entity Rows

**Files:**
- Modify: `web/dashboard/src/dashboard/HomeAssistantPage.tsx:263-339`
- Test: `web/dashboard/src/dashboard/HomeAssistantPage.test.tsx:90-190`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: existing entity actions, switch semantics, tile persistence, and canonical state refresh.
- Produces: `data-label` metadata for Entity, Domain, State, and Tile cells plus labeled mobile rows.

- [ ] **Step 1: Write the failing metadata test**

Add to the existing entity-table test:

```tsx
    const table = screen.getByRole("table", { name: "All Home Assistant entities" });
    const humidityRow = within(table).getByRole("row", { name: /Humidity/ });
    expect(humidityRow.querySelector('[data-label="Entity"]')).not.toBeNull();
    expect(humidityRow.querySelector('[data-label="Domain"]')).not.toBeNull();
    expect(humidityRow.querySelector('[data-label="State"]')).not.toBeNull();
    expect(humidityRow.querySelector('[data-label="Tile"]')).not.toBeNull();
```

- [ ] **Step 2: Prove failure**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/HomeAssistantPage.test.tsx`

Expected: FAIL because the cells lack `data-label`.

- [ ] **Step 3: Label the table cells**

Change the four cell opening tags in `EntityTable` while retaining their current children:

```tsx
<td data-label="Entity">
<td data-label="Domain">{domain}</td>
<td data-label="State">
<td data-label="Tile">
```

- [ ] **Step 4: Add mobile entity-card CSS**

Append inside the authoritative mobile query:

```css
  .ha-dashboard-grid { grid-template-columns: minmax(0, 1fr); }
  .ha-entities-table-wrap { overflow: visible; }
  .ha-entities-table, .ha-entities-table tbody, .ha-entities-table tr, .ha-entities-table td {
    display: block; width: 100%;
  }
  .ha-entities-table thead {
    position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0);
  }
  .ha-entities-table tr {
    margin-bottom: 10px; padding: 12px; border: 1px solid var(--line);
    border-radius: 12px; background: var(--surface-strong);
  }
  .ha-entities-table td {
    min-height: 44px; display: grid; grid-template-columns: minmax(82px, .35fr) minmax(0, 1fr);
    align-items: center; gap: 10px; padding: 6px 0; border: 0;
  }
  .ha-entities-table td::before {
    content: attr(data-label); color: var(--faint); font: 600 10px var(--font-mono); text-transform: uppercase;
  }
  .ha-entities-table td[data-label="Entity"] { display: block; }
  .ha-entities-table td[data-label="Entity"]::before { display: none; }
  .ha-entities-table button { min-height: 44px; }
```

After the mobile query add:

```css
@media (min-width: 481px) and (max-width: 760px) {
  .ha-dashboard-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
```

- [ ] **Step 5: Run and inspect**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/HomeAssistantPage.test.tsx src/styles.test.ts`

Expected: PASS.

Inspect the semantic table changes and responsive CSS together, leaving the verified work uncommitted for the final combined review.

---

### Task 6: File Server Mobile Rows and Preview

**Files:**
- Modify: `web/dashboard/src/dashboard/FileServerPage.tsx:589-805`
- Test: `web/dashboard/src/dashboard/FileServerPage.test.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: `rowFor`, selection/actions, `previewOpen`, and the existing preview close button.
- Produces: labels for Name, Size, Type, Modified, and Actions plus a full-screen mobile preview sheet.

- [ ] **Step 1: Write the failing file-row test**

Add to the ready-state File Server test:

```tsx
    const filesTable = await screen.findByRole("table", { name: "Files" });
    const rows = within(filesTable).getAllByRole("row");
    expect(rows[1].querySelector('[data-label="Name"]')).not.toBeNull();
    expect(rows[1].querySelector('[data-label="Size"]')).not.toBeNull();
    expect(rows[1].querySelector('[data-label="Type"]')).not.toBeNull();
    expect(rows[1].querySelector('[data-label="Modified"]')).not.toBeNull();
```

- [ ] **Step 2: Prove failure**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/FileServerPage.test.tsx`

Expected: FAIL because row cells lack labels.

- [ ] **Step 3: Label the file row cells**

Use these opening tags in `rowFor`:

```tsx
<span className="file-guide-name" role="cell" data-label="Name">
<span className="file-guide-mono" role="cell" data-label="Size">{formatSize(item)}</span>
<span className="file-guide-muted" role="cell" data-label="Type">{fileType(item)}</span>
<span className="file-guide-mono" role="cell" data-label="Modified">{formatModified(item)}</span>
<span className="file-guide-menu-cell" role="cell" data-label="Actions">
```

- [ ] **Step 4: Add File Server mobile CSS**

Append inside the authoritative mobile query:

```css
  .file-guide-header, .file-guide-tools, .file-guide-actions,
  .file-selection-bar, .file-selection-actions, .file-preview-actions {
    align-items: stretch; flex-wrap: wrap;
  }
  .file-share-wrap, .file-search { width: 100%; max-width: none; margin-right: 0; }
  .file-guide-panes, .file-guide-panes.preview-closed { grid-template-columns: minmax(0, 1fr); }
  .file-tree-pane { max-height: 38dvh; overflow-y: auto; border-right: 0; border-bottom: 1px solid var(--line); }
  .file-list-scroll { overflow: visible; }
  .file-guide-table { min-width: 0; }
  .file-guide-head { display: none; }
  .file-guide-row {
    min-width: 0; display: grid; grid-template-columns: 44px minmax(0, 1fr) 44px;
    gap: 4px 8px; padding: 10px 8px;
  }
  .file-guide-name { grid-column: 2; }
  .file-guide-row > [data-label="Size"], .file-guide-row > [data-label="Type"],
  .file-guide-row > [data-label="Modified"] { grid-column: 2; display: inline-flex; gap: 6px; }
  .file-guide-row > [data-label="Size"]::before, .file-guide-row > [data-label="Type"]::before,
  .file-guide-row > [data-label="Modified"]::before { content: attr(data-label) ":"; color: var(--faint); }
  .file-guide-menu-cell { grid-column: 3; grid-row: 1 / span 4; align-self: start; }
  .file-check, .file-menu-button, .file-view-toggle button, .file-preview-close { min-width: 44px; min-height: 44px; }
  .file-preview-panel {
    position: fixed; z-index: 55; inset: 0; width: 100vw; height: 100dvh;
    display: grid; grid-template-rows: minmax(220px, 45dvh) minmax(0, 1fr) auto;
    overflow: hidden; border: 0; background: var(--bg);
  }
  .file-preview-info { overflow-y: auto; }
  .file-preview-actions { padding-bottom: calc(12px + env(safe-area-inset-bottom)); }
  .file-transfer-job { grid-template-columns: 30px minmax(0, 1fr); }
```

- [ ] **Step 5: Run and inspect**

Run: `npm --prefix web/dashboard run test:run -- src/dashboard/FileServerPage.test.tsx src/styles.test.ts`

Expected: PASS.

Inspect the semantic row changes and responsive CSS together, leaving the verified work uncommitted for the final combined review.

---

### Task 7: Settings, Agents, Auth, Guide, and Dialogs

**Files:**
- Modify: `web/dashboard/src/settings/SettingsLayout.tsx:1-116`
- Test: `web/dashboard/src/settings/SettingsLayout.test.tsx:1-59`
- Modify: `web/dashboard/src/styles.css`
- Test: `web/dashboard/src/styles.test.ts`
- Verify: `web/dashboard/src/dashboard/AgentsPage.tsx`
- Modify: `web/dashboard/src/dashboard/DeploymentGuide.tsx`
- Verify: `web/dashboard/src/auth/LoginPage.tsx`
- Verify: `web/dashboard/src/auth/JoinPage.tsx`
- Verify: `web/dashboard/src/auth/PasswordChangePage.tsx`
- Verify: `web/dashboard/src/ui/primitives.tsx`

**Interfaces:**
- Produces: group `Mobile settings section`, permission-filtered links, one-column admin forms/cards, contained terminal/dialogs, and locally scrolling guide code.

- [ ] **Step 1: Write the failing chooser test**

```tsx
  it("renders a permission-filtered mobile section chooser", () => {
    render(<SettingsLayout currentPath="/dashboard/settings/people" isAdmin={false}><section>People page</section></SettingsLayout>);
    const chooser = screen.getByRole("group", { name: "Mobile settings section" });
    expect(within(chooser).getByText("People")).toBeInTheDocument();
    expect(within(chooser).getByRole("link", { name: "Home" })).toBeInTheDocument();
    expect(within(chooser).queryByRole("link", { name: "Apps ADMIN" })).not.toBeInTheDocument();
  });
```

- [ ] **Step 2: Prove failure**

Run: `npm --prefix web/dashboard run test:run -- src/settings/SettingsLayout.test.tsx`

Expected: FAIL because the mobile chooser is absent.

- [ ] **Step 3: Add the chooser**

Compute:

```tsx
const visibleTabs = settingsTabs.filter((tab) => !tab.adminOnly || isAdmin);
const activeTab = visibleTabs.find((tab) => tab.href === normalizedCurrentPath) || visibleTabs[0];
```

Render before `.settings-subnav`:

```tsx
<details className="settings-mobile-chooser" aria-label="Mobile settings section">
  <summary><span>Settings</span><strong>{activeTab ? guideLabel(activeTab) : "Home"}</strong></summary>
  <nav aria-label="Mobile settings destinations">
    {visibleTabs.map((tab) => {
      const label = guideLabel(tab);
      const accessibleLabel = tab.adminOnly ? `${label} ADMIN` : label;
      return <a key={tab.href} href={tab.href} aria-current={tab.href === normalizedCurrentPath ? "page" : undefined} aria-label={accessibleLabel}><SettingsIcon name={iconFor(tab)} /><span>{label}</span>{tab.adminOnly ? <span className="admin-badge" aria-hidden="true">ADMIN</span> : null}</a>;
    })}
  </nav>
</details>
```

- [ ] **Step 4: Add the guide hook and remaining mobile CSS**

Change the guide root to `<div className="dashboard-page deployment-page">` so long commands can be contained without affecting unrelated code elements.

Add desktop default `.settings-mobile-chooser { display: none; }` and append inside the mobile query:

```css
  .settings-layout { min-height: 0; grid-template-columns: minmax(0, 1fr); }
  .settings-subnav { display: none; }
  .settings-mobile-chooser {
    display: block; margin: 14px 14px 0; border: 1px solid var(--line);
    border-radius: 12px; background: var(--surface);
  }
  .settings-mobile-chooser summary {
    min-height: 52px; display: flex; align-items: center; justify-content: space-between;
    gap: 12px; padding: 0 14px; cursor: pointer;
  }
  .settings-mobile-chooser nav { display: grid; gap: 4px; padding: 0 8px 8px; }
  .settings-mobile-chooser a { min-height: 44px; display: flex; align-items: center; gap: 10px; padding: 0 10px; text-decoration: none; }
  .settings-content { width: 100%; max-width: none; padding: 18px 14px 30px; }
  .settings-content form, .settings-content .form-grid, .agent-token-form,
  .agent-grid, .agent-detail-stats { grid-template-columns: minmax(0, 1fr); }
  .settings-actions, .button-row, .agent-detail-head, .agent-shell-head, .agent-shell-actions {
    align-items: stretch; flex-wrap: wrap;
  }
  .agent-card, .agent-panel, .agent-detail, .agent-shell { min-width: 0; }
  .agent-tile-metrics { flex-wrap: wrap; }
  .agent-token-table, .agent-token-table tbody, .agent-token-table tr, .agent-token-table td { display: block; width: 100%; }
  .agent-token-table thead { display: none; }
  .agent-token-table tr { margin-top: 10px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; }
  .agent-token-table td { padding: 6px 0; border: 0; }
  .agent-token-actions { text-align: left; }
  .agent-shell-terminal { min-height: 42dvh; max-height: calc(100dvh - 220px); }
  .auth-surface { min-height: 100dvh; padding: max(14px, env(safe-area-inset-top)) 14px max(14px, env(safe-area-inset-bottom)); }
  .auth-card { width: 100%; padding: 20px 16px; }
  .deployment-page pre, .deployment-page code, .settings-panel pre {
    max-width: 100%; overflow-x: auto; white-space: pre; overscroll-behavior-x: contain;
  }
  .confirm-scrim, .guide-dialog-scrim { align-items: end; padding: 10px; }
  .confirm-card, .guide-dialog {
    width: 100%; max-height: calc(100dvh - 16px); overflow-y: auto;
    padding-bottom: calc(16px + env(safe-area-inset-bottom));
  }
```

- [ ] **Step 5: Run and inspect**

Run: `npm --prefix web/dashboard run test:run -- src/settings/SettingsLayout.test.tsx src/ui/primitives.test.tsx src/App.test.tsx src/styles.test.ts`

Expected: PASS.

Inspect the Settings, guide, and style diffs together. Leave them uncommitted in the current dirty worktree so the existing user-owned style hunks remain outside any commit.

---

### Task 8: Full Regression and Responsive Browser Verification

**Files:**
- Verify: `web/dashboard/src`
- Verify: generated `internal/cloud/ui/react`
- Verify: `docs/superpowers/specs/2026-07-18-mobile-dashboard-responsive-design.md`

**Interfaces:**
- Consumes: Tasks 1–7.
- Produces: passing production checks and visual evidence for every top-level route.

- [ ] **Step 1: Run the complete dashboard gate**

Run: `npm --prefix web/dashboard run check`

Expected: the full Vitest suite passes, TypeScript reports no errors, and Vite builds `internal/cloud/ui/react`.

- [ ] **Step 2: Run whitespace validation**

Run: `git diff --check`

Expected: no output and exit status 0.

- [ ] **Step 3: Verify exact responsive viewports in the authenticated dashboard**

Inspect `320x568`, `390x844`, `430x932`, `768x1024`, and landscape `844x390`. At each applicable route verify:

```text
No document-level horizontal overflow.
The top bar and five-item bottom navigation remain reachable.
The mobile menu exposes Agents, Settings, Setup Guide, status, and sign out.
Home cards and actions form a readable column.
Hank conversations and composer remain reachable.
Notes rail, toolbar, Kanban columns, and card editor remain usable.
Home Assistant rows expose entity, state, domain, and tile action.
File rows, selection, preview close/actions, and transfers remain reachable.
Agents, Settings, auth, guide code, and dialogs remain contained.
The desktop sidebar remains present at 768x1024.
```

Expected: each statement is visibly true or the affected route is explicitly recorded as blocked by unavailable permissions or live data.

- [ ] **Step 4: Inspect final boundaries**

Run `git status --short` and inspect the final diff by file.

Expected: the responsive implementation remains clearly reviewable alongside, but does not overwrite or implicitly commit, the user's pre-existing Kanban changes and audit artifacts.

---

## Plan Self-Review

- Spec coverage: shell, Home, Hank, Notes/Kanban, Home Assistant, File Server, Agents, Settings, auth, Setup Guide, dialogs, safe areas, touch targets, responsive widths, desktop preservation, and production build verification map to explicit tasks.
- Placeholder scan: every code-changing step includes the exact test, markup, state, attribute, or CSS contract to add.
- Type consistency: all existing public component and API interfaces remain unchanged; new state is local boolean state owned by `Shell` or `HankAIPage`.
