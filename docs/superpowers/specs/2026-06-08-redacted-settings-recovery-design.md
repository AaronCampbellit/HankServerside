# Redacted Settings Recovery Design

Date: 2026-06-08

Status: Implemented. Retained as design context for the admin recovery export/import surface and redacted settings bundle behavior.

## Decision

Build an admin-only recovery workflow in the Hank Remote dashboard that exports as much rebuild-useful settings data as possible while leaving tokens, passwords, API keys, encryption keys, database credentials, and agent setup tokens blank. Importing the bundle should restore non-secret settings first, then prompt the operator for the missing secrets needed to activate each imported connection.

This is not a database backup, note backup, attachment backup, or raw `.env` dump. It is a GUI-managed settings recovery profile for rebuilding HankServerside cleanly without copying secrets into a portable file.

## Goals

- Let an operator export the current Hank Remote configuration shape from the dashboard.
- Restore home, profile, connection, assistant, storage, and dashboard settings after a rebuild.
- Preserve the current security model: local credentials remain with the home agent, cloud secrets are not exposed through a downloadable bundle, and SMB is never exposed directly.
- Make the import path operator-friendly by showing exactly which secrets must be re-entered.
- Use current Settings and service-profile flows instead of treating `.env.cloud` or `.env.agent` as the product API.

## Non-Goals

- Exporting raw `.env.cloud` or `.env.agent` with real secret values.
- Restoring app sessions, invite tokens, password reset tokens, notes data, note attachments, media files, backup archives, or database rows outside the settings surfaces.
- Supporting multi-home import/export behavior.
- Adding encrypted full-secret export in V1.
- Making the iPhone app speak raw SMB, Home Assistant, or other local protocols.

## Recovery Bundle Shape

The export should be a JSON document with a stable `schema_version` and enough metadata to support future compatibility checks.

```json
{
  "schema_version": 1,
  "exported_at": "2026-06-08T00:00:00Z",
  "product": "hank-remote",
  "home": {
    "name": "Home"
  },
  "settings": {
    "profile": {},
    "assistant": {},
    "storage": {},
    "quick_links": [],
    "dashboard": {}
  },
  "service_profiles": [
    {
      "service_type": "smb",
      "public_config": {
        "active_source_id": "media",
        "shares": [
          {
            "id": "media",
            "name": "Media",
            "host": "nas.local",
            "share": "Media",
            "domain": "WORKGROUP",
            "username": "aaron"
          }
        ]
      },
      "required_secrets": [
        {
          "id": "smb.media.password",
          "label": "Media SMB password",
          "kind": "password",
          "service_type": "smb",
          "target": {
            "share_id": "media",
            "field": "password"
          }
        }
      ]
    }
  ],
  "env_templates": {
    "cloud": {
      "HANK_REMOTE_CLOUD_ADDR": "127.0.0.1:18080",
      "HANK_REMOTE_CLOUD_DATABASE_URL": "",
      "HANK_REMOTE_SECRET_ENCRYPTION_KEY": "",
      "HANK_REMOTE_DB_OPS_INTENT_SECRET": "",
      "HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS": ""
    },
    "agent": {
      "HANK_REMOTE_AGENT_CLOUD_URL": "",
      "HANK_REMOTE_AGENT_ID": "",
      "HANK_REMOTE_AGENT_TOKEN": "",
      "HANK_REMOTE_AGENT_HOME_NAME": "Home"
    }
  },
  "warnings": [
    "Secrets are intentionally blank and must be re-entered during import or first setup."
  ]
}
```

The exact settings keys should be generated from current store models and existing API payloads. The schema should remain explicit rather than recursively copying every database row, because explicit export code is easier to audit for secret leakage.

## Export Contents

V1 should include these non-secret values when available:

- Deployment home name and other safe home-level settings.
- User profile settings from `/v1/me/profile`, including dashboard layout and app profile preferences.
- Assistant settings that are stored in the database, such as enabled sources, model names, provider selection, prompt profile, planner flags, Ollama URL, and timeout values.
- Storage and backup configuration that is already surfaced in Settings, excluding passphrases and credentials.
- Quick links and other dashboard operator settings that are safe to show in the GUI.
- Service-profile public config for Home Assistant, SMB, Hermes, media workflow settings, and future connection types.
- Environment templates with known non-secret values and blank secret placeholders where useful for rebuild guidance.

V1 must blank or omit these values:

- Home Assistant tokens.
- SMB passwords.
- Hermes API keys.
- Media provider passwords, API keys, cookies, or bearer tokens.
- OpenAI API keys, OAuth/device-code secrets, APNS tokens, and any other cloud-side API secrets.
- `HANK_REMOTE_SECRET_ENCRYPTION_KEY`.
- `HANK_REMOTE_CLOUD_DATABASE_URL` password-bearing values.
- `HANK_REMOTE_DB_OPS_INTENT_SECRET`.
- `HANK_REMOTE_DB_OPS_REPO_CIPHER_PASS`.
- `HANK_REMOTE_AGENT_TOKEN`.
- Session, invitation, password reset, action-token, transfer-token, or WebSocket ticket material.

## Import Workflow

The GUI should expose this as a new `Settings > Recovery` tab. Backup and restore operations stay under `Settings > Backups`; the recovery tab is for rebuilding settings after a clean HankServerside setup.

