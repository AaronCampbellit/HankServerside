# First-Party App Platform Readiness Design

Date: 2026-06-14
Status: Implemented/superseded. Retained as historical design context; the current app platform contract and runtime behavior live in the codebase and active docs.

## TL;DR

HankServerside should become a stable first-party app host so new Hank chat
`/command` apps can usually be built, packaged, installed, configured, and used
without rebuilding HankServerside. The current `.hankapp` package runtime is a
strong base, but HankAI backend routing still knows specific apps such as
Hermes, Gramaton, and YDownload. The next readiness pass should add a generic
installed-app slash command lane, app-level user access, schema-backed
validation, and app-author documentation.

The security model is trusted first-party apps. We are not designing a
third-party marketplace or hostile-code sandbox. Guardrails should prevent
operator mistakes, broken packages, accidental privilege leaks, and contract
drift.

## Intent

Once HankServerside is stable, new first-party Hank apps should be installable
as `.hankapp` packages. A new app should add new Hank chat capabilities through
package metadata and a stdio runtime, not through a HankServerside rebuild, as
long as the app only needs capabilities already exposed by the app platform.

Core HankServerside should still evolve when a new app needs a genuinely new
platform capability, such as a new shared file API, durable job primitive,
assistant rendering type, or cloud-to-agent service contract.

## Current State

What already exists:

- Cloud package import, preview, activation, configuration, and app metadata
  persistence under `/v1/home/apps`.
- Agent-side `.hankapp` validation, extraction, installation, listing,
  configuration, and stdio invocation.
- Package manifest metadata for runtime command, slash commands, command
  summaries, settings schema, file permissions, network permissions, and secret
  fields.
- A package-driven Settings > Apps UI.
- A Hank chat command palette that reads installed app slash commands from
  `/v1/home/apps`.
- Existing first-party app packages for Hermes, Gramaton, and YDownload.

Main gaps:

- HankAI backend slash-command resolution still uses app-specific Go routing for
  known apps.
- App access is not modeled as an app-level setting that controls whether
  regular home members can use an installed app.
- Direct `apps.invoke` routing lacks a clear shared authorization path for app
  access.
- Manifest schema files are checked for existence, but config/input/output
  validation is not yet a platform-level contract.
- Generic app output rendering is not formalized enough for ordinary first-party
  apps to return text, cards, jobs, and diagnostics consistently.

## Product Rules

- Apps are trusted, first-party packages built for Hank.
- Admins install, preview, configure, enable, disable, update, and remove apps.
- Every installed app has one access setting:
  - `admins_only`: only home admins can see or use the app in Hank chat.
  - `home_members`: home admins and regular home members can see and use the app
    in Hank chat.
- Access is app-level, not command-level. If an app is available to users, all
  commands declared by that app are available to those users.
- The old per-command `admin_only` manifest field should not drive the future
  policy. It can remain temporarily for compatibility, but the app-level setting
  is the product decision point.
- New apps should use the generic app slash lane by default. App-specific Go
  routing should be reserved for workflows that need special cloud behavior that
  cannot be expressed through the generic contract.

## Non-Goals

- No third-party marketplace trust model.
- No package signing or remote marketplace distribution in this pass.
- No OS-level sandboxing requirement for first-party packages.
- No multi-home or SaaS app isolation model.
- No per-command permission UI.
- No direct SMB exposure, VPN requirement, or app-side local protocol access.

## Architecture

The generic app flow should be:

1. Admin imports a `.hankapp` package in Settings > Apps.
2. Cloud stages package bytes and asks the connected agent to preview the
   package.
3. Agent validates the archive and manifest, then returns app metadata.
4. Admin activates and configures the app.
5. Cloud persists app metadata, including slash commands, command summaries,
   settings schema, enabled state, and app access mode.
6. Hank chat loads installed app slash commands from `/v1/home/apps`, filtered
   by the current user's app access.
7. User sends `/someapp text`.
8. Cloud resolves `/someapp` against installed app metadata, checks access, and
   sends `apps.invoke` to the agent.
9. Agent runs the installed app's stdio command with a standard request.
10. App returns a standard response.
11. Cloud renders the standard response into HankAI message content.

## App Access Model

Add an app-level access mode to the persisted app metadata and API response.

Suggested field:

```json
{
  "user_access": "admins_only"
}
```

Allowed values:

- `admins_only`
- `home_members`

Default:

- New installs default to `admins_only`.
- Admins can switch to `home_members` from Settings > Apps.
- Existing installed apps migrate or load as `admins_only` unless an operator
  changes them.

Cloud enforcement:

- Admins can always use enabled apps.
- Members can use an enabled app only when `user_access` is `home_members`.
- Members should not see unavailable app slash commands in Hank chat.
- Direct WebSocket `apps.invoke` requests must use the same access check as
  HankAI slash routing.

Agent enforcement:

- The agent can trust cloud authorization for user access.
- The agent still enforces app existence, enabled state, command existence, path
  containment, package validation, timeouts, and output limits.

## Generic Slash Resolver

HankAI should resolve explicit slash prompts in this order:

1. Installed app slash commands available to the current user.
2. Built-in Hank commands such as `/ha`, `/files`, `/notes`, `/append`,
   `/calendar`, `/docs`, and `/status`.
3. Existing non-slash intent handling.

Installed apps are checked first so a first-party package can intentionally own
its installed command name. Built-in names should remain reserved by validation
or conflict handling so packages cannot accidentally shadow core workflows
unless the product explicitly allows it later.

