# Hank App Repository Separation Checklist

Date: 2026-06-18

## Goal

Move optional first-party Hank apps out of `HankServerside` into independently
developed repositories while keeping `HankServerside` as the stable runtime,
installer, manifest validator, Settings > Apps renderer, and invocation
platform.

This checklist is complete when Hermes, Gramaton, and YDownload can be built,
tested, released, installed, configured, invoked, upgraded, and removed from
fresh HankServerside instances without app-specific source code living in this
repo.

## Scope Boundary

Keep these in `HankServerside`:

- account auth, sessions, roles, access modes, and CSRF behavior
- cloud/agent routing and protocol envelopes
- the generic app import, preview, activation, config, permission, and invoke
  runtime
- `.hankapp` manifest validation and package compatibility tests
- Settings > Apps schema rendering
- store/migration logic for installed app records, app config, and app secrets
- core Hank features: Home Assistant, files, notes, storage, backup, restore,
  dashboard infrastructure, and the assistant shell

Moved to sibling per-app repos under `/Volumes/CampbellDrive/Projects`:

- `hermes`
- `gramaton`
- `ydownload`
- app-specific tests, schemas, READMEs, and package scripts

## Extracted Repo Inventory

| App | Repo | Package script | Runtime binary |
| --- | --- | --- | --- |
| Hermes | `/Volumes/CampbellDrive/Projects/hermes` | `scripts/package-hermes-app.sh` | `bin/hank-app-hermes` |
| Gramaton | `/Volumes/CampbellDrive/Projects/gramaton` | `scripts/package-gramaton-app.sh` | `bin/hank-app-gramaton` |
| YDownload | `/Volumes/CampbellDrive/Projects/ydownload` | `scripts/package-ydownload-app.sh` | `bin/hank-app-ydownload` |

Runtime/platform files that stay in this repo:

- `internal/cloud/apps.go`
- `internal/cloud/ui/apps.js`
- `internal/cloud/ui/apps.html`
- `internal/protocol/apps.go`
- `internal/agent/apps/manifest.go`
- `internal/agent/apps/package.go`
- `internal/agent/apps/runner.go`
- `internal/agent/apps/manager.go`
- `internal/store/apps.go`
- `internal/migrations/sql/000010_agent_apps.up.sql`

## Task List

### Phase 1: Lock The Platform Contract

- [x] Document that `HankServerside` is the stable runtime and `.hankapp` is a
  compatibility surface in [`docs/hank-app-platform-contract.md`](../hank-app-platform-contract.md).
- [x] Inventory first-party apps and the runtime files that must remain in this
  repo.
- [x] Add a machine-readable manifest schema file for `hank.app.v1` in this
  repo, owned by the platform.
- [x] Add runtime tests that validate the manifest schema against Hermes,
  Gramaton, and YDownload fixture manifests.
- [ ] Add a package fixture test that imports a built `.hankapp` archive without
  relying on app source directories.
- [ ] Document the package ABI: required archive entries, executable path rules,
  schema file path rules, secret-preservation behavior, file-source binding, and
  access modes.
- [ ] Document a compatibility policy for future `hank.app.v2` changes.

### Phase 2: Prepare Extraction Repositories

- [x] Create `hank-app-hermes` with app source, schemas, tests, package script,
  README, and release workflow.
- [x] Create `hank-app-gramaton` with app source, schemas, tests, package
  script, README, and release workflow.
- [x] Create `hank-app-ydownload` with app source, schemas, tests, package
  script, README, and release workflow.
- [ ] Give each repo a small CI path that runs `go test ./...`, builds the
  Linux amd64 stdio binary, creates the `.hankapp`, and validates it against the
  HankServerside manifest schema.
- [ ] Standardize package output names:
  `hermes-<version>.hankapp`, `gramaton-<version>.hankapp`, and
  `ydownload-<version>.hankapp`.
- [ ] Standardize release metadata so HankServerside can display version,
  publisher, description, checksum, and compatibility information.

### Phase 3: Make HankServerside Consume Packages, Not Source Trees

- [x] Replace app-source package scripts in this repo with fixture-only scripts
  or remove them after external package builds are proven.
- [x] Move current app source out of `cmd/hank-app-*` after external repos build
  equivalent packages.
