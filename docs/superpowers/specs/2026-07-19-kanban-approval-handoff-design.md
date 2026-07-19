# Kanban Approval Handoff Workflow

## Goal

Keep MCP-driven Kanban work moving when an active card requires human approval. A card waiting on a person must leave the active-work queue, preserve why it is blocked, and not prevent the assistant from starting the next intake card.

## Workflow

When work on a card reaches a human handoff:

1. Append a `blocker` work-log entry that states the decision, approval, or review needed without replacing the card's original requirements.
2. Move an unfinished card requiring a human decision or approval to the column with the `human` role.
3. Move completed work awaiting human validation to the column with the `review` role.
4. If the preferred role is not configured, use the other human-handoff role (`human` or `review`).
5. After the handoff, list the configured intake column and continue with its first ordered card instead of waiting for the human response.
6. If neither handoff role exists, leave the card in place, report the board-configuration problem, and still continue with the next intake card.

The assistant must not treat an intake card as next work when it is the same card that was just handed off or when that card remains in intake because no handoff column is configured.

## Implementation Boundary

This is MCP client workflow guidance, not implicit server automation. The existing `append_kanban_worklog`, `move_kanban_card`, and `list_kanban_cards` tools remain the mutation and selection primitives. They continue to use exact stable IDs and the existing Notes revision/conflict rules.

The change will update MCP initialization guidance, relevant tool descriptions, and `docs/mcp.md`. It will not add a task service, mutate board structure, infer moves from card text, or introduce a new database/API contract.

## Failure Handling

- A failed work-log or move call is reported accurately and does not get described as a successful handoff.
- Missing role configuration does not authorize matching a destination by column title.
- A concurrent card or column change follows the existing bounded conflict behavior.
- Continuing intake work must not erase or hide the outstanding human request.

## Verification

Add focused assertions that the MCP initialization instructions and advertised Kanban tool descriptions communicate the handoff-and-continue rule. Run the focused Go tests, formatting checks, and the relevant package test suite.

## Impact

- Security: no authorization, scope, audit-content, secret-handling, or destructive-operation change.
- Database: no schema, migration, or persistence-model change.
- Compatibility: additive guidance only; existing tool names, schemas, and results remain unchanged.
