# Hank Remote Dashboard GUI Audit

Date: 2026-06-02

## Executive Take

The dashboard is close to the right product shape: a left-hand operator shell, clear Home/Tools/Settings grouping, reusable panels, and practical setup/status surfaces. The main issue is that the GUI is now held together by nested iframe composition, embedded-mode CSS, frame resizing, and `postMessage` navigation. That makes the app feel more consistent than the old separate pages, but it is also the highest-risk source of polish defects.

Recommendation: keep iframes only as a short-term reuse bridge and for true document/file preview. Do not make them the long-term settings layout architecture. Move Settings panes toward first-class DOM sections or a small client-side shell that mounts page modules directly.

## Current Shape

- `internal/cloud/ui/admin-nav.js` turns normal dashboard pages into a framed single-shell experience by replacing `main` with `.dashboard-content-frame`.
- `internal/cloud/ui/settings.html` then embeds each Settings pane in its own `.settings-pane-frame`.
- `internal/cloud/ui/settings.js` handles lazy frame loading, hash-to-tab routing, frame height measurement, resize observers, and forwarding dashboard navigation messages.
- `internal/cloud/ui/styles.css` contains a growing embedded-dashboard mode that hides page chrome and resets page layout inside frames.
- Most page scripts now use `window.HankAPI.request`, which is the right direction for CSRF and error handling consistency.

## Are Iframes The Right Move?

Iframes are reasonable for:

- Short-term reuse of existing pages while the dashboard shell is being consolidated.
- File previews where the content is an actual document/media surface.
- Containing a one-off route with very different lifecycle requirements.

Iframes are not the right long-term move for Settings, Home panels, or normal operator workflows. The current Settings page has seven iframe-backed panes, including Home and Quick Links pointing back to `/dashboard`. That creates a nested app inside an app: history, hash routing, height, focus, clipboard permissions, scroll position, loading state, and active nav state all need custom code.

The current code already shows this tax:

- Settings panes need a custom resizer and minimum height guard.
- Embedded pages need global CSS that hides the hero and nav.
- Navigation has to cross frame boundaries via `postMessage`.
- Clipboard permissions had to be explicitly allowed for dashboard frames.
- Home and Quick Links are duplicated conceptually: they are dashboard panels and Settings panes at the same time.

Decision: keep the current iframe path until a small refactor lands, but treat it as transitional. The cleaner professional target is one dashboard shell with direct in-page sections.

## Priority Updates

### 1. Replace Settings Iframe Panes With Native Sections

Settings should render native sections under one page shell:

- Home
- Quick Links
- People
- Connections
- AI
- Backups
- Join Home

The simplest path is not a framework rewrite. Extract each pane's content into reusable partial-style HTML sections and move each pane script toward an `init(container, options)` pattern. Keep the existing routes temporarily for deep links and compatibility, but have Settings mount the same modules directly.

This removes frame resizing, improves keyboard/focus behavior, makes mobile scroll predictable, and gives Settings one loading/error/toast system.

### 2. Make The Shell Real, Not Frame-Driven

`admin-nav.js` currently creates `.dashboard-content-frame` for smooth navigation. That improves perceived consistency, but it means every dashboard page is still a separate mini-app. For a professional operator console, the shell should be the stable document and the selected section should be normal DOM.

Near-term compromise:

- Keep full page routes working.
- Stop nesting Settings panes inside the outer dashboard frame when already on `/dashboard/settings`.
- Avoid framing pages that already have complex internal state, especially File Server, Notes, and Hank chat.

Longer-term:

- Convert the sidebar search/navigation into a real shell-level router.
- Mount page modules directly where practical.
- Use full-page navigation only for heavy sections that are not worth modularizing yet.

### 3. Standardize Page Density And Layout Rules

The current UI has a good restrained visual base, but the composition varies by page. Tighten these rules:

- One shell: left nav, page title row, compact session/account status.
- One panel language: 8px radius, consistent padding, consistent header/action placement.
- Use dense tables/lists for operational data instead of repeated decorative cards.
- Keep page titles compact in dashboard views; reserve large hero styling for login only.
- Make settings tabs look like an operator sidebar or segmented control, not another row of page nav competing with the main sidebar.

