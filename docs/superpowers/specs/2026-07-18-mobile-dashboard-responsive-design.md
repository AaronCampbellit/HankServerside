# Mobile Dashboard Responsive Design

Date: 2026-07-18
Status: Approved

## Objective

Make the existing Hank Remote dashboard fully usable on phones without creating a separate PWA or changing any backend contract. The same authenticated routes, data, actions, and visual language remain in place; the layout and interaction model adapt to narrow screens.

The pass covers the global shell, authentication, Home, HankAI, Home Assistant, Notes and Kanban, File Server, Agents, every Settings section, Setup Guide, dialogs, tables, editors, and empty, loading, error, and destructive-action states.

## Non-goals

- Do not restore the removed `/pwa` routes, manifest, service worker, offline cache, or install prompts.
- Do not change cloud, agent, protocol, database, authentication, authorization, or file-operation behavior.
- Do not redesign the desktop dashboard or replace the existing color, type, spacing, icon, and component language.
- Do not add a parallel mobile application or duplicate route implementation.
- Do not modify unrelated Kanban behavior or overwrite the existing uncommitted Kanban styling work.

## Current Problems

The live audit at the mobile breakpoint found one shell-level defect and several repeated page-level patterns:

- The desktop grid row remains active after the sidebar becomes a horizontal mobile row. The navigation row expands to most of the viewport before route content begins.
- Primary navigation is a horizontally scrolling desktop toolbar. Important destinations can be off-screen and the session controls are not discoverable.
- The shell consumes two sticky rows before content.
- Many interactive controls are 28–38 pixels tall instead of the 44-pixel mobile target.
- Notes exposes a dense editor toolbar and a wide Kanban board without a clear compact interaction model.
- Home Assistant and File Server preserve desktop table widths, clipping lower-priority columns or requiring hidden horizontal scrolling.
- Settings exposes a long horizontal section rail.
- Dialogs, preview panes, terminal surfaces, and editors do not consistently account for the software keyboard or mobile safe areas.
- Responsive rules are repeated across the stylesheet, making cascade order difficult to reason about and easy to regress.

## Responsive Architecture

Desktop behavior remains the default. Mobile behavior activates at `max-width: 760px`, matching the current shell breakpoint. The range immediately above the breakpoint remains a compact desktop/tablet layout and is tested at 768 pixels to catch boundary regressions.

The shell will expose three mobile-specific surfaces derived from the existing navigation configuration:

1. A compact top app bar with Hank identity or the current route title, global search, notifications, and a menu button.
2. A fixed bottom navigation bar for Home, Hank, Notes, Home Assistant, and File Server.
3. An accessible overflow menu for Agents, Settings, Setup Guide, connector/session status, account role, and sign out.

The desktop sidebar and top bar remain unchanged above the mobile breakpoint. Mobile and desktop navigation reuse the same route metadata, active-route calculation, icon treatment, prefetch callbacks, and client-side navigation handler.

On mobile:

- The shell uses explicit `auto minmax(0, 1fr)` grid rows so navigation cannot consume the viewport.
- The document has one vertical scrolling surface. Route pages do not create competing full-page scroll containers.
- Content receives bottom padding for the fixed navigation plus `env(safe-area-inset-bottom)`.
- The top bar and bottom bar account for `env(safe-area-inset-top)` and side safe areas.
- Global search opens as a full-width overlay below the top app bar.
- Notifications and the overflow menu are viewport-contained, keyboard dismissible, and close after navigation.
- Opening a mobile overlay prevents background scrolling and restores focus to its trigger when closed.

## Shared Mobile Rules

- Interactive targets are at least 44 by 44 CSS pixels unless they are inline text links with sufficient surrounding spacing.
- Inputs, selects, and textareas use at least 16-pixel text to avoid automatic mobile browser zoom.
- Page padding is 14–16 pixels; cards retain the existing radii, colors, borders, and typography.
- Primary actions remain visible; secondary and destructive actions move into labeled overflow menus when horizontal space is insufficient.
- Actionable data tables become labeled rows/cards. Dense read-only tables stay inside an explicit, visible local scroller. The document itself never overflows horizontally.
- Toolbars may scroll horizontally only when the controls remain discoverable and meet the touch-target requirement.
- Full-screen mobile dialogs use dynamic viewport units, safe-area padding, internal scrolling, and a visible close action.
- Hover-only affordances become persistently visible on coarse pointers.
- Focus visibility, semantic labels, Escape behavior, reduced motion, and screen-reader route/navigation names are preserved.

## Route Behavior

### Authentication and Account Flows

Login, join, password change, and first-run cards fill the available width with compact outer padding. Form controls and submit buttons use the mobile target height. Validation, recovery, and invitation messages wrap without horizontal overflow.

### Home

The hero, status metrics, services, activity, quick links, Home Assistant shortcuts, and people sections form one readable column. Hero actions stack or split evenly when two actions fit. Service status chips move below descriptive text instead of compressing it. Long user names and addresses wrap safely.

### HankAI

