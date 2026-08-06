# App Folder Location Picker Design

Date: 2026-08-05

## Status

Approved in conversation for specification and implementation.

## Problem

Settings > Apps currently renders every non-boolean, non-secret app setting as
a plain text input. This means Hank does not honor two existing
`hank.app.v1` concepts:

- `select` fields with `source: "file_sources"` do not show configured SMB or
  local sources.
- `path` fields cannot browse folders and require operators to type paths.

Gramaton currently declares one `source_id` and two destination paths. The
requested behavior is more precise:

- Movie and TV destinations choose independent file sources or SMB shares.
- Each destination chooses an existing folder from its selected source.
- Folder navigation can continue to any depth supported by the source.
- The picker does not create, rename, or delete folders.

## Ownership

This change spans the stable platform contract and the Gramaton app:

- HankServerside owns the additive manifest metadata, validation, generic
  settings renderer, source discovery, and reusable folder-picker UI.
- Gramaton owns its movie/TV location fields, backward-compatible config
  interpretation, validation, and source-aware download behavior.

No Gramaton-specific settings panel will be added to HankServerside.

## Chosen Approach

Extend `hank.app.v1` path settings with an optional `source_field` property.

Example:

```json
{
  "key": "movie_destination_path",
  "label": "Movie folder",
  "type": "path",
  "source_field": "movie_source_id",
  "required": true
}
```

When a `path` field declares `source_field`, Hank renders it as an
existing-directory picker rooted at the selected file source. Existing `path`
fields without `source_field` keep their current text-input behavior. This is
an additive, backward-compatible contract change.

Alternatives rejected:

- A compound `location` value would make settings values non-scalar and create
  a larger compatibility change.
- A Gramaton-specific settings panel would duplicate File Server behavior and
  violate the manifest-driven app settings boundary.

## Platform Contract

### Schema

Add `source_field` to `AppSettingsField` in:

- `schemas/hank-app-v1.schema.json`
- `internal/protocol/apps.go`
- the dashboard `AppSettingsField` type
- package-author documentation

Manifest validation requires:

1. `source_field` is only present on a `path` field.
2. It matches the identifier format.
3. It references another field in the same settings schema.
4. The referenced field has `type: "select"` and
   `source: "file_sources"`.

Existing manifests remain valid without changes.

### Source Options

Settings > Apps loads configured primary-agent file sources using the existing
service-profile data. It includes enabled:

- SMB shares
- configured local/host folders

Worker-agent file targets are excluded because an installable app runs on the
primary home agent and cannot use a worker target merely because the dashboard
can browse it.

The generic renderer will also begin honoring static `select.options` and
dynamic `select` fields with `source: "file_sources"`.

### Folder Picker

For a path field with `source_field`:

- The source selector appears before the dependent path field.
- The path control displays the selected source-relative folder and a
  **Choose folder** action.
- Opening the picker starts at the saved folder when accessible, otherwise at
  the source root.
- Breadcrumbs allow returning to any ancestor.
- The list contains directories only.
- Selecting a directory navigates into it.
- **Use this folder** selects the current directory.
- The source root is selectable and is persisted as `/`.
- Navigation uses one `files.list` request per visited folder, so there is no
  artificial folder-depth limit.
- No recursive tree preload is performed.
- The picker cannot create, rename, move, or delete anything.

Changing a source clears its dependent path. A required path must then be
selected before saving.

### Error Behavior

- Missing or offline file agent: show an inline unavailable state and keep the
  settings form open.
- Source listing failure: show the safe agent error and allow retry.
- Saved folder no longer exists: start at the source root and require a new
  selection.
- Empty source: disable the folder chooser and explain that a source must be
  selected first.
- Files returned by `files.list` are ignored; only directories are rendered.

No new HTTP route, WebSocket command, or file mutation capability is needed.

## Gramaton Configuration

Gramaton adds these public settings:

```text
movie_source_id
movie_destination_path
tv_source_id
tv_destination_path
```

The manifest declares two `configured_source` permissions, one for each source
field. Each destination path references its corresponding source field.

The legacy `source_id` remains accepted by `schemas/config.schema.json` and the
runtime but is removed from the visible settings schema. Compatibility rules:

