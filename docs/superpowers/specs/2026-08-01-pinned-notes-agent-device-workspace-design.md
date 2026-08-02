# Synced Pinned Notes and Agent Device Workspace Design

**Status:** Approved on 2026-08-01

## Goal

Deliver two independent Hank Build improvements:

1. Let users pin notebook child notes so the same pinned ordering appears across browsers and devices.
2. Replace the long, fragmented Agents detail workflow with a focused device workspace that embeds Remote Desktop into the selected agent.

## Scope

The work stays inside `HankServerside`. It does not change Hankagent protocols, weaken Remote Desktop authorization, deploy the dashboard, or alter the single-home architecture.

## Synced Pinned Notes

### Persistence and contract

- Add `pinned BOOLEAN NOT NULL DEFAULT FALSE` to `user_notes` in migration `000027`.
- Add a database constraint that prevents root notes and notebooks from remaining pinned: a pinned record must have a non-empty `parent_id` and `page_type <> 'notebook'`.
- Carry `pinned` through `domain.UserNote`, store scans and writes, note collaboration state, revision materialization, and the existing Notes HTTP response/request types.
- Preserve backward compatibility: requests that omit `pinned` keep the current value during updates and default to `false` during creation.

### User experience

- Show a pin action on notebook child note rows and in the selected child note toolbar.
- Do not offer pinning for notebooks or root notes.
- In an explicitly selected notebook, show pinned children first. Within pinned and unpinned groups, retain the API’s existing ordering.
- Pin and unpin through the existing authenticated Notes read/write API. The dashboard fetches the complete note before saving a row action so body, board, revision, parent, and exclusion fields are preserved.
- On success, update the local summary and selected editor without requiring a reload. On conflict or failure, keep the previous state and show the existing error toast.

### Security and data behavior

- Existing Notes authentication, owner scoping, write scopes, CSRF handling, and optimistic revision checks remain authoritative.
- No new public route or permission is introduced.
- The migration is additive and backfills every existing note to `false`.

## Agent Device Workspace

### Navigation

- Keep `/dashboard/agents` as the device list and enrollment-token workspace.
- Add deep-linkable device routes:
  - `/dashboard/agents/{agentID}` opens Overview.
  - `/dashboard/agents/{agentID}/desktop` opens Remote Desktop and preserves the existing desktop deep link.
  - `/dashboard/agents/{agentID}/terminal` opens Terminal.
  - `/dashboard/agents/{agentID}/security` opens Security.
- Selecting a device navigates to its Overview route. The back action returns to `/dashboard/agents`.
- Desktop, Terminal, and Security routes remain admin-gated; Overview retains the current read-only behavior for non-admin users.

### Workspace structure

- The device header contains the device identity, type/platform, online state, and back navigation.
- A keyboard-accessible tab list exposes Overview, Remote Desktop, Terminal, and Security.
- Overview contains the metric summary, device details, and safe high-frequency actions such as lock, wake, and restart.
- Remote Desktop contains readiness, active-session controls, and the embedded viewer. If setup is incomplete, it shows the readiness explanation and links directly to Security.
- Terminal contains the existing audited `ShellConsole` and the current capability/online-state explanation.
- Security contains Remote Desktop trust/setup, capabilities and identifiers, and destructive device removal.
- Enrollment-token creation and revocation remain on the top-level Agents list, not inside a device.

### Remote Desktop integration

- Refactor `DesktopViewerPage` to support an embedded presentation while retaining its standalone behavior and tests.
- The embedded viewer uses the same access check, secure session creation, reconnect, encrypted socket, audit, and termination paths as the existing page.
- No Remote Desktop trust, authorization, local permission, or active-session check is bypassed.

### Responsive and accessible behavior

- Tabs use native buttons with tab roles, selected state, and keyboard focus.
- The workspace collapses to one column on narrow screens.
- Destructive actions retain confirmation dialogs and explicit labels.
- Status is communicated with text as well as color.

## Error Handling

- A missing device returns the user to an inline “device not found” state with a link to all devices.
- Readiness and trust errors stay scoped to their tabs and do not hide the Overview.
- A failed note pin operation restores the previous pin state and reports the API error.
- Existing unsupported-server and loading states remain intact.

## Verification

- Notes: migration definition, PostgreSQL store persistence when the test database is available, HTTP contract and authorization, API client serialization, pinned ordering, pin/unpin interaction, and failure rollback.
- Agents: route parsing, device deep links, tab semantics, content placement, admin gates, embedded viewer behavior, and preserved desktop lifecycle tests.
- Broad gates: frontend focused tests, full frontend tests/check/build, targeted Go tests, `go test ./...`, `go build ./...`, migration status, schema drift check, and `git diff --check`.

## Non-Goals

- Reordering pinned notes independently from normal note order.
- Pinning notebooks or root notes.
- Changing agent enrollment contracts.
- Changing Remote Desktop wire protocol or Hankagent behavior.
- Committing, pushing, or deploying without explicit user authorization.
