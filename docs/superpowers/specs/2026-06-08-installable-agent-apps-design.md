# Installable Agent Apps Design

Date: 2026-06-08

Status: Runtime and Hermes implementation complete; Gramaton extraction pending follow-up implementation plan.

## Decision

Add an installable agent-app system for optional Hank workflows that should not require rebuilding HankServerside. The first target apps are Hermes and Gramaton. Core Hank features such as auth, Home Assistant, files, notes, dashboard, sync, storage, and the assistant shell remain built into Hank.

The first runtime uses agent-managed local executable packages. Each package is imported as a `.hankapp` bundle, validated by the home agent, configured through Settings, and invoked through generic app commands over the existing cloud-to-agent relay.

## Goals

- Install optional workflows without rebuilding or redeploying HankServerside.
- Make Hermes and Gramaton removable app packages instead of permanently compiled Hank features.
- Keep local credentials, local network access, and file-write authority inside the home agent.
- Add a Settings GUI surface for importing, validating, configuring, enabling, disabling, updating, and inspecting app packages.
- Define a concrete `.hankapp` package schema that is sufficient to build real Hermes and Gramaton app packages.
- Keep app failures isolated from the agent and from core Hank features.

## Non-Goals

- No public app marketplace in V1.
- No multi-home or SaaS app distribution model.
- No direct app access to Hank Cloud credentials, app sessions, or raw cloud database state.
- No generic unrestricted shell tool exposed to HankAI.
- No migration of core Hank features into app packages.
- No direct SMB exposure to the public internet.

## Product Boundary

Installable apps are for optional, workflow-like capabilities that can reasonably live outside the Hank server binary. The initial examples are:

- Hermes: explicit `/Hermes` chat bridge to a Hermes API server reachable from the home network.
- Gramaton: explicit `/gramaton` media search and download workflow.

Built-in Hank features remain compiled and supported directly:

- authentication and sessions
- home membership and permissions
- Home Assistant state and controls
- file browsing, transfers, and managed file jobs
- notes and note sync
- dashboard shell and Settings
- assistant session storage, model settings, confirmations, and retrieval
- backup, restore, and storage operations

## Runtime Architecture

The home agent gains an Agent App Runtime. Installed app bundles live under an agent-owned directory such as:

```text
/var/lib/hank/apps/
  hermes/
    app.json
    bin/hermes-app
    config.json
    schemas/
  gramaton/
    app.json
    bin/gramaton-app
    config.json
    schemas/
```

The agent discovers apps at startup and through a reload action. For each app, it validates the manifest, validates file permissions, loads the app config, and advertises enabled app capabilities to the cloud on heartbeat.

The cloud should not know Hermes or Gramaton as hard-coded command families long term. It should expose a generic app surface:

- `apps.list`
- `apps.config_status`
- `apps.config_apply`
- `apps.invoke`
- `apps.job_status`
- app event forwarding

The existing app/dashboard clients continue to talk to Hank Cloud over HTTPS and WebSocket. The cloud remains the authenticated relay and operator UI host. The agent owns app execution, secrets, local network access, and local file access.

## Package Format

An app package is a `.hankapp` archive. It is staged and validated before activation.

```text
hermes.hankapp
  app.json
  README.md
  bin/hermes-app
  assets/icon.png
  schemas/config.schema.json
  schemas/chat.input.schema.json
  schemas/chat.output.schema.json
```

The initial package schema is `hank.app.v1`.

```json
{
  "schema_version": "hank.app.v1",
  "id": "hermes",
  "name": "Hermes",
  "version": "1.0.0",
  "publisher": "Hank",
  "description": "Route explicit /Hermes prompts to a local Hermes API server.",
  "runtime": {
    "type": "stdio",
    "command": "bin/hermes-app",
    "args": []
  },
  "assistant": {
    "slash_commands": [
      {
        "command": "/Hermes",
        "command_id": "chat",
        "description": "Send a prompt to Hermes."
      }
    ]
  },
  "commands": [
    {
      "id": "chat",
      "mode": "request_response",
      "input_schema": "schemas/chat.input.schema.json",
      "output_schema": "schemas/chat.output.schema.json",
      "timeout_seconds": 120,
      "admin_only": true
    }
  ],
  "config": {
    "schema": "schemas/config.schema.json",
    "secret_fields": ["api_key"]
  },
  "permissions": {
    "network": [
      {
        "kind": "configured_base_url",
        "field": "api_base_url"
      }
    ],
    "files": [],
    "events": []
  }
}
```

