# Hank App Singleton Home Checklist

This document turns the singleton Home backend changes in `hankserverside` into concrete Hank app work.

Use [SERVER_SYNC.md](/Users/aaroncampbell/Documents/HankServerside/SERVER_SYNC.md) as the shared contract ledger. Use this file to implement the Hank side.

## Summary

Hank Remote is no longer a multi-home system. The app now needs to treat Remote as one deployment-scoped Home with:
- one singleton `Home`
- one connected agent surface
- `admin` and `member` roles
- Home-wide permission defaults
- per-member permission overrides
- singleton shared-note, sync, file, and service-profile routes

## Assumptions

- This is a clean break from the old `/v1/homes/...` contract.
- Hank should not keep a home picker or selected-home state for Remote.
- The first registered account auto-creates the Home on the server.
- The app should keep profile notes under `/v1/me/notes`.
- Shared Home notes stay under `/v1/home/notes`.
- Admin-only editing remains the rule for:
  - Home settings rename
  - member removal
  - role changes
  - Home permission edits
  - member permission edits
  - service-profile edits
  - agent token issuance and revocation

## Workstream 1: Remote Client Contract

### Replace Removed Multi-Home Endpoints

- Remove use of:
  - `GET /v1/homes`
  - `POST /v1/homes`
  - `GET /v1/homes/{homeID}`
  - `/v1/homes/{homeID}/...`
  - `GET /v1/agents`
- Add singleton Home clients for:
  - `GET /v1/home`
  - `PUT /v1/home`
  - `GET /v1/home/agent`
  - `GET /v1/home/agent/tokens`
  - `POST /v1/home/agent/tokens`
  - `DELETE /v1/home/agent/tokens/{tokenID}`
  - `POST /v1/home/invitations/accept`

### Update Shared Resource Clients

- Move shared-file transfer setup to:
  - `POST /v1/home/files/downloads`
  - `POST /v1/home/files/uploads`
- Move shared-note HTTP clients to:
  - `GET /v1/home/notes`
  - `GET /v1/home/notes/{noteID}`
  - `PUT /v1/home/notes/{noteID}`
  - `DELETE /v1/home/notes/{noteID}`
- Move sync-status loading to:
  - `GET /v1/home/sync`
- Move service-profile loading and saves to:
  - `GET /v1/home/service-profiles`
  - `PUT /v1/home/service-profiles/{serviceType}`
- Add admin-only server-storage clients if Hank exposes server operations:
  - `GET /v1/home/storage/status`
  - `GET /v1/home/storage/config`
  - `PUT /v1/home/storage/config`
  - `GET /v1/home/storage/events`
  - `POST /v1/home/storage/backup`
  - `POST /v1/home/storage/restore-test`
  - `POST /v1/home/storage/restore-primary`

### WebSocket Relay

- Stop sending `home_id` on `app.command`.
- Keep sending:
  - `request_id`
  - `command`
  - optional payload body
- Remove any selected-home dependency from the Remote WebSocket client and any request builders layered on top of it.

## Workstream 2: Remote Settings Shell

### Remove Home Selection

- Remove home list, home picker, and home-creation UI from Hank Remote settings.
- Replace them with a singleton Home settings surface that loads from `GET /v1/home`.
- Rename any “selected Home” local state to deployment Home state or remove it entirely.

### Home Summary

- Show:
  - Home name
  - current user role
  - agent status
  - sync status summary
- Allow admins to rename the Home with `PUT /v1/home`.

### Agent And Token Surface

- Keep the existing agent token issuance flow conceptually, but make it singleton:
  - no home picker
  - one Home
  - one visible agent surface
- Load:
  - `GET /v1/home/agent`
  - `GET /v1/home/agent/tokens`
- Save and revoke via the singleton token routes.

## Workstream 3: Members, Roles, And Invitations

### Roles

- Replace `owner` wording with `admin`.
- Replace owner-based local booleans and guards with admin-based ones.
- Assume roles are now:
  - `admin`
  - `member`

### Members Screen

- Load members from `GET /v1/home/members`.
- Show:
  - email
  - user ID if currently shown in Hank Remote
  - role
  - current-user marker
- Add admin-only member removal with:
  - `DELETE /v1/home/members/{userID}`

### Invitations

- Create invitations with:
  - `POST /v1/home/members/invitations`
- Accept invitations with:
  - `POST /v1/home/invitations/accept`
