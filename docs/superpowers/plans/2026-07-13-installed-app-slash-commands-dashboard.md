# Installed App Slash Commands Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore package-owned slash commands such as `/gramaton` to the React HankAI dashboard command palette.

**Architecture:** Extend the existing frontend app metadata type with `slash_commands`, fetch app metadata in parallel with HankAI bootstrap requests, and derive one command palette from stable built-ins plus enabled app commands. Keep command authorization and execution server-side through the existing assistant and `apps.invoke` paths.

**Tech Stack:** React 19, TypeScript 6, Vitest, Testing Library, Vite

## Global Constraints

- Do not hardcode Gramaton, Hermes, YDownload, or another installable app command in HankAI.
- Preserve built-in commands when an app declares a case-insensitive token conflict.
- Ignore disabled apps and malformed or blank package command tokens.
- Treat app discovery as optional enrichment; a failed apps request must leave HankAI usable with built-in commands.
- Add no routes, authorization changes, secret handling, database writes, schema migrations, or new dependencies.

---

### Task 1: Restore contract-driven slash command discovery

**Files:**
- Modify: `web/dashboard/src/api/apps.ts:19-37`
- Modify: `web/dashboard/src/dashboard/HankAIPage.tsx:1-196`
- Test: `web/dashboard/src/dashboard/HankAIPage.test.tsx`

**Interfaces:**
- Consumes: `appsClient.listApps(): Promise<AppsListPayload>` and `AppSummary.slash_commands` from `GET /v1/home/apps`.
- Produces: `SlashCommand[]` stored in ready HankAI page state and rendered by the existing command palette.

- [ ] **Step 1: Write the failing enabled-app palette test**

Add an `appsClient` mock and a default empty app response:

```tsx
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const appsClient = vi.hoisted(() => ({
  listApps: vi.fn(),
}));

vi.mock("../api/apps", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/apps")>();
  return { ...actual, appsClient };
});

beforeEach(() => {
  appsClient.listApps.mockResolvedValue({ apps: [] });
});
```

Add a test that exposes the regression:

```tsx
it("shows enabled installed app slash commands and inserts the selected token", async () => {
  hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
  hankAIClient.listSessions.mockResolvedValue({ sessions: [] });
  appsClient.listApps.mockResolvedValue({
    apps: [{
      id: "gramaton",
      name: "Gramaton",
      enabled: true,
      slash_commands: [{
        command: "/gramaton",
        command_id: "search",
        description: "Search for a movie or TV show on Gramaton.",
      }],
    }],
  });

  render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

  const composer = await screen.findByRole("textbox", { name: "Message" });
  fireEvent.change(composer, { target: { value: "/gra" } });
  fireEvent.mouseDown(screen.getByRole("option", { name: /gramaton/i }));

  expect(composer).toHaveValue("/gramaton ");
  expect(appsClient.listApps).toHaveBeenCalledTimes(1);
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
npm --prefix web/dashboard run test:run -- HankAIPage.test.tsx -t "shows enabled installed app slash commands"
```

Expected: FAIL because `appsClient.listApps` is not called and `/gramaton` is absent.

- [ ] **Step 3: Add failure and filtering regression tests before production code**

Add tests proving disabled commands are hidden and app discovery failure preserves built-ins:

```tsx
it("excludes disabled installed app slash commands", async () => {
  hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
  hankAIClient.listSessions.mockResolvedValue({ sessions: [] });
  appsClient.listApps.mockResolvedValue({
    apps: [{
      id: "gramaton",
      name: "Gramaton",
      enabled: false,
      slash_commands: [{ command: "/gramaton", command_id: "search" }],
    }],
  });

  render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

  const composer = await screen.findByRole("textbox", { name: "Message" });
  fireEvent.change(composer, { target: { value: "/" } });
  expect(screen.queryByRole("option", { name: /gramaton/i })).not.toBeInTheDocument();
});

it("keeps built-in commands when installed app discovery fails", async () => {
  hankAIClient.status.mockResolvedValue({ provider: "gpt-5-codex", ready: true });
  hankAIClient.listSessions.mockResolvedValue({ sessions: [] });
  appsClient.listApps.mockRejectedValue(new Error("apps unavailable"));

  render(<ConfirmDialogProvider><HankAIPage /></ConfirmDialogProvider>);

  const composer = await screen.findByRole("textbox", { name: "Message" });
  fireEvent.change(composer, { target: { value: "/fi" } });
  expect(screen.getByRole("option", { name: /files/i })).toBeInTheDocument();
});
```

- [ ] **Step 4: Extend the frontend app contract type**

Add the package slash-command shape and expose it on `AppSummary`:

```ts
export type AppSlashCommand = {
  command: string;
  command_id: string;
  description?: string;
};

slash_commands?: AppSlashCommand[];
```

- [ ] **Step 5: Implement command normalization and optional parallel discovery**

