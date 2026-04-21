# Hank / hankserverside Sync

This file is the contract ledger for Hank app changes that must stay aligned with `hankserverside`.

Use it for:
- API route changes
- request or response shape changes
- auth or permission changes
- WebSocket envelope changes
- rollout and compatibility notes

If a change is app-only, do not add it here.

## Status Labels

- `planned`
- `in_progress`
- `blocked`
- `ready_to_ship`
- `shipped`

## [Single-Deployment Home Model]

- Status: in_progress
- Date Opened: 2026-04-19
- Owner: shared
- Related Ticket/PR: singleton-home refactor

### Summary

Hank Remote is now modeled as one deployment Home instead of multi-home tenancy. The first registered account auto-creates that Home and becomes an `admin`. Hank now needs to stop treating Remote as a home picker and instead treat it as one deployment-scoped Home with members, admins, permissions, notes, integrations, files, and one connected agent.

### Contract Update

- New singleton home routes:
  - `GET /v1/home`
  - `PUT /v1/home`
- Home membership and invitation routes:
  - `GET /v1/home/members`
  - `POST /v1/home/members/invitations`
  - `POST /v1/home/invitations/accept`
  - `DELETE /v1/home/members/{userID}`
  - `PUT /v1/home/members/{userID}/role`
- Home permission routes:
  - `GET /v1/home/permissions`
  - `PUT /v1/home/permissions`
  - `GET /v1/home/members/{userID}/permissions`
  - `PUT /v1/home/members/{userID}/permissions`
- Agent and token routes:
  - `GET /v1/home/agent`
  - `GET /v1/home/agent/tokens`
  - `POST /v1/home/agent/tokens`
  - `DELETE /v1/home/agent/tokens/{tokenID}`
- Shared-resource routes:
  - `POST /v1/home/files/downloads`
  - `POST /v1/home/files/uploads`
  - `GET /v1/home/notes`
  - `GET /v1/home/notes/{noteID}`
  - `PUT /v1/home/notes/{noteID}`
  - `DELETE /v1/home/notes/{noteID}`
  - `GET /v1/home/sync`
  - `GET /v1/home/service-profiles`
  - `PUT /v1/home/service-profiles/{serviceType}`
- First-user bootstrap:
  - the first successful `POST /v1/auth/register` auto-creates the singleton Home
  - the default Home name is `Home`
- Roles:
  - `admin`
  - `member`
- The old multi-home routes are removed:
  - `/v1/homes`
  - `/v1/homes/{homeID}/...`
  - `/v1/agents`

### Hank Changes

- Remove Remote home list, home creation, and home selection flows.
- After auth, load the singleton Home from `GET /v1/home`.
- Replace `owner` wording and local role checks with `admin`.
- Move Remote settings, membership, permissions, sync health, integrations, files, and shared notes to a singleton Home model.
- Stop storing selected-home UI state for Remote.

### hankserverside Changes

- Singleton Home routing is implemented.
- First-user bootstrap is implemented.
- Multi-home startup is rejected server-side.
- `owner` memberships are migrated to `admin`.

### Rollout Order

- server first, then app

Details:
- This is a clean contract break. Hank should switch directly to the singleton routes instead of trying to support both shapes at once.

### Fallback Behavior

- What happens if Hank ships first?
  - Calls to singleton routes fail against older servers.
- What happens if `hankserverside` ships first?
  - Existing Hank builds that still call `/v1/homes` and related home-scoped routes break until Hank adopts the singleton contract.

### Compatibility Notes

- Minimum Hank version: current Hank singleton-home client
- Minimum `hankserverside` version: current singleton-home branch
- Is this backward compatible?
  - No. This is a deliberate clean break from the multi-home contract.
- When can old behavior be removed?
  - Old behavior is already removed from `hankserverside`.

### Validation

- Register the first account and confirm `GET /v1/home` succeeds without a separate create step.
- Confirm the app no longer expects a home picker or home creation flow.
- Confirm the app reads one Home, one agent surface, and one token list.

## [App WebSocket Relay Without home_id]

- Status: in_progress
- Date Opened: 2026-04-19
- Owner: shared
- Related Ticket/PR: singleton-home refactor

### Summary

App WebSocket commands now route implicitly against the singleton Home. Hank should stop sending `home_id` on `app.command` envelopes.

### Contract Update

- Hank app WebSocket endpoint remains:
  - `GET /ws/app`
- Requests must now include:
  - `request_id`
  - `command`
  - optional JSON payload
- Requests must no longer require:
  - `home_id`
- The cloud resolves the authenticated user to the singleton Home automatically.

### Hank Changes

- Remove `home_id` from outgoing `app.command` envelopes.
- Remove any home-selection dependency from the remote WebSocket client.
- Keep request correlation by `request_id`.

### hankserverside Changes

- The app WebSocket now accepts singleton-home commands without `home_id`.
- Route authorization still checks authenticated membership in the singleton Home.

### Rollout Order

- server first, then app

Details:
- Hank should not try to send both with and without `home_id`. The target contract is the singleton envelope.

### Fallback Behavior

- What happens if Hank ships first?
  - Older servers reject commands because they still require `home_id`.
- What happens if `hankserverside` ships first?
  - Older Hank builds still sending `home_id` continue to include the field, but the clean target is to remove the field from Hank.

### Compatibility Notes

- Minimum Hank version: current Hank singleton-home relay client
- Minimum `hankserverside` version: current singleton-home branch
- Is this backward compatible?
  - Treat it as no. Hank should align to the new envelope contract.
- When can old behavior be removed?
  - Hank should remove `home_id` handling in its Remote client during this migration.

### Validation