- Revoke invitations with:
  - `DELETE /v1/home/members/invitations/{invitationID}`
- Treat invitation creation as member-only. Do not expose role selection during invite creation.

### Role Changes

- Add admin-only role editing with:
  - `PUT /v1/home/members/{userID}/role`
- Support:
  - promote member to admin
  - demote admin to member
- Surface server-side last-admin protection errors clearly.

## Workstream 4: Home Permissions

### Home-Wide Defaults

- Load and edit:
  - `GET /v1/home/permissions`
  - `PUT /v1/home/permissions`
- The three Home-wide toggles are:
  - `homeassistant`
  - `files`
  - `notes`

### Per-Member Overrides

- Load and edit:
  - `GET /v1/home/members/{userID}/permissions`
  - `PUT /v1/home/members/{userID}/permissions`
- Each field is nullable:
  - `null` means inherit the Home default
  - `true` means allow
  - `false` means deny

### UI Expectations

- Admins need an editor for:
  - Home-wide toggles
  - per-member overrides
- Members should not see edit affordances for Home or member permissions.
- If the server denies a member action because of permissions, show a plain explanation tied to the affected feature:
  - Home Assistant disabled for this member
  - file access disabled for this member
  - shared-note access disabled for this member

## Workstream 5: Shared Home Notes

### Shared Notes Client

- Change shared-note list/detail/save/delete requests to the singleton `/v1/home/notes` routes.
- Keep profile notes under `/v1/me/notes`.
- Preserve:
  - `page_type`
  - `board`
  - `expected_revision`
  - conflict handling

### Shared Note Access Model

- Treat the server as authoritative for whether a member may access shared Home notes.
- Do not assume every member can access shared notes just because they are in the Home.
- Handle permission denial cleanly when the Home or member note permission is off.

### Collaboration

- Remove any app-side assumption that note collaboration requests need a selected `home_id`.
- Keep collaboration request routing anchored on note ID and the singleton Home connection.

## Workstream 6: Sync Health And Service Profiles

### Sync Health

- Load sync health from `GET /v1/home/sync`.
- Keep showing:
  - notes sync status
  - last manifest/pull/push timestamps
  - last successful sync
  - last successful backup
  - pending pull and push counts
  - last error
- Keep mapping status values into user-facing copy:
  - `healthy`
  - `degraded`
  - `out_of_sync`
  - `offline`
  - `pending`

### Service Profiles

- Load singleton service profiles from `GET /v1/home/service-profiles`.
- Save Home Assistant and SMB settings with `PUT /v1/home/service-profiles/{serviceType}`.
- Keep secret handling the same:
  - never redisplay stored secrets
  - send secrets only when saving
  - clear transient secret input after save
- Keep editing admin-only.
- Members may still view profile status if the app already exposes a read-only status surface.

## Workstream 7: Local State And Model Cleanup

- Remove any Remote cache partitioning keyed by selected `home_id`.
- Remove model assumptions that the current user owns the Home because `home.user_id == current user`.
- Treat membership as the access model and role as the management model.
- Remove old owner-only naming in models, view copy, and helper methods.
- Remove multi-home routing helpers and any code paths that derive URLs from a selected home ID.

## Validation

### Bootstrap And Basic Navigation

- Register the first Remote account and confirm the app can load `GET /v1/home` without a create step.
- Confirm there is no home picker in the Remote flow.
- Confirm the singleton Home rename path works for admins.

### Members And Roles

- Invite a second user.
- Accept the invitation from a second Hank account.
- Confirm the second user appears as `member`.
- Promote the second user to `admin`.
- Demote back to `member`.
- Confirm the server blocks removal or demotion of the last admin and Hank surfaces that clearly.

### Permissions

- Turn off Home-level `notes` permission and confirm a member loses shared-note access.
- Add a per-member `notes = true` override and confirm access returns.
- Turn off Home-level `files` permission and confirm file access fails for a member.
- Turn off Home-level `homeassistant` permission and confirm Home Assistant actions fail for a member.
- Confirm admins continue to work through all three features regardless of the toggles.

### Shared Notes, Sync, And Integrations

- Load shared notes over `/v1/home/notes`.
- Save a shared note and handle revision conflicts correctly.
- Delete a shared note over the singleton route.
- Load sync status over `/v1/home/sync`.
- Load service-profile status over `/v1/home/service-profiles`.
- Save Home Assistant and SMB settings as an admin.
- Confirm a member cannot edit service profiles.

