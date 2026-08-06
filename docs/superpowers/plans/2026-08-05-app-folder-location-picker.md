# App Folder Location Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:executing-plans` to implement this plan task-by-task. Subagents
> are prohibited for this repository task.

**Goal:** Add reusable, unlimited-depth existing-folder selection to
manifest-driven Hank app settings and let Gramaton use independent Movie and TV
sources and folders.

**Architecture:** Extend `hank.app.v1` with an additive `source_field`
relationship from a `path` field to a `select` field backed by
`file_sources`. HankServerside discovers primary-agent sources and provides a
generic directory-only browser over the existing `files.list` command.
Gramaton declares two source/path pairs and resolves file operations by media
type with legacy `source_id` fallback.

**Tech Stack:** Go 1.24+, JSON Schema draft 2020-12, React, TypeScript, Vite,
Vitest, Testing Library, JSON-over-WebSocket agent commands, ZIP `.hankapp`
packages.

## Global Constraints

- Preserve all unrelated changes in both working trees.
- Do not commit, push, tag, or publish without separate explicit authorization.
- Do not use subagents.
- Existing `hank.app.v1` packages without `source_field` remain valid.
- Only primary-agent SMB and local sources are eligible for installable apps.
- The picker selects existing directories only and has no artificial depth
  limit.
- Root is selectable and persists as `/`.
- No new file mutation operation, HTTP route, WebSocket command, or database
  migration is allowed.
- Gramaton retains legacy `source_id` fallback.
- Build, package, install, and end-to-end acceptance run on the Hank demo
  server; local work is limited to source edits and cheap static inspection.

---

### Task 1: Extend the app settings contract

**Files:**
- Modify: `internal/protocol/apps.go`
- Modify: `schemas/hank-app-v1.schema.json`
- Modify: `internal/agent/apps/manifest.go`
- Modify: `internal/agent/apps/manifest_test.go`
- Modify: `docs/hank-app-platform-contract.md`
- Modify: `/Users/aaroncampbell/.codex/skills/hank-create-app/references/hank-app-packages.md`

**Interfaces:**
- Produces: `AppSettingsField.SourceField string` serialized as
  `source_field`.
- Validation rule: a non-empty `source_field` belongs to a `path` field and
  references a `select` field whose source is `file_sources`.

- [ ] **Step 1: Write failing manifest validation tests**

Add table cases covering:

```go
{
    Key: "destination_path",
    Type: "path",
    SourceField: "source_id",
}
```

The valid fixture also includes:

```go
{
    Key: "source_id",
    Type: "select",
    Source: "file_sources",
}
```

Reject `source_field` on a text field, an unknown reference, a non-select
reference, and a select reference without `file_sources`.

- [ ] **Step 2: Run the focused tests on the demo worktree and confirm failure**

Run:

```bash
go test ./internal/agent/apps -run 'TestValidateManifest' -count=1
```

Expected: compile failure because `SourceField` does not exist.

- [ ] **Step 3: Implement the additive contract**

Add to the Go and TypeScript-compatible JSON contract:

```go
SourceField string `json:"source_field,omitempty"`
```

Add this JSON Schema property:

```json
"source_field": {
  "$ref": "#/$defs/identifier"
}
```

Perform relationship validation after all settings fields are indexed so
forward references work.

- [ ] **Step 4: Run contract tests**

Run:

```bash
go test ./internal/agent/apps -count=1
```

Expected: PASS.

- [ ] **Step 5: Update contract documentation**

Document that `source_field` is optional, directory-only, and leaves old
free-text `path` fields unchanged. Update the app-builder reference with the
two-field example.

### Task 2: Build the reusable directory picker

**Files:**
- Create: `web/dashboard/src/settings/AppFolderPicker.tsx`
- Create: `web/dashboard/src/settings/AppFolderPicker.test.tsx`
- Modify: `web/dashboard/src/api/apps.ts`

**Interfaces:**
- Consumes: `fileServerClient.list(path, sourceID)`.
- Produces:

```ts
type AppFolderPickerProps = {
  disabled: boolean;
  label: string;
  sourceID: string;
  value: string;
  onChange: (path: string) => void;
};
```

- [ ] **Step 1: Write failing UI tests**

Mock `fileServerClient.list` with root, nested, and deep folder responses.
Verify:

- chooser is disabled without `sourceID`;
- files are excluded;
- folder activation lists that folder;
- breadcrumbs return to ancestors;
- repeated navigation works beyond three levels;
- **Use this folder** emits `/` at root and the full source-relative path when
  nested;
- listing errors show Retry without closing the picker.

- [ ] **Step 2: Run the picker test and confirm failure**

Run:

```bash
npm --prefix web/dashboard test -- AppFolderPicker.test.tsx
```

Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement path helpers**

