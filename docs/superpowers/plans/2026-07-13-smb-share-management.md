# SMB Share Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build complete multi-share SMB management in Settings > Connections, including selection, add, edit, save, remove, and non-persisting connection tests.

**Architecture:** Keep the SMB service profile as the canonical array and add pure frontend helpers that update shares by stable ID while preserving unrelated fields. Add one admin-only test endpoint and one versioned agent command that probes a transient SMB configuration without applying or persisting it.

**Tech Stack:** Go standard library, existing cloud/router/protocol packages, go-smb2 through the existing agent files service, React 19, TypeScript 6, Vitest, Testing Library.

## Global Constraints

- Preserve the single-home outbound cloud-and-agent architecture.
- Do not store SMB credentials in the cloud database or logs.
- Keep existing single-share and legacy public-config compatibility.
- Preserve unrelated SMB shares, per-source policy, and host-folder configuration.
- Blank passwords on existing shares preserve the agent-owned password.
- No database migration.
- Follow red-green-refactor for every behavior change.

---

### Task 1: Pure multi-share editor model

**Files:**
- Create: `web/dashboard/src/settings/smbShareEditor.ts`
- Create: `web/dashboard/src/settings/smbShareEditor.test.ts`

**Interfaces:**
- Consumes: SMB `public_config_json` records.
- Produces: `SMBShare`, `smbSourceRecords`, `shareDraft`, `newShareDraft`, `upsertSMBShare`, `removeSMBShare`, and `validateSMBShare`.

- [ ] **Step 1: Write failing tests**

Cover parsing all shares, preserving policy and unrelated records on update, appending a generated-ID draft, deleting only the selected share, preserving `active_source_id`, falling back when the active share is removed, and rejecting duplicate IDs.

```ts
expect(upsertSMBShare(config, edited).shares).toEqual([editedPublic, archivePublic]);
expect(removeSMBShare(config, "media").active_source_id).toBe("archive");
expect(validateSMBShare(duplicate, [existing], "new")).toBe("Share ID is already in use.");
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `npm --prefix web/dashboard test -- --run src/settings/smbShareEditor.test.ts`

Expected: FAIL because `smbShareEditor.ts` does not exist.

- [ ] **Step 3: Implement the typed pure helpers**

```ts
export type SMBShare = {
  id: string; name: string; host: string; share: string;
  domain: string; username: string; password_set?: boolean;
  policy?: Record<string, unknown>;
};

export function upsertSMBShare(
  config: Record<string, unknown>,
  share: SMBShare,
  originalID?: string,
): Record<string, unknown>;

export function removeSMBShare(
  config: Record<string, unknown>,
  shareID: string,
): Record<string, unknown>;
```

Normalize hosts and IDs, retain unknown fields on the matching public record, retain unrelated config keys, and mirror legacy top-level fields from the active share.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `npm --prefix web/dashboard test -- --run src/settings/smbShareEditor.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/dashboard/src/settings/smbShareEditor.ts web/dashboard/src/settings/smbShareEditor.test.ts
git commit -m "Add SMB share editor model"
```

### Task 2: Non-persisting SMB connection test command

**Files:**
- Modify: `internal/protocol/config.go`
- Modify: `internal/agent/config_manager.go`
- Modify: `internal/agent/config_manager_test.go`
- Modify: `internal/agent/commands.go`
- Modify: `internal/agent/commands_test.go`

**Interfaces:**
- Produces: `protocol.ConfigSMBTestRequest`, `protocol.ConfigSMBTestResponse`, `configManager.TestSMB`, and command `config.smb_test`.
- Consumes: existing `agentfiles.NewWithConfig`, `ListSource`, and dispatcher error mapping.

- [ ] **Step 1: Write failing agent tests**

```go
func TestConfigManagerSMBTestDoesNotMutateOrPersist(t *testing.T) {
    manager := newConfigManager(envPath, ha, files)
    manager.testSMB = func(_ context.Context, cfg agentfiles.SMBConfig) error {
        if cfg.ID != "archive" || cfg.Password != "draft-secret" { t.Fatalf("cfg = %#v", cfg) }
        return nil
    }
    before := files.SMBConfigs()
    response, err := manager.TestSMB(context.Background(), protocol.ConfigSMBTestRequest{ID: "archive", Host: "nas.local", Share: "archive", Password: "draft-secret"})
    if err != nil || !response.OK { t.Fatalf("response = %#v, err = %v", response, err) }
    if !reflect.DeepEqual(before, files.SMBConfigs()) { t.Fatalf("live SMB config changed: before=%#v after=%#v", before, files.SMBConfigs()) }
}
```

Also dispatch `config.smb_test` and assert invalid host/share requests return a bad request without exposing the password.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/agent -run 'Test(ConfigManagerSMBTest|DispatcherConfigSMBTest)' -count=1`