Resolution rules:

- Match the first token case-insensitively.
- Preserve the manifest command casing for display.
- Pass the remainder of the prompt as `raw_text`.
- Return a clear "not configured or not available" response when the app is not
  installed, disabled, or inaccessible to the user.
- Trace the resolved app ID, command ID, access mode, and elapsed time in the
  assistant trace log without recording secrets.

## Generic Invocation Request

For a generic slash command, cloud sends `apps.invoke` with:

```json
{
  "app_id": "example",
  "command_id": "run",
  "input": {
    "raw_text": "text after the slash command",
    "slash_command": "/example"
  },
  "context": {
    "home_id": "home_...",
    "user_id": "usr_...",
    "session_id": "asess_...",
    "role": "admin",
    "timezone": "America/Chicago",
    "device": {}
  }
}
```

The context object should include only values that are useful and safe for
first-party apps. It must not include raw session tokens, cookies, API keys,
agent tokens, OAuth secrets, or service credentials.

## Generic App Response

First-party apps should return this output shape unless they deliberately use a
specialized contract:

```json
{
  "text": "Human-readable response.",
  "cards": [],
  "job_id": "",
  "diagnostics": {}
}
```

Rules:

- `text` is the primary assistant message text.
- `cards` may contain supported HankAI result-card payloads.
- `job_id` may link to a platform or app job when a future generic job contract
  is available.
- `diagnostics` is for non-secret operator/debug metadata.
- Unknown fields are allowed so first-party apps can evolve without immediate
  HankServerside changes.

Cloud should render:

- `text` directly when present.
- supported `cards` through the existing HankAI renderer contract.
- a fallback response when the app returns an empty successful output.
- clear error messages for timeout, missing app, disabled app, access denied,
  invalid output, and invocation failure.

## Schema Validation

The platform should validate enough schema to prevent broken first-party
packages from installing or silently misbehaving.

Package preview validation:

- `app.json` is valid and uses the supported schema version.
- Runtime command is present and contained in the package.
- Slash command names are valid and do not conflict with reserved built-in
  commands.
- Referenced config, input, and output schema files exist.
- Settings schema field types, sources, defaults, and select options are valid.

Configuration validation:

- Settings submitted from the UI are validated against the app config schema
  before being sent to the agent when feasible.
- Secret fields are separated from public config and stay agent-side.
- File-source select fields use known Hank file source IDs.

Invocation validation:

- Generic slash invocation should validate the generated input against the
  command input schema when the schema can be interpreted locally.
- App output should be validated against the command output schema when the
  schema can be interpreted locally.
- Schema validation failures should be surfaced as package or invocation
  contract errors, not as generic bad gateway errors.

## Settings > Apps UI

Settings > Apps should remain the operator surface.

Required controls:

- Install App button at the top of the page.
- Package picker modal.
- Preview before activation.
- Installed app dropdown.
- In-place selected-app configuration panel.
- Enable/disable toggle.
- App access toggle:
  - `Admins only`
  - `Home members`
- Package-driven settings fields from `config.settings_schema`.
- Secret fields displayed only as set/not-set metadata.

Member behavior:

- Regular members do not access the Settings > Apps admin page.
- Regular members see only accessible installed app slash commands in Hank chat.

## Compatibility

The current Hermes, Gramaton, and YDownload app packages should keep working.

Recommended transition:

- Add generic slash routing and app access while keeping existing specialized
  Hermes, Gramaton, and YDownload code paths as compatibility paths.
- Move simple explicit slash commands onto the generic path first.
- Keep specialized Gramaton media workflow code only where it needs existing
  media cards, download planning, confirmation, or job behavior.
- Remove or shrink app-specific routes only after tests prove generic parity.

## Tests

Add or update focused tests for:

- App metadata persistence includes `user_access`.
- New installs default to `admins_only`.
- Admin can update app access from Settings > Apps API.
- Members see app slash commands only for apps with `home_members` access.
- Members cannot invoke `admins_only` apps through HankAI slash prompts.
- Members cannot bypass the UI by sending direct WebSocket `apps.invoke`.
- Admins can use installed apps regardless of app access mode.
- Generic slash command invokes the correct `app_id` and `command_id`.
- Generic invocation sends safe context and raw text.
- Generic response text renders in HankAI.
- Empty output, timeout, app disabled, app missing, access denied, and invalid
  output produce useful assistant responses.
- Built-in command names cannot be accidentally shadowed by app packages.
- Existing Hermes, Gramaton, and YDownload tests still pass.

Validation commands for implementation:

```bash
gofmt -w ./cmd ./internal
go build ./...
go test ./...
```

Database-impacting implementation should also run:

```bash
make migrate-status
make schema-drift-check
```

## Rollout

1. Add app access metadata and migration.
2. Add access-aware app listing for the current user.
3. Add Settings > Apps access toggle.
4. Add shared cloud authorization for `apps.invoke`.
5. Add generic HankAI installed-app slash resolver and renderer.
6. Add schema validation improvements.
7. Update app-author docs and example package guidance.
8. Run full local validation.
9. Validate on the demo server with at least one admin-only app and one
   member-available app.

## Self-Review

- No unresolved placeholders remain.
- The scope is first-party app platform readiness only.
- The access model is app-level, not command-level.
- The design preserves single-home cloud-and-agent boundaries.
- The design does not add a VPN, public SMB exposure, third-party marketplace,
  or multi-home SaaS assumptions.
