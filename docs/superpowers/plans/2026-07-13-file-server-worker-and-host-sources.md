# File Server Worker and Host Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show primary SMB shares, primary host folders, and online worker shared folders in the HankServerside File Server and route every file action to the owning agent.

**Architecture:** Introduce a focused dashboard source-discovery module that combines the primary SMB profile snapshot with file-capable worker agents into stable `FileTarget` values. Extend the existing file client with optional `agentID` parameters, then make `FileServerPage` select and route by target instead of source ID alone while limiting moves to the active agent.

**Tech Stack:** React 19, TypeScript, Vitest, Testing Library, Hank WebSocket commands, authenticated Hank HTTP APIs.

## Global Constraints

- Keep existing single-agent behavior compatible by using a blank `agent_id` for the primary agent.
- Worker virtual roots use a blank `source_id` and expose shared folders as top-level directories.
- Do not offer cross-agent moves.
- Add no public routes, credentials, filesystem permissions, dependencies, or database schema changes.
- Preserve existing authentication, file policy, containment, transfer lease, and routing enforcement.

---

### Task 1: Agent-Aware File Client

**Files:**
- Modify: `web/dashboard/src/api/fileServer.ts`
- Test: `web/dashboard/src/api/fileServer.test.ts`

**Interfaces:**
- Consumes: `HankSocket.sendCommand(command, body, {agentID})` and existing transfer setup routes accepting `agent_id`.
- Produces: existing `FileServerClient` methods with an optional final `agentID?: string` argument.

- [ ] **Step 1: Write failing command-routing tests**

Add tests that call `list`, `search`, `createDirectory`, `stat`, `rename`, `move`, and `deleteItem` with `agentID = "worker-1"` and assert each socket call includes `{ agentID: "worker-1" }`. Add transfer tests asserting download/upload setup bodies contain `agent_id: "worker-1"`.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `npm --prefix web/dashboard test -- src/api/fileServer.test.ts`

Expected: FAIL because file methods neither accept nor forward an agent ID.

- [ ] **Step 3: Add the minimal agent-aware client API**

Change the socket interface to:

```ts
sendCommand<T>(command: string, body?: unknown, options?: { agentID?: string }): Promise<T>;
```

Add optional `agentID?: string` to every file command method and call:

```ts
this.socket.sendCommand(command, body, withAgentID(agentID));
```

where `withAgentID` returns `{ agentID }` only when nonblank. Add `agent_id` to transfer setup request bodies when nonblank and thread the parameter through `uploadFile`.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `npm --prefix web/dashboard test -- src/api/fileServer.test.ts`

Expected: PASS.

---

### Task 2: Unified File Target Discovery

**Files:**
- Create: `web/dashboard/src/dashboard/fileServerTargets.ts`
- Create: `web/dashboard/src/dashboard/fileServerTargets.test.ts`

**Interfaces:**
- Consumes: `ServiceProfile`, `HomeAgentEntry`, `agentDisplayName`, `agentHasCapability`, `agentIsOnline`, and `agentIsPrimary`.
- Produces:

```ts
export type FileTarget = {
  key: string;
  sourceID: string;
  agentID: string;
  name: string;
  detail: string;
  kind: "smb" | "host" | "worker";
};

export function fileTargetsFrom(profiles: ServiceProfile[], agents: HomeAgentEntry[]): FileTarget[];
export async function loadFileTargets(): Promise<FileTarget[]>;
```

- [ ] **Step 1: Write failing normalization tests**

Cover a primary profile containing duplicate `sources` / `file_sources`, `shares`, and `folders`; assert SMB and local records remain, duplicate IDs collapse, and host folder details are retained. Cover online workers with `files.read` or `files.list`, and assert offline, primary, and non-file workers are omitted.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `npm --prefix web/dashboard test -- src/dashboard/fileServerTargets.test.ts`

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement target normalization and resilient loading**

Parse all four compatible profile arrays. Use keys `primary:${sourceID}` for primary targets and `worker:${agentID}` for workers. Workers have blank `sourceID`, their actual `agentID`, and a detail identifying their shared-folder virtual root. Load profiles and agents independently with `Promise.allSettled` so one failed discovery path does not hide the other.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `npm --prefix web/dashboard test -- src/dashboard/fileServerTargets.test.ts`

Expected: PASS.

---

### Task 3: File Server Target Selection and Routing

**Files:**
- Modify: `web/dashboard/src/dashboard/FileServerPage.tsx`
- Modify: `web/dashboard/src/dashboard/FileServerPage.test.tsx`

**Interfaces:**
- Consumes: `FileTarget`, `loadFileTargets`, and agent-aware `FileServerClient` methods from Tasks 1–2.
- Produces: a unified target picker and target-aware browse/action state.

- [ ] **Step 1: Write failing page tests**

Add tests proving:

```ts
expect(fileServerClient.list).toHaveBeenCalledWith("/", "host-media", undefined);
expect(fileServerClient.list).toHaveBeenCalledWith("/", undefined, "worker-1");
```

Also assert the picker shows the host folder and worker device, worker preview URLs include `agent_id`, worker download/upload calls receive the agent ID, changing targets clears stale path/search/selection state, and move destinations exclude targets owned by another agent.

- [ ] **Step 2: Run focused page tests and verify RED**

Run: `npm --prefix web/dashboard test -- src/dashboard/FileServerPage.test.tsx`

Expected: FAIL because the page still selects only by source ID and filters local sources/workers.

- [ ] **Step 3: Replace source-only state with target state**

Store `targets: FileTarget[]` and `activeTargetKey`. Resolve the active target once per render, and pass its `sourceID` and `agentID` to list/search/create/delete/upload/download/rename/move/preview operations. Use the target key for picker identity so blank worker source IDs cannot collide.

- [ ] **Step 4: Preserve deep links and reset state on target changes**

Read optional `source_id` and `agent_id` query parameters during initial selection. On target change, load `/`, clear search results, selection, preview, and dialogs, and never fall back to the primary if the selected worker goes offline mid-request.

- [ ] **Step 5: Restrict move destinations to the active agent**

Pass only targets whose `agentID` equals the active target's `agentID` into the move dialog. Continue passing the destination source ID to `files.move`; do not expose another device as a destination.

- [ ] **Step 6: Run focused page tests and verify GREEN**

Run: `npm --prefix web/dashboard test -- src/dashboard/FileServerPage.test.tsx`

Expected: PASS.

---

### Task 4: Regression and Contract Verification

**Files:**
- Modify only if a failing check identifies a task-scoped regression.

**Interfaces:**
- Consumes: completed dashboard behavior.
- Produces: build and test evidence for the shipped surface.

- [ ] **Step 1: Run all dashboard checks**

Run: `npm --prefix web/dashboard run check`

Expected: TypeScript checks, Vitest suite, and Vite production build all pass.

- [ ] **Step 2: Run relevant Go routing and host-folder tests**

Run:

```bash
go test ./internal/agent ./internal/agent/files ./internal/config ./internal/cloud -run 'Host|Local|Folder|FileTransfer|Preview|MultiAgent|Target' -count=1
```

Expected: PASS.

- [ ] **Step 3: Verify generated dashboard assets and working tree**

Run `git status --short`, confirm the Vite build updated only expected embedded assets, and run `git diff --check`.

- [ ] **Step 4: Commit the implementation**

Stage the plan, dashboard source/tests, and expected embedded build output, then commit with:

```bash
git commit -m "Show worker and host file sources"
```
