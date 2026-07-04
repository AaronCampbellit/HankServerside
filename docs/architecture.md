# Architecture

## Goal

Allow Hank to work off the home network without requiring a separate VPN app.

## High-Level Design

```text
Hank iPhone App
    |
    | HTTPS / WebSocket
    v
Hank Remote Cloud
    |
    | persistent outbound WebSocket
    v
Hank Remote Agent (inside home network)
    |                |
    | local HTTP     | local SMB / filesystem adapters
    v                v
Home Assistant     NAS / TrueNAS
```

## Why This Direction

- The iPhone app gets one remote API surface.
- The home network needs only an outbound connection from the agent.
- SMB and Home Assistant stay private to the home LAN.
- We avoid protocol-specific networking tricks in the iOS app.

## Current System Shape

HankServerside is Hank's stable OS/runtime. It owns auth, routing, protocol, persistence, core cloud/agent capabilities, operator surfaces, and the generic installable-app runtime. Hank apps are installable first-party extensions for optional workflows that run on top of this runtime. The app/runtime boundary and `.hankapp` compatibility rules are documented in `docs/hank-app-platform-contract.md`.

### Cloud

- authenticate app clients
- authenticate agents
- maintain the singleton deployment home, membership, permissions, invitations, and agent tokens
- maintain routing from users/homes to the active outbound agent connection
- relay commands, streams, realtime events, and managed file-operation jobs
- serve the dashboard, Hank chat UI, Settings panes, profile notes, file browser, backup/restore controls, audit events, query telemetry, and setup status
- persist users, homes, sessions, notes, assistant state, file transfers, file jobs, storage operations, tokens, audit metadata, and operational state in Postgres
- store note attachment bytes on the cloud filesystem under the configured attachment root
- run lifecycle maintenance for expired credentials, transfer/job history, operational rows, assistant attachment metadata, and stale note attachment files

### Agent

- connect outbound to cloud
- expose local capabilities
- translate cloud commands into local Home Assistant, filesystem/SMB, note, media, Hermes, and config operations
- enforce local policy and capability checks
- hold local network credentials and apply service-profile settings sent from the cloud

### Protocol

- versioned envelope
- request/response correlation
- command and event schemas
- error format
- long-running file and media workflows report status through job/event payloads

### Installable Apps

- apps are optional first-party extensions, not replacements for core Hank services
- `.hankapp` packages are imported, validated, configured, and invoked through the generic app runtime
- Settings > Apps renders app configuration from package manifests rather than app-specific settings panes
- the app package format and `apps.*` commands are compatibility surfaces that require versioning or migration paths for breaking changes

## Operator Surfaces

- `/` and `/dashboard` provide first admin setup, home/agent status, token lifecycle, and operator troubleshooting.
- Browser UI source lives in `web/dashboard` as a React/Vite/TypeScript app; the production build is embedded from `internal/cloud/ui/react`.
- `/dashboard/settings/*` exposes Settings sections as direct authenticated routes. Dashboard navigation does not use iframe composition; only file preview content uses sandboxed iframes.
- `/dashboard/file-server` exposes source browsing, uploads/downloads, file moves, cancellation, retry, and rollback for managed jobs.
- `/dashboard/hank` exposes HankAI conversations, assistant model/provider settings, attachments, confirmations, media workflows, and client-tool result handling.
- `/dashboard/profile-notes` exposes user notes, note sharing, collaboration, and note attachment handling.
- `scripts/bootstrap-first-run.sh`, `scripts/doctor.sh`, `make migrate-status`, and `make schema-drift-check` are the setup and database-safety entry points.

There is no standalone PWA surface at this time. The `/pwa` route family and web app manifest are intentionally not served; `docs/PWA/current-scope.md` records the current scope and the conditions for any future mobile-web product work.

## Security Notes

- Never expose SMB directly to the internet.
- The agent should only connect outbound to cloud.
- Agent registration tokens should be rotated and revocable.
- App auth should be separate from agent auth.
- Cloud should not need raw SMB credentials if the agent owns local access.