### Relay

- Send WebSocket `app.command` messages without `home_id`.
- Verify Home Assistant, files, and notes commands still succeed for an admin.
- Verify permission-denied responses are handled cleanly for members.

## Done Criteria

- Hank no longer calls `/v1/homes`, `/v1/homes/{homeID}/...`, or `/v1/agents`.
- Hank no longer sends `home_id` on `app.command`.
- Hank uses `admin/member` terminology consistently.
- Hank has no Remote home picker or selected-home dependency.
- Hank can manage members, roles, permissions, sync status, service profiles, files, and shared notes through the singleton `/v1/home` contract.

## Workstream 8: Hank Assistant + OpenAI Link UX

### Assistant Session API Integration

Add app client support for assistant routes:
- `GET /v1/home/assistant/sessions`
- `POST /v1/home/assistant/sessions`
- `GET /v1/home/assistant/sessions/{sessionID}`
- `GET /v1/home/assistant/sessions/{sessionID}/messages`
- `POST /v1/home/assistant/sessions/{sessionID}/messages`
- `GET /v1/home/assistant/runs/{runID}`
- `POST /v1/home/assistant/runs/{runID}/confirm`
- `POST /v1/home/assistant/runs/{runID}/client-tool-results`
- `PUT /v1/home/assistant/calendar-index`

### Client Tool Bridge (Calendar)

For `waiting_client_tool` runs:
- execute requested EventKit tool locally (initially `calendar.create_event`)
- post normalized tool result to `/v1/home/assistant/runs/{runID}/client-tool-results`
- poll `/v1/home/assistant/runs/{runID}` until completed or next action required

### Confirmation UX

For `waiting_confirmation` runs:
- present clear approve/cancel UI with mutation details
- call `/v1/home/assistant/runs/{runID}/confirm` with explicit decision

### OpenAI OAuth Linking

Add app-side account linking UX wired to:
- `GET /v1/oauth/openai/start`
- server callback route `GET /v1/oauth/openai/callback`

Expected behavior:
- open `authorization_url`
- complete provider login/consent
- return to Hank and refresh assistant link status

### Validation Additions

- Verify assistant sessions list/create/send/reload works over app restarts.
- Verify a calendar-create prompt enters `waiting_client_tool`, executes EventKit call, and resumes to completion.
- Verify mutation confirmation flow blocks execution until user confirms.
- Verify OpenAI link flow completes and linked status persists across relaunch.

## Workstream 9: Server Storage Health

### Storage Status

- Load server storage health from `GET /v1/home/storage/status`.
- Show checksum state:
  - enabled or not enabled
  - last checksum check
  - last `pg_amcheck`
  - corruption flag
  - last error
- Highlight corruption when `checksum.corruption_detected == true`.
- Show recent failures from the `failures` list.
- Show current backup/restore work from `tasks`:
  - queued or running full backup
  - queued or running differential backup
  - queued or running restore test
  - queued or running primary restore
  - latest task step and updated time

### Backup Configuration

- Treat storage configuration as admin-only.
- Do not show storage pages, schedule controls, backup controls, or restore controls to non-admin members.
- Show and edit:
  - backup target type/path
  - full backup schedule
  - differential backup schedule
  - checksum check interval
  - restore verification schedule
  - retained full backup count
- Keep the app model target-typed so future external targets can be added without replacing the UI contract.

### Backup And Restore Actions

- Let admins request:
  - manual full/differential backup
  - restore test
  - primary restore
- Require the server-provided confirmation phrase before calling `POST /v1/home/storage/restore-primary`.
- Present primary restore as destructive and server-wide.

### Storage Notifications

- Subscribe to `storage.health` when the storage screen is active.
- Display only redacted event summary fields:
  - event id
  - operation
  - status
  - severity
  - message
  - backup label
- Never display storage notification details that include command output, database URLs, passwords, tokens, or backup encryption values.
- Handle these event names:
  - `storage.health.changed`
  - `storage.backup.failed`
  - `storage.checksum.corruption`
  - `storage.restore.started`
  - `storage.restore.completed`
  - `storage.restore.failed`

### Validation Additions

- Verify members cannot access storage routes.
- Verify members cannot open or use storage restore controls.
- Verify admins see checksum and backup logs.
- Verify corruption and backup failures are visually prominent.
- Verify restore-primary cannot be called unless the confirmation phrase matches.