- Send `system.ping` through `/ws/app` without `home_id`.
- Send one Home Assistant command, one file command, and one notes command without `home_id`.

## [Home Members, Roles, And Permissions]

- Status: in_progress
- Date Opened: 2026-04-19
- Owner: shared
- Related Ticket/PR: singleton-home refactor

### Summary

The singleton Home now supports `admin/member` roles plus Home-wide feature defaults and per-member overrides for `homeassistant`, `files`, and `notes`.

### Contract Update

- Membership routes:
  - `GET /v1/home/members`
  - `POST /v1/home/members/invitations`
  - `POST /v1/home/invitations/accept`
  - `DELETE /v1/home/members/{userID}`
  - `PUT /v1/home/members/{userID}/role`
- Permission routes:
  - `GET /v1/home/permissions`
  - `PUT /v1/home/permissions`
  - `GET /v1/home/members/{userID}/permissions`
  - `PUT /v1/home/members/{userID}/permissions`
- Permission model:
  - Home defaults: `homeassistant`, `files`, `notes`
  - Per-member overrides: nullable allow or deny for the same three features
  - `admin` bypasses feature toggles
- Admin-only management surfaces:
  - member removal
  - role changes
  - home permission edits
  - member permission edits
  - service-profile edits
  - agent token issuance and revocation
- Invitation creation now always creates `member` access. Promotion to `admin` is separate.

### Hank Changes

- Replace owner-only UI and logic with admin-only UI and logic.
- Add member role editing.
- Add Home permissions editor for `homeassistant`, `files`, and `notes`.
- Add per-member override editor for the same three features.
- Hide or disable admin-only actions for non-admin members.
- Make Remote settings explain that feature access can be denied by Home permissions even when the user is still a member.

### hankserverside Changes

- `admin/member` roles are implemented.
- Last-admin protection is implemented for removal and demotion.
- Home defaults and per-member overrides are implemented.

### Rollout Order

- server first, then app

Details:
- Hank should switch its role and permission UI together so the new admin/member vocabulary is consistent.

### Fallback Behavior

- What happens if Hank ships first?
  - Permission and role-management calls fail against older servers.
- What happens if `hankserverside` ships first?
  - Existing Hank builds still show owner-based or missing permission UI until the app is updated.

### Compatibility Notes

- Minimum Hank version: current Hank admin/member and permission UI
- Minimum `hankserverside` version: current singleton-home branch
- Is this backward compatible?
  - No. The permission and role model changed.
- When can old behavior be removed?
  - Remove owner naming and any selected-home assumptions from Hank during this migration.

### Validation

- Invite a second user and accept the invitation.
- Promote that user to `admin`, then demote back to `member`.
- Confirm the last admin cannot be removed or demoted.
- Confirm Home notes, Home Assistant, and file access are denied for a member when the corresponding Home permission is off.
- Confirm a per-member override can restore access while the Home default remains off.

## [Shared Home Notes, Sync Health, And Service Profiles]

- Status: in_progress
- Date Opened: 2026-04-19
- Owner: shared
- Related Ticket/PR: singleton-home refactor

### Summary

Shared Home notes, sync health, and integration settings are still the Remote collaboration model, but they now live under the singleton `/v1/home` contract and inherit the new permission system.

### Contract Update

- Shared Home notes:
  - `GET /v1/home/notes`
  - `GET /v1/home/notes/{noteID}`
  - `PUT /v1/home/notes/{noteID}`
  - `DELETE /v1/home/notes/{noteID}`
- Sync health:
  - `GET /v1/home/sync`
- Service profiles:
  - `GET /v1/home/service-profiles`
  - `PUT /v1/home/service-profiles/{serviceType}`
- Notes still support:
  - `page_type`
  - `board`
  - `expected_revision`
  - HTTP conflict responses
- Profile notes remain separate and unchanged:
  - `GET /v1/me/notes`
  - `POST /v1/me/notes`
  - `GET /v1/me/notes/{noteID}`
  - `PUT /v1/me/notes/{noteID}`
  - `DELETE /v1/me/notes/{noteID}`

### Hank Changes

- Change all shared Remote note HTTP clients from `/v1/homes/{homeID}/...` to `/v1/home/...`.
- Keep profile notes under `/v1/me/notes`.
- Change sync-status and service-profile clients to the singleton routes.
- Gate shared notes, files, and Home Assistant actions on server-returned permission failures.
- Keep service-profile editing admin-only in the app.

### hankserverside Changes

- Singleton note, sync, and service-profile routes are implemented.
- Notes permission gating is enforced for non-admin members.
- Service-profile editing remains admin-only.

### Rollout Order

- server first, then app

Details:
- The route migration and role/permission migration should ship together in Hank so the Remote settings and Notes areas stay coherent.

### Fallback Behavior

- What happens if Hank ships first?
  - Shared-note, sync, and service-profile requests fail against older servers still on the multi-home routes.
- What happens if `hankserverside` ships first?
  - Existing Hank builds continue calling removed `/v1/homes/{homeID}/...` routes and break until updated.

### Compatibility Notes

- Minimum Hank version: current Hank singleton shared-note and settings client
- Minimum `hankserverside` version: current singleton-home branch
- Is this backward compatible?
  - No. The shared-resource route base changed.
- When can old behavior be removed?
  - Remove old shared-note and settings route code in Hank during this migration.

### Validation

- Load shared Home notes over `GET /v1/home/notes`.
- Save and delete shared Home notes over the singleton routes.
- Load sync health from `GET /v1/home/sync`.
- Load and update service profiles from the singleton routes.
- Confirm a non-admin member can view status but cannot edit service profiles.