Keep helpers in `AppFolderPicker.tsx`:

```ts
function normalizeFolderPath(value: string): string
function parentFolderPath(value: string): string
function folderBreadcrumbs(value: string): Array<{ label: string; path: string }>
```

Normalize to one leading slash, no trailing slash except root, and never
generate parent traversal.

- [ ] **Step 4: Implement the picker component**

Load only the current directory. Filter with:

```ts
const folders = (payload.items || []).filter((item) => item.is_directory);
```

Use buttons for folders and breadcrumbs, an inline loading/error state, Retry,
Cancel, and **Use this folder**. Do not expose create, rename, move, or delete.

- [ ] **Step 5: Run picker tests**

Run:

```bash
npm --prefix web/dashboard test -- AppFolderPicker.test.tsx
```

Expected: PASS.

### Task 3: Integrate sources and dependent paths into Apps Settings

**Files:**
- Modify: `web/dashboard/src/settings/AppsSettings.tsx`
- Modify: `web/dashboard/src/settings/AppsSettings.test.tsx`
- Modify: `web/dashboard/src/dashboard/fileServerTargets.ts`
- Modify: `web/dashboard/src/dashboard/fileServerTargets.test.ts`
- Modify: `web/dashboard/src/api/apps.ts`

**Interfaces:**
- Produces:

```ts
export async function loadPrimaryFileTargets(...): Promise<FileTarget[]>
```

- Consumes: `AppSettingsField.source_field` and `AppFolderPicker`.

- [ ] **Step 1: Write failing source-discovery tests**

Verify `loadPrimaryFileTargets` returns enabled SMB/local entries and excludes
worker targets. Preserve the existing `loadFileTargets` behavior for File
Server.

- [ ] **Step 2: Write failing Apps Settings tests**

Use a Gramaton-style schema containing:

```ts
[
  { key: "movie_source_id", type: "select", source: "file_sources" },
  { key: "movie_destination_path", type: "path", source_field: "movie_source_id", required: true },
  { key: "tv_source_id", type: "select", source: "file_sources" },
  { key: "tv_destination_path", type: "path", source_field: "tv_source_id", required: true },
]
```

Verify independent select options, source changes clear only their dependent
path, static select options render, free-text path fields remain text inputs,
and required dependent paths prevent save until selected.

- [ ] **Step 3: Run the focused tests and confirm failure**

Run:

```bash
npm --prefix web/dashboard test -- fileServerTargets.test.ts AppsSettings.test.tsx
```

- [ ] **Step 4: Implement primary source loading**

Reuse the existing service-profile parsing and return only entries with
`agentID === ""`.

- [ ] **Step 5: Extend Apps Settings state**

Load bootstrap, apps, and primary file targets together. Add source options to
ready state and render:

- static `select.options`;
- dynamic `file_sources` options;
- `AppFolderPicker` for `path` fields with `source_field`;
- the existing text input for path fields without `source_field`.

When a source changes, find fields whose `source_field` matches and clear only
those values.

- [ ] **Step 6: Enforce required values in the form**

Disable save when a required non-secret field is blank. Root `/` is non-blank
and valid.

- [ ] **Step 7: Run dashboard tests**

Run:

```bash
npm --prefix web/dashboard test -- fileServerTargets.test.ts AppsSettings.test.tsx AppFolderPicker.test.tsx
npm --prefix web/dashboard run check
npm --prefix web/dashboard run build
```

Expected: all PASS; the existing bundle-size warning is non-blocking.

### Task 4: Declare independent Gramaton locations

**Files in `/Volumes/CampbellDrive/Projects/gramaton`:**
- Modify: `app.json`
- Modify: `schemas/config.schema.json`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `internal/protocol/media.go`
- Modify: `README.md`

**Interfaces:**
- Produces config keys:

```go
MovieSourceID string `json:"movie_source_id,omitempty"`
TVSourceID    string `json:"tv_source_id,omitempty"`
```

- Compatibility: both fall back to `SourceID`.

- [ ] **Step 1: Write failing config tests**

Verify `newMediaService` maps explicit source IDs independently and maps legacy
`source_id` to both effective locations when explicit fields are absent.

- [ ] **Step 2: Run the focused Gramaton tests on the demo and confirm failure**

Run:

```bash
go test . -run 'TestNewMediaService.*Source' -count=1
```

- [ ] **Step 3: Update manifest and config schema**

Replace the visible `source_id` setting with `movie_source_id` and
`tv_source_id`, each using `source: "file_sources"`. Add `source_field` to each
required destination path. Declare two `configured_source` permissions.
Retain legacy `source_id` in `config.schema.json`.

- [ ] **Step 4: Update config decoding and protocol settings**

