# SMB Share Management Design

Date: 2026-07-13

## Goal

Make Settings > Connections the complete management surface for SMB file-server shares. Administrators can see every configured share, select one, edit it, test draft connection details, save it, add another share, or remove a share without affecting unrelated shares.

## Current Problem

The SMB service profile already supports multiple shares, but the dashboard initializes its editor from only the first share. Saving always rewrites that first entry, and the visible Add share button has no behavior. Other configured shares cannot be selected or managed. There is also no non-persisting connection test path.

## User Experience

The SMB panel contains a compact selectable list and one editor:

- Each list row shows the share label, `host/share`, and whether a password is configured.
- Selecting a row loads that share into the editor.
- Add share opens a blank draft with a collision-safe generated ID.
- Save validates the draft, updates or appends it by stable share ID, and preserves all other SMB shares and host-folder configuration.
- Test Connection validates the draft against the home agent without saving or changing the active configuration.
- Remove share requires confirmation, removes only the selected share, and then selects the next available share or an empty state.
- A blank password on an existing share preserves its current password. A password entered for a new or existing share is sent only for that share.
- View-only users can inspect configured public fields but cannot add, edit, test, save, or remove shares.

The existing active/default SMB source remains active when another share is edited. If the active share is removed, the first remaining share becomes active. No remaining shares produces an empty active source.

## Data Flow

The existing SMB service profile remains the canonical configuration. The dashboard parses the complete `shares` array into typed share records and keeps a selected share ID plus an editor draft.

Saving constructs the full public `shares` array by replacing the matching stable ID or appending a new share. It also retains unrelated SMB public configuration, including host folders and per-source policy. The existing `PUT /v1/home/service-profiles/smb` path continues to apply and optionally persist the complete configuration on the agent. Existing passwords are preserved agent-side by share ID when a save omits a password.

Removing sends the complete remaining share array through the same authenticated and CSRF-protected service-profile update path. The agent drops the removed share from its live and persisted configuration.

Testing adds an authenticated admin-only endpoint under the SMB service-profile surface and a versioned cloud-to-agent command. The request carries one draft share and its optional password. The agent creates an isolated SMB client, connects to the share, lists its root, closes the connection, and returns a small success or sanitized failure response. The test must not mutate the live file service, service profile, `.env.agent`, or database. Credentials must not be logged or included in the response.

## Validation And Errors

Save and test require a non-empty label, normalized host, share name, and stable ID. Passwords remain optional so guest-accessible shares continue to work. Duplicate IDs are rejected before sending a request.

The editor keeps the draft intact after a failed save or test. Success and error messages identify the selected share without exposing credentials. Removal confirmation names the share and explains that File Server access through that source will stop.

## Security And Compatibility

- Existing admin authorization and browser CSRF rules apply to all writes and tests.
- SMB credentials remain transient relay input and agent-owned persisted configuration; they are never stored in the cloud database or written to logs.
- Raw SMB access remains inside the home network and outbound-connected agent architecture.
- Existing single-share and legacy public-config shapes continue to load into the multi-share editor.
- No database migration is required.

## Testing

Frontend tests cover loading and selecting multiple shares, adding a draft, editing the selected share without changing others, preserving a blank existing password, removing only the selected share, active-source fallback, validation, and test-result rendering.

API client and cloud tests cover the test endpoint, admin authorization, CSRF behavior, offline-agent failure, request routing, and response sanitization. Protocol and agent tests cover draft decoding, successful root access, connection failure, cleanup, no live-config mutation, no persistence, and credential redaction. Existing config-apply and file-source tests continue to pass.

## Out Of Scope

- Reordering shares independently of selecting the active/default source
- Multi-home or multi-cloud-node behavior
- SMB discovery or network scanning
- Changing host-folder management
- Exposing SMB directly to the browser, cloud host, or public internet