Expected: FAIL because the protocol types and methods do not exist.

- [ ] **Step 3: Add protocol and agent implementation**

```go
type ConfigSMBTestRequest struct {
    ID, Name, Host, Share, Username, Password, Domain string
}
type ConfigSMBTestResponse struct { OK bool `json:"ok"` }

func (m *configManager) TestSMB(ctx context.Context, request protocol.ConfigSMBTestRequest) (protocol.ConfigSMBTestResponse, error) {
    cfg := agentfiles.SMBConfig{ID: strings.TrimSpace(request.ID), Name: strings.TrimSpace(request.Name), Host: strings.TrimSpace(request.Host), Share: strings.TrimSpace(request.Share), Username: strings.TrimSpace(request.Username), Password: request.Password, Domain: strings.TrimSpace(request.Domain)}
    if !cfg.Enabled() { return protocol.ConfigSMBTestResponse{}, errors.New("SMB host and share are required") }
    if err := m.testSMB(ctx, cfg); err != nil { return protocol.ConfigSMBTestResponse{}, fmt.Errorf("SMB connection failed: %w", err) }
    return protocol.ConfigSMBTestResponse{OK: true}, nil
}
```

Initialize `testSMB` with a transient `agentfiles.Service` that lists the draft share root, and add the dispatcher case. Do not call `ApplySMBConfigs` or any persistence helper.

- [ ] **Step 4: Run focused and package tests**

Run: `go test ./internal/agent ./internal/protocol -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/protocol/config.go internal/agent/config_manager.go internal/agent/config_manager_test.go internal/agent/commands.go internal/agent/commands_test.go
git commit -m "Add transient SMB connection tests"
```

### Task 3: Admin-only SMB test HTTP endpoint

**Files:**
- Modify: `internal/cloud/collaboration_handlers.go`
- Modify: `internal/cloud/collaboration_test.go`
- Modify: `web/dashboard/src/api/connections.ts`
- Modify: `web/dashboard/src/api/connections.test.ts`

**Interfaces:**
- Produces: `POST /v1/home/service-profiles/smb/test` and `ConnectionsClient.testSMB(input)`.
- Consumes: `protocol.ConfigSMBTestRequest`, `sendAgentCommand`, existing authenticated home membership and CSRF middleware.

- [ ] **Step 1: Write failing cloud and client tests**

Assert member access returns 403, offline agent returns 409, an online agent receives only `config.smb_test`, successful output is `{ "ok": true }`, and the API client uses the exact POST path.

```ts
await client.testSMB({ id: "archive", host: "nas.local", share: "archive", password: "secret" });
expect(calls[0]).toEqual({ path: "/v1/home/service-profiles/smb/test", method: "POST", body: expect.any(Object) });
```

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/cloud -run TestSMBServiceProfileTest -count=1`

Run: `npm --prefix web/dashboard test -- --run src/api/connections.test.ts`

Expected: FAIL because the endpoint and client method do not exist.

- [ ] **Step 3: Implement endpoint and client**

Handle the three-part path before the generic PUT branch. Require admin, parse a bounded JSON request, return 409 when the home agent is offline, relay `config.smb_test`, map agent errors to 502, decode `ConfigSMBTestResponse`, and return only the decoded response.

```ts
testSMB(input: SMBTestInput) {
  return this.api.request<SMBTestResult>("/v1/home/service-profiles/smb/test", {
    method: "POST",
    body: input,
  });
}
```

- [ ] **Step 4: Run focused tests and verify GREEN**

Run the two commands from Step 2.

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/collaboration_handlers.go internal/cloud/collaboration_test.go web/dashboard/src/api/connections.ts web/dashboard/src/api/connections.test.ts
git commit -m "Expose SMB connection test endpoint"
```