1. The admin uploads a recovery bundle.
2. The server validates `product`, `schema_version`, JSON shape, and size.
3. The dashboard shows an import preview:
   - settings that will be added,
   - settings that will be updated,
   - settings that are unchanged,
   - unsupported bundle entries,
   - required secrets that are blank.
4. The admin confirms the non-secret import.
5. The server applies safe DB-backed settings and service-profile public config.
6. Service profiles that require missing secrets are left pending with a clear status such as `pending` and a last error like `secret required`.
7. The wizard prompts for each required secret.
8. Secret submission uses the existing service-profile apply path so the online agent receives `config.apply` and persists to `.env.agent` only when the operator chooses the existing save-on-connector behavior.
9. The final screen shows connection status and tells the operator to run the normal post-setup validation path, including `scripts/doctor.sh`.

The import path should not silently overwrite live settings. The preview and final apply request should require explicit admin confirmation.

## Backend Components

Add a focused recovery package or handler set under `internal/cloud` with these responsibilities:

- Build redacted recovery exports from store data and current service-profile public config.
- Classify service-profile settings and emit required-secret descriptors.
- Validate recovery bundles and produce import previews.
- Apply non-secret settings through store APIs.
- Leave service profiles pending when required secrets are missing.
- Never read or download raw runtime env files as the export source.

Suggested endpoints:

- `GET /v1/home/recovery/export`
  - Admin-only.
  - Returns `application/json` with a download filename.
- `POST /v1/home/recovery/import/preview`
  - Admin-only.
  - Accepts a recovery bundle and returns a diff plus required-secret checklist.
- `POST /v1/home/recovery/import/apply`
  - Admin-only.
  - Applies confirmed non-secret settings.
- Secret completion uses the existing `PUT /v1/home/service-profiles/{serviceType}` endpoints in V1. Do not add a batch secret endpoint in the first implementation.

## Frontend Components

Add a Settings recovery surface with these states:

- Export card: explains that secrets are blank and provides the JSON download.
- Import upload: accepts a JSON bundle and calls preview.
- Preview: shows add/update/unchanged rows and the missing-secret checklist.
- Confirm import: applies the non-secret settings.
- Secret prompts: renders typed fields for Home Assistant token, SMB share passwords, Hermes API key, media provider secret fields, and any future service-profile descriptors.
- Completion: shows which connections are active, pending, or failed.

The UI should use current dashboard patterns: admin-only visibility, explicit status chips, selectable rows or cards for service profiles, and toast/status feedback only after the server confirms persistence.

## Security Requirements

- Every recovery route is authenticated and admin-only.
- Cookie-authenticated writes use the existing CSRF protection.
- The export builder uses typed allowlists for included fields and explicit secret-field omission.
- Tests must prove known secret fields do not appear in the exported JSON.
- The server does not log uploaded bundle contents, secret prompts, or env templates.
- Uploaded bundles are parsed in memory with a bounded size limit and are never stored unless a future operator setting explicitly asks for retention.
- Import validation rejects unknown `product`, unsupported `schema_version`, invalid JSON, and dangerous overlarge payloads.
- Secret prompts send values only through authenticated settings writes and do not persist them in browser local storage.

## Error Handling

- Invalid bundle: show a concise error and do not apply anything.
- Unsupported schema version: show the supported versions and stop.
- Non-secret apply failure: stop and show which setting failed.
- Agent offline during secret apply: keep imported public settings pending and show that the agent must reconnect before secrets can be applied.
- Missing secret: leave that connection pending rather than creating a misleading healthy status.
- Partial service-profile failure: preserve successfully imported settings and mark failed connection rows degraded or pending with the agent/store error.

## Testing Plan

Backend tests:

- Export requires admin role.
- Export includes service-profile public config but not tokens, passwords, API keys, or agent tokens.
- Export emits required-secret descriptors for configured Home Assistant, SMB, Hermes, and media settings.
- Import preview reports add/update/unchanged rows and required secrets without applying state.
- Import apply writes non-secret settings and leaves secret-backed service profiles pending.
- Import rejects unsupported schema version, invalid JSON, oversized payloads, and member-role access.

Frontend tests or checks:

- Recovery tab/panel is hidden for non-admins.
- Export button downloads JSON.
- Import preview renders missing secrets per service/share.
- Secret prompt submission calls the existing service-profile endpoints with the expected payload shape.
- JavaScript passes `node --check`.

Validation commands after implementation:

```bash
gofmt -w ./cmd ./internal
go build ./...
go test ./...
git diff --check
```

For deployment/setup-impacting work, run `scripts/doctor.sh` when the local environment supports it.

## Implementation Decisions

- Add a dedicated `Settings > Recovery` tab and keep backup scheduling, backup runs, and restore tests under `Settings > Backups`.
- The existing `/v1/me/profile-backup` endpoint is user-profile backup storage, not the same thing as this operator recovery export. It may inform schema shape, but the new recovery workflow should be home/admin-scoped.
- Existing `PUT /v1/home/service-profiles/{serviceType}` behavior already has the right agent-secret boundary. Reuse it for final secret application instead of inventing a second secret transport.
- The first implementation should avoid a database migration unless a durable import-history table is required. V1 can generate exports and previews on demand.
