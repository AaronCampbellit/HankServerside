# Kanban Approval Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Teach MCP clients to hand approval-blocked Kanban cards to a human-facing column and continue with the next ordered intake card.

**Architecture:** Keep orchestration in advertised MCP guidance while preserving the existing explicit, exact-ID card tools. Update initialization instructions and relevant tool descriptions, document the queue semantics, and verify the client-visible contract with focused unit assertions.

**Tech Stack:** Go, MCP JSON-RPC tool metadata, Markdown documentation, Go `testing`.

## Global Constraints

- `user_notes.board_json` remains the only Kanban store.
- Use existing `append_kanban_worklog`, `move_kanban_card`, and `list_kanban_cards`; add no implicit server automation or new API.
- Prefer `human` for unfinished decisions or approvals and `review` for completed work awaiting validation; fall back to the other configured role.
- Continue with the first ordered intake card after handoff; when neither role exists, report configuration trouble, skip the blocked card, and continue intake work.
- Do not guess columns from titles or alter board structure.
- Do not commit, push, or deploy without explicit user authorization.

---

### Task 1: Advertise and Document the Handoff-and-Continue Contract

**Files:**
- Modify: `internal/cloud/mcp_unit_test.go`
- Modify: `internal/cloud/mcp_server.go`
- Modify: `internal/cloud/mcp_tools.go`
- Modify: `docs/mcp.md`

**Interfaces:**
- Consumes: `(*Server).mcpInitializeResult(json.RawMessage) map[string]any`, `mcpToolByName(string) (mcpToolDef, bool)`, and the existing Kanban tool definitions.
- Produces: additive client-visible workflow guidance in MCP initialization and tool descriptions; no schema or result changes.

- [x] **Step 1: Add failing contract assertions**

Extend `TestMCPToolListAndLookup` to assert that the list, work-log, and move tool descriptions collectively state the next-intake-card rule and both `human` and `review` handoff roles. Extend `TestMCPInitializeAndDispatchNoDB` to require the initialization instructions to state that human-approval work is handed off and intake work continues rather than waiting.

```go
workflowDescriptions := strings.Join([]string{
	listCardsDef.Description,
	worklogDef.Description,
	moveDef.Description,
}, " ")
for _, required := range []string{"human", "review", "intake", "continue"} {
	if !strings.Contains(strings.ToLower(workflowDescriptions), required) {
		t.Fatalf("Kanban workflow descriptions missing %q: %s", required, workflowDescriptions)
	}
}
```

```go
instructions := strings.ToLower(res["instructions"].(string))
for _, required := range []string{"human approval", "needs human", "review", "next ordered intake card", "rather than waiting"} {
	if !strings.Contains(instructions, required) {
		t.Fatalf("initialize instructions missing %q: %s", required, instructions)
	}
}
```

- [x] **Step 2: Run the focused tests and confirm RED**

Run:

```bash
go test ./internal/cloud -run 'TestMCPToolListAndLookup|TestMCPInitializeAndDispatchNoDB' -count=1
```

Expected: FAIL because the current metadata advertises Kanban operations but does not define human handoff or continuing the intake queue.

- [x] **Step 3: Add minimal MCP workflow guidance**

Update `mcpInitializeResult` instructions to say:

```text
When an active card needs human approval, append a blocker work-log entry and move unfinished work to Needs Human or completed work awaiting validation to Review, falling back to the other configured role. Then continue with the next ordered intake card rather than waiting. If neither handoff role exists, report the configuration issue, skip the blocked card, and continue intake work.
```

Update the existing tool descriptions without changing their schemas:

- `list_kanban_cards`: explain that ordered results from the configured intake column select the next card after a handoff.
- `append_kanban_worklog`: explain that a blocker entry preserves the human decision or approval request before handoff.
- `move_kanban_card`: explain `human` versus `review`, fallback to the other role, and continuing intake work after the move.

- [x] **Step 4: Run the focused tests and confirm GREEN**

Run:

```bash
go test ./internal/cloud -run 'TestMCPToolListAndLookup|TestMCPInitializeAndDispatchNoDB' -count=1
```

Expected: PASS.

- [x] **Step 5: Document operator-visible workflow semantics**

Add a paragraph to `docs/mcp.md` under **Kanban cards** covering blocker work-log preservation, `human`/`review` routing, role fallback, continuing the next ordered intake card, and the no-role behavior. State that role resolution uses configured semantic metadata and never guesses from column titles.

- [x] **Step 6: Format and run full relevant verification**

Run:

```bash
make fmt
go test ./internal/cloud -count=1
go test ./...
make build
git diff --check
```

Expected: every command exits 0. If a broader command fails for an unrelated pre-existing reason, record its exact failure and keep focused verification separate.

- [x] **Step 7: Review the final diff against the approved design**

Confirm the diff contains only additive guidance/tests/docs plus the approved spec and plan, preserves the untracked `.codex/` directory, and introduces no authorization, audit-content, database, migration, tool-schema, or destructive-operation change.
