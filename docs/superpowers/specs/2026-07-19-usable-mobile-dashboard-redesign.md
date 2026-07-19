# Usable Mobile Dashboard Redesign

Date: 2026-07-19
Status: Approved by user direction to proceed

## Objective

Turn the Hank Remote web GUI into a task-focused phone experience instead of a vertically stacked desktop dashboard. Preserve every existing route, permission, API contract, action, and desktop layout while making the daily mobile workflows fast enough to use one-handed.

## Evidence From The Live 430px Audit

- Home is 2,434px tall before all secondary content is exhausted. Two oversized setup actions dominate the first screen, four low-density metrics follow, and the actionable service state is pushed below the fold.
- Hank is functional, but nested header and conversation cards spend too much height on framing while the empty conversation body receives most of the screen.
- Notes shows the browser/list and editor simultaneously. The list consumes 427px before the editor begins, and the editor toolbar is 814px wide inside a 400px viewport.
- Home Assistant renders 80 entity cards at roughly 218px each, producing an 18,810px document. State, domain, and the add action receive equal visual weight even though name and state are the primary scanning fields.
- Files duplicates folders in a dedicated folder block and again in the results list. Each mobile result expands desktop metadata into several vertical lines, while transfer history remains in the same long document.
- A non-admin opening Settings lands on the admin-only Home & Connector section and sees only `admin role required`.
- The five-item bottom navigation and compact top bar are sound foundations and should remain.

Audit captures are stored in `.codex/mobile-redesign-audit-2026-07-19/`.

## Chosen Direction: Task-Focused Progressive Disclosure

Three approaches were considered:

1. **Task-focused progressive disclosure (chosen).** Keep the shared React routes, but give each route one primary mobile workspace. Move secondary data into compact summaries, tabs, sheets, or drill-in states. This produces the largest usability gain without duplicating backend or route behavior.
2. **Dense responsive restyling.** Keep every current section visible and reduce spacing. This is low risk, but it preserves the underlying information overload that caused the failed first pass.
3. **Separate mobile application shell and pages.** Build mobile-specific route implementations. This gives maximum freedom but duplicates logic, increases parity risk, and is not justified for the single-home operator product.

The chosen direction keeps one implementation per route and adds small mobile-only state where progressive disclosure cannot be achieved with CSS alone.

## Global Mobile Model

- Keep the existing top bar and five-item bottom navigation.
- Use 12px page gutters, 12px card radii, and the current Hank color, type, icon, border, and focus systems.
- Reduce decorative card nesting. A mobile page should usually have one surface boundary per meaningful object, not one around every section.
- Keep the current route title in the top bar. Page bodies should not repeat a large branded eyebrow unless it helps distinguish a mode.
- Prefer compact 52–64px rows for scannable collections. A tap opens detail or invokes an existing action.
- Keep primary actions visible and move infrequent, destructive, or administrative actions into labeled overflow controls.
- Maintain 44px touch targets and 16px form text.
- Preserve one document scroll surface. Local horizontal scrolling is allowed only for editor toolbars and Kanban columns, with visible affordances and snap behavior.

## Route Designs

### Home: Mobile Control Center

The first screen answers three questions: Is the home reachable, what needs attention, and what can I do now?

- Replace the large greeting with a compact header row containing `Home`, connector state, and last-seen time.
- Convert setup/restart prompts into a single prioritized alert card. Disabled actions are not rendered as giant empty buttons.
- Replace four separate metric cards with a two-column summary grid no taller than two rows.
- Show only services that require attention in the first service block, plus one compact `All services` disclosure for healthy rows.
- Move Quick Links and Home Assistant shortcuts above historical activity.
- Collapse Recent Activity and People to compact summaries with a `View all` affordance.

### Hank: Full-Height Conversation Workspace

- Remove the redundant outer HankAI card on mobile.
- Combine provider and ready state into a single compact status line.
- Keep Conversations as a sheet trigger.
- Size the conversation to the usable dynamic viewport between the top and bottom bars.
- Anchor the composer inside the conversation workspace and keep Send adjacent to the input when width permits.