Carry explicit and effective source IDs through settings status/apply. Preserve
legacy fields in the wire types until all existing consumers migrate.

- [ ] **Step 5: Run main/protocol tests**

Run:

```bash
go test . ./internal/protocol -count=1
```

Expected: PASS.

### Task 5: Make Gramaton file operations media-source aware

**Files in `/Volumes/CampbellDrive/Projects/gramaton`:**
- Modify: `internal/media/service.go`
- Modify: `internal/media/service_test.go`

**Interfaces:**
- Produces:

```go
func (s *Service) sourceIDForMediaType(mediaType string) string
func (s *Service) destinationForMediaType(mediaType string) (sourceID string, path string)
```

- [ ] **Step 1: Write failing service tests**

Use two in-memory/local sources. Verify movie stat/write/delete calls affect
only the Movie source and TV calls affect only the TV source. Verify legacy
fallback, root, deep nested paths, missing source, missing folder, and
non-directory folder validation.

- [ ] **Step 2: Run focused tests and confirm failure**

Run:

```bash
go test ./internal/media -run 'Test.*(Source|Destination|Settings)' -count=1
```

- [ ] **Step 3: Implement effective source selection**

Replace direct `cfg.SourceID` use in existence checks, part cleanup, writers,
final stat, and destination labels with the media-type helper.

- [ ] **Step 4: Validate both configured locations**

For each effective source/path pair:

```go
item, err := s.files.StatSource(ctx, sourceID, path)
if err != nil { /* safe validation error */ }
if !item.IsDirectory { /* folder required */ }
```

Treat `/` as source root. Do not write a probe file.

- [ ] **Step 5: Run the full Gramaton gate on the demo**

Run:

```bash
gofmt -w .
go mod tidy -go=1.24.0
go test ./...
go build ./...
./scripts/package-gramaton-app.sh
test -s dist/gramaton.hankapp
```

Expected: PASS and a non-empty Linux amd64 package.

### Task 6: Demo integration and artifact acceptance

**Files:**
- Update generated artifact:
  `/Volumes/CampbellDrive/Projects/HankServerside/dist/gramaton.hankapp`
- Preserve any pre-existing different artifact before replacement.

**Interfaces:**
- Consumes the final HankServerside dashboard/runtime and Gramaton package.

- [ ] **Step 1: Sync only intentional source changes to isolated demo worktrees**

Do not copy `.env.*`, tokens, Git metadata, unrelated local files, or existing
demo secrets.

- [ ] **Step 2: Run the HankServerside focused and broad gates on demo**

Run:

```bash
go test ./internal/agent/apps ./internal/protocol
npm --prefix web/dashboard test -- AppFolderPicker.test.tsx AppsSettings.test.tsx fileServerTargets.test.ts
npm --prefix web/dashboard run check
npm --prefix web/dashboard run build
go build ./...
```

- [ ] **Step 3: Build and preview the package**

Confirm root `app.json`, referenced schemas, executable runtime mode, package
hash, replacement status, and nine declared commands.

- [ ] **Step 4: Deploy the Hank demo runtime**

Build the shared Hank image with an explicit `HANK_REMOTE_BUILD_VERSION`.
Preserve `.env.cloud`, `.env.agent`, volumes, `hankdemo-cloudflared`, RTI, and
RTM workloads. Recreate only required Hank services and retain the loopback
cloud bind `127.0.0.1:18080`.

- [ ] **Step 5: Activate and configure Gramaton**

Install the package, confirm package replacement preserves credentials, then
select different existing nested folders on `hankdemo` and `hankdemo2`.
Reopen settings and verify all four values persist.

- [ ] **Step 6: Prove the operator workflow**

Through the live dashboard/API, verify source dropdowns, arbitrary-depth
navigation, breadcrumbs, directories-only listing, source-dependent clearing,
and save/reload.

- [ ] **Step 7: Prove Gramaton end to end**

Use a public-domain movie published before 1931. Verify search, selection,
planning, confirmation, job completion, and final file placement in the Movie
source/folder. Verify TV planning reports the independent TV destination.

- [ ] **Step 8: Run final operational checks**

Run:

```bash
scripts/doctor.sh
curl -fsS https://hankdemo.campbellservers.com/healthz
curl -fsS https://hankdemo.campbellservers.com/readyz
```

Also verify current dashboard asset hashes and inspect cloud/agent logs for new
errors.

- [ ] **Step 9: Copy the verified artifact to the MacBook**

Use pinned SSH host verification, copy through a temporary file, verify SHA-256,
then atomically replace:

```text
/Volumes/CampbellDrive/Projects/HankServerside/dist/gramaton.hankapp
```

- [ ] **Step 10: Report completion**

State security impact, database impact, exact validation, any skipped checks,
package path/hash, and whether the legal movie download completed.
