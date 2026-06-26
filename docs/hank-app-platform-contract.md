# Hank App Platform Contract

Date: 2026-06-18

## Decision

Treat `HankServerside` as Hank's stable OS/runtime. Treat Hank apps as installable first-party extensions that run on top of that runtime. Keep `.hankapp` compatibility strict so optional feature development can move quickly without destabilizing the core remote-access platform.

## Runtime Boundary

`HankServerside` owns the platform responsibilities:

- account auth, sessions, roles, permissions, and CSRF behavior
- the single-home cloud-and-agent model
- cloud/agent routing, WebSocket relay, and protocol envelopes
- Home Assistant, files, notes, media, backup, restore, storage operations, and dashboard infrastructure
- database migrations, schema drift checks, observability, audit, and recovery workflows
- the generic app import, configuration, permission, invocation, and Settings > Apps runtime

Hank apps own optional workflows:

- slash-command workflows
- local-home integrations that can be enabled, disabled, upgraded, or removed independently
- focused tools such as Hermes, Gramaton, or YDownload-style packages
- behavior that can fail without breaking auth, routing, files, notes, Home Assistant, backup, or the assistant shell

Do not move core Hank responsibilities into apps just to make feature development easier. If an app needs new stable platform capability, add that capability to the runtime contract first, then let apps consume it.

## Compatibility Contract

The `.hankapp` package format is a compatibility surface, not an internal implementation detail. Changes to the app runtime must preserve existing valid packages unless there is an explicit migration plan.

Compatibility-sensitive surfaces include:

- `schema_version` values such as `hank.app.v1`
- `schemas/hank-app-v1.schema.json`, which external app repos should use for
  local manifest validation
- manifest fields in `app.json`
- the app folder / `.hankapp` archive layout, including root-level `app.json`
- package archive validation rules
- command IDs, slash-command declarations, and reserved built-in command names
- `apps.list`, `apps.package_preview`, `apps.package_activate`, `apps.config_status`, `apps.config_apply`, and `apps.invoke`
- Settings > Apps schema rendering
- `config.settings_schema`, `secret_fields`, and secret-preservation behavior
- file-source bindings such as `source: "file_sources"` plus matching `permissions.files` entries
- access mode behavior for `admins_only` and `home_members`

Breaking changes require all of the following:

- a new schema version or compatibility adapter
- package/runtime tests for the old and new behavior
- docs updates for package authors and operators
- a migration or repackage path for existing first-party apps

## Development Rules

Use app packages for optional or experimental workflows when the core runtime already exposes the needed stable capability.

Use core runtime changes when work affects auth, routing, database shape, cloud/agent protocol, file safety, Home Assistant access, notes, backup/restore, assistant sessions, operator setup, or shared dashboard infrastructure.

Keep app-specific settings inside Settings > Apps through manifest-driven schema rendering. Do not add bespoke Settings panes for individual apps unless the platform contract is intentionally being expanded.

Keep first-party apps aligned with the contract. When the contract changes, update the runtime, manifest validation, package docs, package examples, and existing first-party packages together.

## Package Layout

Settings > Apps accepts either a prebuilt `.hankapp` archive or a selected app
folder. The app folder is packaged in the browser before upload. The selected
folder name is stripped while packaging, so `my-app/app.json` becomes
`app.json` at the archive root.

The folder or archive root must contain `app.json`:

```text
my-app/
  app.json
  bin/my-app
  schemas/config.schema.json
  schemas/run.input.schema.json
  schemas/run.output.schema.json
  README.md
```

`app.json` must use `schema_version: "hank.app.v1"`. `runtime.command` must be
a clean relative path to a file in the package, such as `bin/my-app`. Every
schema path referenced by `config.schema`, `commands[].input_schema`, or
`commands[].output_schema` must exist in the package and be a valid JSON object.

Package paths must be clean relative paths. Do not use absolute paths, `../`,
`.` path segments, double slashes, backslashes, Windows drive paths, symlinks,
or duplicate archive paths. Settings > Apps skips `.DS_Store` and `__MACOSX`
entries when packaging a folder.

The current `hank.app.v1` stdio runtime supports `runtime.type` and
`runtime.command`. It does not support `runtime.args`; package authors should
wrap arguments in the app executable or add runtime support before depending on
manifest-provided args.

## Authoring Validation

External app repositories should validate `app.json` against
`schemas/hank-app-v1.schema.json` before building a `.hankapp` archive. Passing
that JSON Schema is a package-author check, not a replacement for HankServerside
runtime validation. The runtime validator remains authoritative because it also
checks cross-field rules such as slash-command command references, reserved
commands, package path containment, default/option compatibility, and supported
permission semantics.

When using Codex to create or update an installable Hank app, use the
`hank-create-app` skill, shown as "Hank App Builder" in the local skill list.
That skill should leave the app with a non-empty `dist/<id>.hankapp` artifact
after running the app tests, build, and package script. Treat that archive as
the install-ready artifact for Settings > Apps. If packaging cannot run, the
app is not install-ready yet.