### Notes: Master-Detail Navigation

- Default to the notes browser on entry.
- Selecting a note switches to the editor; the list is no longer stacked above the editor.
- The editor top row gains a clear `Back to notes` control, note title, save state, and overflow actions.
- Keep search and notebook filter in the browser mode only.
- Keep core formatting actions visible and move lower-frequency formatting controls into a `More formatting` disclosure. The toolbar must not silently clip controls.
- Kanban remains a horizontally snapping board, but its work bar becomes a compact sticky row.

### Home Assistant: Dashboard First, Dense Entity Browser

- Preserve saved dashboard tiles at the top.
- Replace the 218px entity cards with 64–76px rows showing name, entity ID/domain, current state, and one trailing action.
- Add a sticky search/filter bar above the entity list.
- Render a bounded initial result set with an explicit `Show more` action so hundreds of entities do not create an 18,000px document.
- A row remains semantically associated with its toggle/run/add action and retains the existing service-call behavior.

### Files: One Browser, Separate Activity

- Remove the duplicate mobile Folders block; folders and files share one result list.
- Use 60–72px rows: icon and name on the first line, size/type/modified summary on the second, actions at the trailing edge.
- Keep share, path, search, list/grid, upload, and new-folder controls in a compact sticky browser header.
- Move transfer history behind an `Activity` sheet or disclosure with an active-transfer count.
- Preserve grid mode and full-screen preview behavior.

### Settings: Permission-Aware Entry

- Resolve `/dashboard/settings` to the first visible section for the current role.
- A non-admin must never land on an admin-only screen through the generic Settings link.
- The mobile chooser shows only visible destinations and behaves as a full-width section menu.
- Permission errors remain available for direct unauthorized URLs, but include a clear route back to an allowed Settings section.

## Component Boundaries

- `DashboardHome` owns attention prioritization and compact service disclosure.
- `HankAIPage` owns the mobile conversation/list mode and viewport workspace.
- `ProfileNotesPage` owns explicit mobile browser/editor mode while preserving the desktop split view.
- `HomeAssistantPage` owns entity result limiting and show-more state.
- `FileServerPage` owns the mobile browser/activity mode and removes duplicated mobile-only folder presentation.
- `App` and `SettingsLayout` own permission-aware Settings entry and chooser behavior.
- The final mobile CSS section remains the authoritative responsive layer, but route behavior changes receive semantic classes instead of relying on positional selectors.

## State, Errors, And Permissions

- New disclosure state is local UI state only and does not change API payloads.
- Loading and error states occupy the same compact workspace geometry as loaded content.
- Existing auth, CSRF, role, confirmation, file-containment, and service-call behavior remains unchanged.
- Direct unauthorized Settings URLs still show the server-derived permission error; generic navigation chooses an allowed destination.

## Verification

- Add behavior tests for Notes browser/editor mode, Home Assistant result limiting, File activity disclosure, Home prioritization, and permission-aware Settings navigation.
- Add stylesheet contracts for dense HA and File rows, compact Home hierarchy, full-height Hank, and non-clipping Notes controls.
- Run `npm --prefix web/dashboard run check`.
- Compare fresh local 430px screenshots against the seven live audit captures, then validate 320px, 390px, 430px, 760px, and 768px boundaries.
- Confirm no document-level horizontal overflow, no primary action below an unnecessary full screen, and no list route that creates an unbounded multi-thousand-pixel document from default data.

## Security And Data Impact

Presentation and local disclosure state only. No backend routes, database schema, migrations, tokens, permissions, agent commands, file operations, or secret handling change.

## Acceptance Criteria

- Home exposes connection state and the highest-priority action in the first viewport without giant disabled controls.
- Notes shows either the browser or editor on mobile, never both stacked by default.
- Home Assistant's initial mobile entity document is bounded and its rows are under 80px tall.
- Files shows each folder once and keeps default item rows under 80px tall.
- Hank's composer stays reachable above the bottom navigation and software keyboard.
- Generic Settings navigation lands on a section allowed for the current user.
- All existing desktop behavior remains intact above 760px.