### Task 4: Complete SMB management UI

**Files:**
- Modify: `web/dashboard/src/settings/ConnectionsSettings.tsx`
- Create: `web/dashboard/src/settings/ConnectionsSettings.test.tsx`
- Modify: `web/dashboard/src/styles.css`

**Interfaces:**
- Consumes: Task 1 editor helpers and Task 3 `connectionsClient.testSMB`.
- Produces: selectable share list and Add, Save, Test Connection, Remove actions.

- [ ] **Step 1: Write failing rendered tests**

Mock bootstrap/profile requests and render `ConnectionsSettings`. Assert both share rows appear, selecting Archive changes labeled inputs, editing and saving preserves Media, Add creates a blank draft, Test sends draft values without saving, and Remove confirms then saves only remaining shares.

```ts
fireEvent.click(screen.getByRole("button", { name: /Edit Archive/i }));
expect(screen.getByLabelText("SMB share name")).toHaveValue("archive");
fireEvent.click(screen.getByRole("button", { name: "Test Connection" }));
await waitFor(() => expect(testSMB).toHaveBeenCalledWith(expect.objectContaining({ id: "archive" })));
```

- [ ] **Step 2: Run rendered test and verify RED**

Run: `npm --prefix web/dashboard test -- --run src/settings/ConnectionsSettings.test.tsx`

Expected: FAIL because only the first share is rendered and actions are missing.

- [ ] **Step 3: Implement the list/editor UI**

Track `smbConfig`, `smbShares`, `selectedSMBID`, `smbDraft`, and `originalSMBID` in ready state. Use stable event handlers to select or add drafts. Save with `upsertSMBShare`, test with `connectionsClient.testSMB`, and remove with the existing confirmation provider plus `removeSMBShare`. Disable all management actions for view-only users and show per-action progress/result copy.

Render accessible buttons named `Edit <label>`, one editor with explicit labels, and destructive removal copy. Keep the password input empty after every reload.

- [ ] **Step 4: Run focused frontend tests and build**

Run: `npm --prefix web/dashboard test -- --run src/settings/ConnectionsSettings.test.tsx src/settings/smbShareEditor.test.ts src/api/connections.test.ts`

Run: `npm --prefix web/dashboard run build`

Expected: PASS and production assets emitted to `internal/cloud/ui/react`.

- [ ] **Step 5: Commit**

```bash
git add web/dashboard/src/settings/ConnectionsSettings.tsx web/dashboard/src/settings/ConnectionsSettings.test.tsx web/dashboard/src/settings/smbShareEditor.ts web/dashboard/src/styles.css internal/cloud/ui/react
git commit -m "Build complete SMB share management"
```

### Task 5: Full verification

**Files:**
- Verify only.

**Interfaces:**
- Consumes: all prior tasks.
- Produces: proof that backend, frontend, security, and repository checks pass.

- [ ] **Step 1: Format and inspect**

Run: `gofmt -w ./cmd ./internal`

Run: `git diff --check`

Expected: no output from `git diff --check`.

- [ ] **Step 2: Run backend verification**

Run: `go build ./...`

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Run frontend verification**

Run: `npm --prefix web/dashboard run check`

Expected: all Vitest tests and Vite production build pass.

- [ ] **Step 4: Review security and database impact**

Confirm the test endpoint is admin-only and CSRF-protected, secrets are absent from logs/responses/database writes, the test path does not persist or mutate live config, and no migration/schema change exists.

- [ ] **Step 5: Commit any generated frontend asset update**

```bash
git add internal/cloud/ui/react
git commit -m "Refresh dashboard assets for SMB management"
```