Import the apps API and type, rename the static constant to `BUILT_IN_SLASH_COMMANDS`, add commands to ready state, and derive package commands with built-in precedence:

```tsx
import { appsClient, type AppSummary } from "../api/apps";

type SlashCommand = { token: string; hint: string };

const BUILT_IN_SLASH_COMMANDS: SlashCommand[] = [
  { token: "/ha", hint: "Control Home Assistant entities" },
  { token: "/files", hint: "Search or browse the file server" },
  { token: "/notes", hint: "Find or append to a note" },
  { token: "/append", hint: "Append text to a note" },
  { token: "/calendar", hint: "Check the calendar" },
  { token: "/docs", hint: "Search project docs" },
  { token: "/status", hint: "Report connector + agent status" },
];

function slashCommandsForApps(apps: AppSummary[]): SlashCommand[] {
  const commands = [...BUILT_IN_SLASH_COMMANDS];
  const seen = new Set(commands.map((command) => command.token.toLowerCase()));
  for (const app of apps) {
    if (!app.enabled) continue;
    for (const slashCommand of app.slash_commands || []) {
      const token = slashCommand.command?.trim();
      if (!token?.startsWith("/") || token.includes(" ")) continue;
      const identity = token.toLowerCase();
      if (seen.has(identity)) continue;
      seen.add(identity);
      commands.push({
        token,
        hint: slashCommand.description?.trim() || `Run ${app.name?.trim() || app.id || app.app_id || "installed app"}`,
      });
    }
  }
  return commands;
}
```

Load app metadata without making it page-fatal and store the merged commands:

```tsx
const [assistantStatus, sessionPayload, appsPayload] = await Promise.all([
  hankAIClient.status(),
  hankAIClient.listSessions(),
  appsClient.listApps().catch(() => ({ apps: [] })),
]);

setState({
  status: "ready",
  assistantStatus,
  sessions,
  selectedSessionID,
  messages,
  draft: "",
  notice: "",
  sending: false,
  slashCommands: slashCommandsForApps(appsPayload.apps),
});
```

Render palette matches from state with case-insensitive filtering:

```tsx
const paletteMatches = showPalette
  ? state.slashCommands.filter((command) => command.token.toLowerCase().startsWith(draftTrimmed.toLowerCase()))
  : [];
```

- [ ] **Step 6: Run the focused test file and verify GREEN**

Run:

```bash
npm --prefix web/dashboard run test:run -- HankAIPage.test.tsx
```

Expected: all `HankAIPage` tests PASS with no unhandled rejection or React warnings.

- [ ] **Step 7: Commit the contract restoration**

```bash
git add web/dashboard/src/api/apps.ts web/dashboard/src/dashboard/HankAIPage.tsx web/dashboard/src/dashboard/HankAIPage.test.tsx
git commit -m "Restore installed app slash commands"
```

---

### Task 2: Verify the dashboard and live Gramaton flow

**Files:**
- Verify: `web/dashboard/src/api/apps.ts`
- Verify: `web/dashboard/src/dashboard/HankAIPage.tsx`
- Verify: `web/dashboard/src/dashboard/HankAIPage.test.tsx`
- Generated build output: `internal/cloud/ui/react`

**Interfaces:**
- Consumes: the completed React change from Task 1 and the deployed Gramaton app metadata.
- Produces: tested dashboard assets and evidence that `/gramaton` is discovered and invokes the working installed app.

- [ ] **Step 1: Run the complete dashboard test suite**

Run:

```bash
npm --prefix web/dashboard run test:run
```

Expected: all dashboard tests PASS.

- [ ] **Step 2: Build production dashboard assets**

Run:

```bash
npm --prefix web/dashboard run build
```

Expected: TypeScript validation and Vite production build PASS; embedded assets update under `internal/cloud/ui/react`.

- [ ] **Step 3: Run focused backend contract tests**

Run:

```bash
go test ./internal/cloud -run 'TestAssistantInstalledGramatonSlashUsesSearchInput|TestHomeApps' -count=1
```

Expected: PASS, proving the server still resolves installed `/gramaton` metadata and emits the typed search input.

- [ ] **Step 4: Run rendered interaction validation**

Use the available Browser plugin against the authenticated HankAI page. Verify page identity, meaningful DOM content, absence of framework overlays, console health, command-palette interaction, and screenshot evidence. Type `/gra`, select `/gramaton`, and confirm the composer contains `/gramaton `.

- [ ] **Step 5: Verify live command execution**

Deploy through the repository's demo workflow, run `scripts/doctor.sh`, verify deployed asset freshness, then submit `/gramaton Project Hail Mary` through HankAI. Expected: the assistant response contains at least one real Gramaton media result rather than an empty search or command error.

- [ ] **Step 6: Commit generated production assets if changed**

```bash
git add internal/cloud/ui/react
git commit -m "Build dashboard app command assets"
```

Skip this commit only if the build produces no tracked asset changes.