- `movie_source_id` falls back to legacy `source_id` when absent.
- `tv_source_id` falls back to legacy `source_id` when absent.
- Existing movie and TV destination paths remain unchanged.
- Once the new settings are saved, the explicit per-destination source fields
  take precedence.

No automatic destructive rewrite is performed during package replacement.

## Gramaton Runtime Behavior

The media service resolves a source independently for each media type:

- Movie plans, existence checks, downloads, partial files, and completion
  checks use `movie_source_id`.
- TV plans, existence checks, downloads, partial files, and completion checks
  use `tv_source_id`.

Settings validation verifies for both destinations:

1. The source exists and is available.
2. The selected path exists.
3. The selected path is a directory.

Root `/` is valid. Paths remain source-relative and continue through existing
path cleaning and containment checks. Agent source policies remain
authoritative for every later read and write; validation does not perform a
probe write or mutate the selected folder.

Job snapshots and status responses continue to identify the selected media and
destination. User-facing destination labels include both the source and folder
so Movie and TV locations are unambiguous.

## Data Flow

1. Hank loads the app manifest settings and primary-agent file sources.
2. The operator selects the Movie source.
3. Hank browses that source with `files.list` until the operator selects an
   existing folder.
4. The operator independently repeats the process for TV.
5. Hank submits all four scalar values through the existing app config route.
6. The agent invokes Gramaton's `settings_apply` validation.
7. Gramaton validates both locations and accepts or rejects the whole update.
8. Later media jobs select the source and path based on media type.

## Security

- Existing authentication, admin-role checks, CSRF handling, and app-config
  routes remain unchanged.
- Folder browsing uses the authenticated, home-scoped file command path.
- Agent file-source policies and path containment remain authoritative.
- The UI exposes folder names only after the operator already has app-management
  and file access.
- No SMB credentials, secrets, file contents, or raw request bodies are logged.
- The picker adds no file mutation operation.

## Database Impact

None. App public configuration remains JSON managed by the existing app runtime.
No migration, table, index, or backfill is required.

## Testing

### HankServerside

- Manifest schema and Go validator accept valid `source_field` relationships.
- Validator rejects missing, non-select, non-file-source, and non-path
  relationships.
- Existing manifests without `source_field` remain valid.
- Apps Settings renders dynamic file-source selects.
- Movie and TV selectors retain independent values.
- Changing one source clears only its dependent path.
- Folder picker shows directories only, navigates multiple nested levels,
  supports breadcrumbs and root, and persists the chosen path.
- Offline, missing-source, inaccessible-folder, and listing-error states remain
  recoverable.
- Existing text path fields keep the old renderer.

### Gramaton

- New source fields decode and appear in settings status.
- Legacy `source_id` populates both effective destinations.
- Movie file operations use only the Movie source.
- TV file operations use only the TV source.
- Settings validation rejects missing sources, missing paths, and non-directory
  paths.
- Root and deeply nested existing directories are accepted.
- Package tests, build, archive validation, and executable-bit validation pass.

### Demo Acceptance

Using only the Hank demo server:

1. Build and preview the updated `.hankapp`.
2. Replace the installed Gramaton package and confirm configuration
   preservation.
3. Configure Movie and TV to different demo shares and nested existing folders.
4. Reopen settings and confirm both selections persist.
5. Search for media, prepare a plan, confirm a legal test download, and verify
   the completed file lands only in the selected Movie source/folder.
6. Verify TV planning identifies the separate TV source/folder without
   downloading copyrighted material.
7. Run targeted tests, dashboard checks, package validation, `scripts/doctor.sh`,
   `/healthz`, `/readyz`, asset freshness checks, and relevant log review.

## Completion Criteria

- The source and folder controls are generic manifest-driven Hank controls.
- Movie and TV can target different primary-agent file sources.
- Operators can select any existing nested directory with no artificial depth
  limit.
- Legacy Gramaton configurations continue to function.
- The final `dist/gramaton.hankapp` is non-empty, contract-valid, installed on
  the demo, and copied back to the MacBook.
- End-to-end demo proof distinguishes package installation, configuration,
  search, planning, download completion, and file placement.