Manifest validation should reject:

- unsupported `schema_version`
- missing or invalid `id`, `name`, `version`, or runtime
- app ids or command ids outside a strict lowercase identifier format
- command paths that escape the app directory
- executable files that are missing or unsafe
- unknown permission kinds
- slash commands that collide with built-in Hank commands or another enabled app
- schema references that escape the app directory
- archives with absolute paths, parent-directory paths, symlinks, or unsafe file modes

## Stdio App Protocol

V1 apps use line-delimited JSON over stdin/stdout. The agent starts the app process, sends one request, reads one response, and exits the process unless the runtime later supports a reusable worker mode.

Request:

```json
{
  "protocol_version": "hank.app.stdio.v1",
  "request_id": "req_123",
  "app_id": "hermes",
  "command_id": "chat",
  "config": {
    "api_base_url": "http://hermes-vm:8642",
    "model": "hermes-agent",
    "timeout_seconds": 120
  },
  "secrets": {
    "api_key": "redacted-agent-side-value"
  },
  "input": {
    "prompt": "what should I check next?",
    "conversation_id": "conv_123",
    "session_key": "sess_123"
  }
}
```

Response:

```json
{
  "request_id": "req_123",
  "ok": true,
  "output": {
    "text": "Hermes says check the API server.",
    "model": "hermes-agent",
    "response_id": "resp_123",
    "conversation_id": "conv_123"
  }
}
```

Error response:

```json
{
  "request_id": "req_123",
  "ok": false,
  "error": {
    "code": "upstream_unavailable",
    "message": "Hermes API returned 503"
  }
}
```

The agent must bound stdin/stdout size, stderr capture, process runtime, and response parsing. Raw secrets must not be logged.

## Command Flow

Assistant command flow:

1. User sends `/Hermes check this` or `/gramaton Project Hail Mary`.
2. Cloud resolves the slash command from the app registry.
3. Cloud sends `apps.invoke` to the agent with `app_id`, `command_id`, and input JSON.
4. Agent verifies the app is installed, enabled, healthy enough, and allowed for the user/action.
5. Agent starts the app executable and sends a stdio JSON request.
6. App returns a structured response or a job id for long-running work.
7. Agent wraps the response in the normal Hank protocol response.
8. Agent forwards app events for progress, completion, and failure.

Hermes uses request/response only. Gramaton uses request/response for search and planning, then job/event support for downloads.

## Settings GUI

Add a new Settings surface: `Settings > Apps`.

The Apps section should include:

- Installed apps list with name, version, enabled state, health, last error, capabilities, and running job count.
- Import app button that accepts `.hankapp` packages.
- Validation preview before install:
  - app name
  - publisher
  - version
  - requested permissions
  - slash commands
  - config fields
  - executable entrypoint
  - replacement or new-install status
- Install or replace action, admin-only.
- Enable and disable toggle.
- Configure action that renders public and secret fields from the app config schema.
- Reload apps action that asks the agent to rescan local app folders.
- Uninstall action that is blocked while jobs are running unless the admin explicitly cancels them first.

Upload must use staging. The server should not blindly unpack archives into the active app directory. The flow is:

1. Admin uploads `.hankapp`.
2. Cloud receives the package with a bounded size limit and stores it in a staging area or streams it to the online agent for staging.
3. Agent validates the package and returns an install preview.
4. Dashboard displays the preview and permission summary.
5. Admin approves install or replacement.
6. Agent activates the package atomically.
7. Agent advertises new capabilities on heartbeat.

If the agent is offline, Settings should show that app import requires the home agent to be online.

## Configuration And Secrets

Each app defines a JSON config schema and lists secret fields. Public config can be persisted in cloud service-profile style records if useful for dashboard rendering, but secret values must remain agent-side.

For V1, app config should be applied through the agent. The cloud may store redacted metadata such as:

- app id
- app version
- enabled state
- public config fields
- whether each secret field is set
- status, applied version, last error, and updated time