The conversation list and active conversation become a single-column experience. The conversation list is accessible from a compact control instead of permanently occupying horizontal space. The active conversation uses the available viewport height, and the composer stays visible above the software keyboard and bottom navigation. Attachments, slash-command results, confirmations, and status controls wrap without shrinking below touch size.

### Notes and Kanban

The notes rail becomes a collapsible stacked region that reuses the existing collapse state. The selected note editor occupies the full width. Title, notebook, save state, and destructive actions wrap in a predictable order.

The editor toolbar becomes a touch-sized horizontal control strip. The Kanban work bar wraps its search and primary actions, while columns use horizontal scrolling with snap alignment and a mobile-appropriate width. Card controls remain visible on touch devices. Card editing uses a full-screen mobile dialog with a scrolling body and anchored close/save actions.

### Home Assistant

Saved dashboard tiles form one column through 480 pixels and two columns from 481 through 760 pixels. Entity search and filters remain visible above results. The desktop entity table becomes labeled mobile rows that prioritize entity name, state, and the add/remove action; domain and entity ID remain available as secondary text. Toggle semantics and canonical state refresh behavior do not change.

### File Server

Share selection, breadcrumbs, search, upload, and folder creation wrap into clear rows. The folder rail becomes a compact full-width section. File results become touch-friendly mobile rows that prioritize name, type, size, selection, and actions; modified time remains secondary detail. Preview opens as a full-screen sheet at the mobile breakpoint rather than a narrow third pane. Transfer jobs become one-column cards with wrapping paths and status actions.

### Agents

Agent cards form one column. Status and resource metrics wrap without overlap. Agent detail, token tables, and interactive shell surfaces use full-width panels. Terminal sizing responds to dynamic viewport changes and keeps its controls reachable above the bottom navigation.

### Settings

The long Settings rail becomes a compact section chooser at the top of the page. Forms use one column, related button groups wrap, secret/token fields do not force overflow, and data-heavy tables use mobile rows or explicit local scrolling. Admin-only visibility and permission behavior remain server-driven and unchanged.

### Setup Guide

Long commands, URLs, tables, and code blocks scroll within their own containers. Step actions and callouts fit the mobile width without shrinking text.

## Component and Stylesheet Boundaries

- `Shell` owns mobile navigation, global overlays, safe-area structure, focus return, and menu state.
- Navigation configuration remains the single source for route labels and destinations.
- Route components add only the semantic hooks or data labels required for mobile transformation; they do not fork into separate mobile implementations.
- Shared primitives own minimum control heights, dialog sizing, button wrapping, and local overflow rules.
- Mobile overrides are consolidated near the end of the stylesheet in one intentional cascade layer or section. Obsolete duplicate mobile rules are removed only after confirming no current selector depends on them.

## Error and Loading States

Loading placeholders occupy the final mobile layout instead of shifting from desktop geometry after data arrives. Errors wrap within the content width and retain their retry action. Empty states keep their primary action visible. Confirmation dialogs name the affected item and retain the existing authorization, CSRF, and destructive-action gates.

## Verification

Automated coverage will assert:

- Mobile shell navigation visibility, active state, overflow menu contents, keyboard dismissal, and focus restoration.
- Desktop sidebar behavior remains intact.
- The mobile grid has an explicit navigation/content row structure.
- No fixed-width File Server or Home Assistant result surface forces document-level horizontal overflow.
- Notes and Kanban controls remain reachable and correctly labeled.
- Settings section navigation works at mobile width.
- Existing route, permission, authorization, and action tests continue to pass.

The responsive matrix is:

- 320 by 568
- 390 by 844
- 430 by 932
- 768 by 1024
- Phone landscape
- Coarse pointer
- Reduced motion
- Software-keyboard-sensitive editor/composer states where browser tooling permits

Repository validation uses `npm run check` in `web/dashboard`. Visual verification covers every top-level route plus representative Settings forms, tables, dialogs, Notes/Kanban editing, Home Assistant entities, File Server browsing/preview, and Agent detail at the responsive matrix above. The production Vite build is required because the served dashboard assets are generated from that build.

## Security and Data Impact

This is a presentation-only change. It adds no routes, writes, tokens, permissions, database columns, migrations, agent commands, or new secret handling. Existing authentication, authorization, CSRF, audit, file-containment, confirmation, and server-side permission behavior remain unchanged.

## Acceptance Criteria

- Every supported dashboard route is usable without document-level horizontal scrolling at 320 pixels wide.
- Primary navigation and route content are visible immediately, without a navigation row consuming most of the viewport.
- Daily destinations are one tap away from the bottom navigation.
- Secondary destinations, session state, and sign out are reachable from the mobile menu.
- Core actions have 44-pixel touch targets and remain keyboard accessible.
- Tables, editors, Kanban columns, previews, dialogs, and terminals have an explicit mobile interaction model.
- Desktop navigation and route layouts remain functionally unchanged.
- `npm run check` passes and browser verification finds no clipped content, unreachable action, or unexpected page-level horizontal overflow in the responsive matrix.