- [x] Move current app manifests, schemas, and app READMEs out of
  `packages/<app>` after equivalent external repos exist.
- [ ] Keep only minimal test fixtures under `testdata/apps/<app>` if runtime
  tests need stable package examples.
- [ ] Update `go build ./...` expectations so HankServerside no longer builds
  app binaries.
- [ ] Update docs that currently point at `scripts/package-*-app.sh` or
  `packages/<app>` to point at external app repos and release artifacts.

### Phase 4: Install And Update Flow

- [ ] Add or document a supported install path for local `.hankapp` upload or
  server-side package import.
- [ ] Add or document checksum verification for external app packages.
- [ ] Define upgrade behavior for reinstalling a newer package with the same app
  ID: config preservation, secret preservation, command replacement, and access
  mode preservation.
- [ ] Define downgrade behavior or explicitly block downgrades when the manifest
  version is lower than the installed version.
- [ ] Define uninstall behavior: app record removal, config/secret retention or
  deletion, command removal, and audit trail.
- [ ] Add operator docs for installing the three first-party apps on a fresh
  HankServerside instance.

### Phase 5: Runtime Compatibility And Security

- [ ] Confirm imported packages cannot escape their archive root during
  extraction.
- [ ] Confirm runtime command paths must resolve inside the package install
  directory.
- [ ] Confirm app invocations use configured permissions rather than implicit
  access to all file sources or network targets.
- [ ] Confirm `admins_only` and `home_members` access modes are enforced
  server-side for external packages.
- [ ] Confirm app config writes preserve existing secret values when password
  fields are left blank.
- [ ] Confirm package manifests cannot override core Hank command names or
  built-in assistant behavior.
- [ ] Confirm app stdout/stderr logging does not expose raw secrets.

### Phase 6: Demo And Fresh Instance Proof

- [ ] Build release `.hankapp` artifacts from all three external repos.
- [ ] Install Hermes on a fresh HankServerside instance and invoke its command.
- [ ] Install Gramaton on a fresh HankServerside instance and invoke search plus
  non-destructive job/status commands.
- [ ] Install YDownload on a fresh HankServerside instance and verify its
  file-source settings path includes SMB-backed sources.
- [ ] Validate Settings > Apps renders all three app settings schemas without
  app-specific UI code.
- [ ] Validate `apps.list`, `apps.config_status`, `apps.config_apply`, and
  `apps.invoke` against all three external packages.
- [ ] Run `scripts/doctor.sh` after install flow changes.
- [ ] Update the demo-server runbook with the external package install steps.

### Phase 7: Cleanup And Publish

- [x] Remove duplicated app source from `HankServerside` after external package
  installs are proven.
- [x] Remove stale package scripts from `HankServerside` or convert them into
  fixture builders that do not contain first-party app source assumptions.
- [ ] Keep a small compatibility fixture set for runtime regression tests.
- [ ] Update `README.md`, `docs/architecture.md`,
  [`docs/hank-app-platform-contract.md`](../hank-app-platform-contract.md), and
  [`docs/project-knowledge-index.md`](../project-knowledge-index.md).
- [ ] Run `gofmt -w ./cmd ./internal`.
- [ ] Run `go build ./...`.
- [ ] Run `go test ./...`.
- [ ] Run `make migrate-status` and `make schema-drift-check` if any app-store
  or migration behavior changed.
- [ ] Deploy to the demo server if the install path or runtime behavior changed.

## Recommended Extraction Order

1. Hermes first. It is the smallest app and proves the stdio runtime,
   slash-command mapping, network permission, secret config, and package release
   flow without file-source complexity.
2. YDownload second. It proves external packages can use the generic
   file-source settings contract, including SMB-backed sources.
3. Gramaton third. It is the broadest app and should move after the package,
   permissions, settings, and release paths are proven by smaller apps.

## Completion Criteria

The separation is complete when:

- `HankServerside` has no app-specific source directories under `cmd/hank-app-*`
  or package source directories under `packages/<app>`.
- all three first-party apps have independent repositories with reproducible
  `.hankapp` release artifacts.
- a fresh HankServerside instance can install the released packages through the
  supported app import path.
- Settings > Apps config, app permissions, slash commands, and app invocation
  all work from package metadata.
- runtime tests prove old valid `hank.app.v1` packages still import and invoke
  after the source split.