The agent persists secrets in its protected app config area or `.env.agent`-managed equivalent. App config files and extracted bundles should be mode `0600` for files and `0700` for directories where possible.

## Permissions

Apps must declare permissions in the manifest and the admin must approve them during import or enablement.

Initial permission kinds:

- `network.configured_base_url`: app may call only a URL from an app config field.
- `files.read_source`: app may read from selected file source ids.
- `files.write_destination`: app may write to selected file source ids and destination paths.
- `events.publish`: app may publish declared event topics.
- `assistant.admin_only`: app slash command requires home admin role.

The agent enforces permissions before launching an app where possible and constrains the request data sent to the app. Apps should not receive raw access to every SMB share unless the manifest and app settings explicitly allow it.

## Hermes Package Verification

Hermes fits the V1 schema cleanly.

Hermes config:

- `api_base_url`
- `api_key`
- `model`
- `timeout_seconds`

Hermes command:

- `chat`

Hermes input:

- `prompt`
- optional `conversation_id`
- optional `session_key`

Hermes output:

- `text`
- optional `model`
- optional `response_id`
- optional `conversation_id`

Hermes permissions:

- outbound network only to configured `api_base_url`
- no file permissions
- no event permissions
- admin-only assistant command

Hermes does not need persistent jobs, progress events, file access, or cancellation in V1. The package runtime can replace the current compiled `hermes.chat` path after a compatibility period.

## Gramaton Package Requirements

Gramaton should move after Hermes because it needs the broader app runtime.

Gramaton config:

- provider base URL
- username
- password or provider secret
- selected source id
- movie destination path
- TV destination path
- preferred quality
- require confirmation

Gramaton commands:

- `search`
- `plan_download`
- `download_start`
- `download_status`
- `download_cancel`
- `download_jobs`
- `image_fetch`

Gramaton outputs:

- search result cards with title, year, type, summary, rating, genres, poster URL, page path, and search text
- download plans with item list, destination, existing counts, missing-link counts, and confirmation requirement
- download job status with progress, bytes, current path, completed path, mode, verification status, and error message

Gramaton permissions:

- network to configured provider base URL
- read/write only to selected SMB source ids and destination paths
- publish `media.downloads` events

The package must preserve current media safety behavior:

- source-aware destination selection
- login and destination validation before saving settings
- ranged download verification with fallback to single-stream download
- no writes outside configured source and destination policy
- progress events for long downloads
- cancellation support

## Install, Update, And Uninstall

Install:

1. Package is staged.
2. Manifest and archive are validated.
3. Settings shows a permission preview.
4. Admin approves.
5. Agent activates the package.
6. App starts disabled unless the admin explicitly enables it.

Update:

1. New package version is staged next to the active version.
2. Agent validates it before switching.
3. Running jobs keep using the executable version they started with when the job is represented by a live app process. If the job is represented only by persisted status and later polling, the agent records the app version in the job state and routes follow-up status or cancel calls to the matching active or archived app version.
4. New invocations use the new version after activation.
5. Failed validation keeps the previous version active.

Uninstall:

1. Admin disables the app.
2. Agent stops accepting new invocations.
3. Running jobs finish or are cancelled.
4. Agent removes the active bundle.
5. Cloud hides slash commands once the agent no longer advertises them.

## Failure Handling

- App missing: slash command returns that the app is not installed or enabled.
- Agent offline: import/configure/invoke flows show that the home agent must reconnect.
- App process fails to start: mark app degraded and show a sanitized executable error in Settings.
- App times out: return a structured timeout error and record a workflow log event.
- App crashes mid-command: record app id, command id, exit code, and sanitized stderr excerpt.
- Bad app JSON response: reject it with `invalid_app_response`.
- Permission violation: block before app launch when possible.
- Gramaton job crash: job moves to failed, progress UI shows the last known item, retry starts a new app invocation.
- Hermes failure: return a simple failed response; no persistent job is needed.

Installable apps must not crash the agent, leak credentials, expose local services publicly, or corrupt built-in file, notes, Home Assistant, or storage behavior.

## Migration Plan

### Phase 1: Runtime First