The biggest polish improvement would be reducing the top hero footprint. In an operator dashboard, page title, status, and logout can live in a compact top bar.

### 4. Introduce A Small UI Component Layer

There is a lot of hand-built repeated UI: pills, empty states, toasts, panel heads, tabs, status chips, action rows, and inline forms. Add a lightweight shared helper file rather than a full framework:

- `renderToast(message, options)`
- `renderEmptyState(container, config)`
- `setBusy(button, isBusy, label)`
- `renderStatusChip(status)`
- `confirmDangerAction(config)`
- `bindTabs(root, options)`

This would make Home, Home Assistant, File Server, Storage, Notes, and AI settings feel more consistent without changing backend behavior.

### 5. Finish API Client Consolidation

Most scripts use `window.HankAPI.request`, which is good. Continue by moving repeated direct fetch/download/upload error handling into named helpers:

- JSON request
- blob download
- upload with progress hook
- event stream or WebSocket setup
- standard error-to-toast mapping

This matters because GUI consistency is not only visual. A professional app reports failures the same way everywhere.

### 6. Add GUI Regression Checks

Add a small browser smoke suite for:

- Login page layout.
- Dashboard shell loads without double nav.
- Settings deep links open the correct tab.
- Admin-only Backups tab is hidden for non-admin users.
- Mobile width does not overlap nav, hero, or panel controls.
- File Server, Notes, Hank chat, and Home Assistant main surfaces render without blank frames.

The app has several pages with complex responsive CSS. Visual regressions will keep recurring until the GUI has browser-level checks.

## Page-Specific Notes

### Home

Home is the best candidate for a polished operator landing screen. Keep Quick Links, First Setup, Connector status, and Health, but reduce visual competition. Make Health and First Setup prominent only when action is needed. Otherwise, default to Quick Links and current connector status.

### Settings

Settings should be the first refactor target. It currently depends most heavily on iframe composition and hash routing. Native sections will immediately improve perceived quality.

### Home Assistant

The direction is right: lightweight dashboard/search, not a raw command console. Tighten it by keeping entity search, favorites, common controls, and live state updates prominent. Put low-level diagnostics behind a collapsible troubleshooting section.

### File Server

This is a full application surface. It can stay visually denser than the rest of the dashboard. Priorities are predictable selection state, stable command bar layout, clear source selector, and professional empty/error states. Iframes should only be used for file preview, not for the page itself.

### Hank Chat

Hank chat should feel like a workbench, not a settings page. Keep logs and sessions collapsible. Make the active conversation and tool/result state more visually stable than decorative.

### Notes

Notes already has its own workspace feel. Keep it native, not framed long-term. The main consistency need is matching top navigation/status behavior and shared toast/error language.

## Suggested Implementation Order

1. Create shared dashboard UI helpers and move toasts/empty states/busy buttons onto them.
2. Convert Settings from iframe panes to direct native sections, starting with Home and Quick Links.
3. Convert People, Connections, AI, Backups, and Join Home to direct mounted sections.
4. Replace outer iframe navigation for Settings and other light pages with direct shell mounting.
5. Keep File Server, Notes, and Hank chat on full routes until they have explicit module boundaries.
6. Add browser screenshots/smoke checks for desktop and mobile widths.

## Guardrails

- Do not weaken route-level auth while moving UI sections. Existing server-side membership/admin checks must remain.
- Do not expose local Home Assistant, SMB, files, media, or secrets in browser-only logic.
- Do not remove existing routes until all sidebar links, search results, deep links, docs, and tests are updated.
- Keep full-page fallback routes during the transition so operator bookmarks continue to work.
- Validate Settings saves with actual persisted state, not just optimistic toasts.

## Definition Of Done For A GUI Tightening Pass

- Settings has no nested iframe panes for normal sections.
- Sidebar navigation and Settings tabs do not fight each other.
- Desktop and mobile screenshots show no overlapping text or controls.
- Every save action has consistent busy, success, no-op, and error behavior.
- Admin-only controls are hidden client-side and still protected server-side.
- File previews remain iframe-backed where useful, but core dashboard pages are native DOM.
- `go test ./...`, `go build ./...`, and JS syntax checks pass.