Add the generic app runtime while keeping existing compiled Hermes and Gramaton behavior. The agent can discover apps, validate manifests, invoke stdio commands, expose app status, and advertise app capabilities. No user-facing workflow changes are required in this phase.

### Phase 2: Hermes App

Build `hermes.hankapp` and route `/Hermes` through the app runtime when installed and enabled. Keep the compiled `hermes.chat` path as a compatibility fallback for one release. After validation, remove the compiled Hermes service and Hermes service-profile special case.

### Phase 3: Gramaton App

Build `gramaton.hankapp` after job and event support is available in the app runtime. Move Gramaton search, planning, download jobs, progress, cancellation, image fetch, and media settings into the package. Keep compatibility fallback until app-based downloads are proven.

## Data Model

The implementation needs new store models for cloud-visible app metadata:

- installed app id
- version
- enabled state
- public config JSON
- secret-set metadata
- status
- applied version
- last error
- updated by
- updated at

The exact table layout should be decided in the implementation plan. Database work must use versioned migrations and schema checks.

Agent-local storage should track:

- active package path
- staged package path
- package version
- app config
- app secrets
- running jobs
- last validation result

## Security Requirements

- Every app-management route is authenticated.
- Import, install, replace, enable, disable, configure, and uninstall are admin-only.
- Cookie-authenticated writes use CSRF protection.
- Package upload size is bounded.
- Archive extraction rejects absolute paths, parent-directory paths, symlinks, unsafe modes, duplicate paths, and unsupported file types.
- App manifests use strict allowlists.
- App executable paths must stay inside the extracted app directory.
- Raw secrets never appear in cloud logs, app previews, workflow traces, or dashboard responses.
- Stderr excerpts are sanitized and bounded.
- App commands run with bounded time, stdout, stderr, and memory where the platform allows.
- Gramaton file writes remain source-aware and path-contained.
- App slash commands are permission-checked server-side before routing and agent-side before launch.

## Testing Plan

Runtime tests:

- Manifest validation accepts valid Hermes and Gramaton fixtures.
- Manifest validation rejects unsafe ids, path traversal, symlinks, missing executables, unknown permissions, and command collisions.
- Stdio runtime handles success, app error, invalid JSON, timeout, oversized response, and process crash.
- Agent capability advertisement includes enabled app commands and excludes disabled apps.
- App invocation refuses disabled, missing, unauthorized, and permission-invalid commands.

Cloud/API tests:

- App import routes require admin.
- App import rejects member access.
- Upload preview does not activate a package.
- Install activation records app metadata and emits settings changed events.
- Assistant slash command registry includes installed app commands.
- `/Hermes` routes through `apps.invoke` when the Hermes app is installed and enabled.
- Compatibility fallback still works during the migration window.

Settings UI tests/checks:

- Apps section lists installed apps and health.
- Import preview renders app metadata, permissions, commands, and config fields.
- Enable/disable/configure actions call the expected endpoints.
- Secret fields are blank after save and display only set/unset metadata.
- JavaScript passes syntax checks.

Hermes app tests:

- Hermes package manifest validates.
- `chat` command sends the expected Hermes API request shape.
- Empty prompt returns a validation error.
- Non-2xx Hermes API response returns a structured app error.
- Successful response maps to Hank's existing Hermes response shape.

Gramaton app tests:

- Search returns current media card fields.
- Selection planning preserves the existing media-selection follow-up behavior.
- Settings validation logs into the provider and validates the selected destination source before saving.
- Download start creates a job and publishes progress.
- Ranged download verification falls back to single-stream download on probe or verification failure.
- File writes cannot escape the configured destination source and path.
- Cancel marks the job cancelled and stops further writes.

## Acceptance Criteria

- Hank can install a new `.hankapp` package without rebuilding HankServerside.
- Settings can import, preview, install, configure, enable, disable, inspect, and uninstall app packages.
- Hermes works entirely from an installed Hermes package.
- Gramaton works entirely from an installed Gramaton package after the job/event runtime is complete.
- App package validation blocks unsafe manifests and archives.
- Secrets remain agent-side.
- App crashes do not crash the agent.
- Gramaton cannot write outside its approved destination/source policy.
- Core Hank features remain built in and do not depend on the app runtime.
