package cloud

import "github.com/dropfile/hankremote/internal/domain"

// mcpToolDef describes one MCP tool: its advertised schema and the scopes that
// authorize it (any-of). The execution lives in executeMCPTool.
type mcpToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
	Scopes      []string
	Annotations map[string]any
}

func mcpObjectSchema(props map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func mcpStr(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func mcpInt(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func mcpBool(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func mcpStringArray(desc string) map[string]any {
	return map[string]any{"type": "array", "description": desc, "items": map[string]any{"type": "string"}}
}
func mcpEnum(desc string, values ...string) map[string]any {
	items := make([]any, len(values))
	for index, value := range values {
		items[index] = value
	}
	return map[string]any{"type": "string", "description": desc, "enum": items}
}

var (
	mcpReadOnlyAnnotations  = map[string]any{"readOnlyHint": true}
	mcpSafeWriteAnnotations = map[string]any{"readOnlyHint": false, "destructiveHint": false}
)

func mcpToolDefs() []mcpToolDef {
	return []mcpToolDef{
		{Name: "list_context_sources", Description: "List live read-only project context sources enabled for this Hank account.", InputSchema: mcpObjectSchema(map[string]any{}), Scopes: []string{domain.MCPScopeDocsRead}},
		{Name: "list_context_files", Description: "List files and folders inside an approved live project context source.", InputSchema: mcpObjectSchema(map[string]any{"source_id": mcpStr("Context source id."), "path": mcpStr("Optional source-relative directory path.")}, "source_id"), Scopes: []string{domain.MCPScopeDocsRead}},
		{Name: "search_context", Description: "Search filenames and approved text content in one live project context source.", InputSchema: mcpObjectSchema(map[string]any{"source_id": mcpStr("Context source id."), "query": mcpStr("Search text."), "limit": mcpInt("Maximum results, up to 50.")}, "source_id", "query"), Scopes: []string{domain.MCPScopeDocsRead}},
		{Name: "read_context_file", Description: "Read one approved text file from a live project context source.", InputSchema: mcpObjectSchema(map[string]any{"source_id": mcpStr("Context source id."), "path": mcpStr("Source-relative file path.")}, "source_id", "path"), Scopes: []string{domain.MCPScopeDocsRead}},
		{
			Name:        "list_docs",
			Description: "List the HankServerside project documents exposed to this server (README, AGENTS.md, SERVER_SYNC.md, docs/, schemas/, and a code-reference/ source snapshot). Returns relative paths to use with read_doc.",
			InputSchema: mcpObjectSchema(map[string]any{}),
			Scopes:      []string{domain.MCPScopeDocsRead},
		},
		{
			Name:        "search_docs",
			Description: "Full-text search across the HankServerside project documentation. Returns matching document paths with line-numbered snippets.",
			InputSchema: mcpObjectSchema(map[string]any{
				"query": mcpStr("Search terms (space-separated)."),
				"limit": mcpInt("Max documents to return (default 10)."),
			}, "query"),
			Scopes: []string{domain.MCPScopeDocsRead},
		},
		{
			Name:        "read_doc",
			Description: "Read the full contents of one project document. path must be a relative path returned by list_docs or search_docs (e.g. 'README.md', 'docs/architecture.md').",
			InputSchema: mcpObjectSchema(map[string]any{
				"path": mcpStr("Relative document path."),
			}, "path"),
			Scopes: []string{domain.MCPScopeDocsRead},
		},
		{
			Name:        "list_notes",
			Description: "List your Hank notes (summaries: id, title, updated_at, tags).",
			InputSchema: mcpObjectSchema(map[string]any{}),
			Scopes:      []string{domain.NotesAPIScopeRead},
		},
		{
			Name:        "search_notes",
			Description: "Search your Hank notes by text. Returns matching notes with previews.",
			InputSchema: mcpObjectSchema(map[string]any{
				"query": mcpStr("Search text."),
				"limit": mcpInt("Max results (default 20)."),
			}, "query"),
			Scopes: []string{domain.NotesAPIScopeRead},
		},
		{
			Name:        "list_note_tags",
			Description: "List tags used across your Hank notes, with counts.",
			InputSchema: mcpObjectSchema(map[string]any{}),
			Scopes:      []string{domain.NotesAPIScopeRead},
		},
		{
			Name:        "get_note",
			Description: "Fetch a single Hank note's full content by id. Returns content and revision; keep the revision if you intend to update.",
			InputSchema: mcpObjectSchema(map[string]any{
				"note_id": mcpStr("The note id."),
			}, "note_id"),
			Scopes: []string{domain.NotesAPIScopeRead},
		},
		{
			Name:        "create_note",
			Description: "Create a new Hank note with a title and body content. Use this to save a prompt, plan, or brainstorm output from this session into Hank.",
			InputSchema: mcpObjectSchema(map[string]any{
				"title":       mcpStr("Note title."),
				"content":     mcpStr("Note body (plain text / markdown)."),
				"body_format": mcpStr("Optional body format hint, e.g. 'markdown'."),
			}, "title", "content"),
			Scopes: []string{domain.NotesAPIScopeWrite},
		},
		{
			Name:        "append_note",
			Description: "Append text to the end of an existing Hank note without rewriting it.",
			InputSchema: mcpObjectSchema(map[string]any{
				"note_id":           mcpStr("The note id."),
				"content":           mcpStr("Text to append."),
				"separator":         mcpStr("Optional separator inserted before the appended text (e.g. '\\n\\n')."),
				"expected_revision": mcpStr("Optional revision for optimistic concurrency."),
			}, "note_id", "content"),
			Scopes: []string{domain.NotesAPIScopeAppend, domain.NotesAPIScopeWrite},
		},
		{
			Name:        "update_note",
			Description: "Replace the full body of an existing Hank note. To avoid clobbering concurrent edits, pass expected_revision from a recent get_note (a conflict means re-fetch first).",
			InputSchema: mcpObjectSchema(map[string]any{
				"note_id":           mcpStr("The note id."),
				"content":           mcpStr("New full body content."),
				"title":             mcpStr("Optional new title."),
				"expected_revision": mcpStr("Optional revision for optimistic concurrency."),
			}, "note_id", "content"),
			Scopes: []string{domain.NotesAPIScopeWrite},
		},
		{
			Name:        "delete_note",
			Description: "Delete a Hank note by id. This is irreversible.",
			InputSchema: mcpObjectSchema(map[string]any{
				"note_id": mcpStr("The note id."),
			}, "note_id"),
			Scopes: []string{domain.NotesAPIScopeDelete},
		},
		{
			Name: "list_kanban_boards", Description: "List your MCP-visible profile Notes Kanban boards, ordered columns, workflow roles, intake configuration, card counts, revisions, and usable default board.",
			InputSchema: mcpObjectSchema(map[string]any{}), Scopes: []string{domain.NotesAPIScopeRead}, Annotations: mcpReadOnlyAnnotations,
		},
		{
			Name: "list_kanban_cards", Description: "List and filter ordered cards on an exact board_id or the configured default Kanban board. Completed cards are hidden unless requested. After a human handoff, list the configured intake column and continue with its first ordered unblocked card.",
			InputSchema: mcpObjectSchema(map[string]any{
				"board_id":         mcpStr("Optional exact board ID; defaults to the configured default board."),
				"column_id":        mcpStr("Optional exact column ID."),
				"role":             mcpEnum("Optional semantic column role.", "planning", "active", "rework", "human", "review", "complete"),
				"query":            mcpStr("Optional case-insensitive title and Markdown details query."),
				"tags":             mcpStringArray("Optional tags; every supplied tag must match."),
				"due_from":         mcpStr("Optional inclusive YYYY-MM-DD lower due-date bound."),
				"due_through":      mcpStr("Optional inclusive YYYY-MM-DD upper due-date bound."),
				"include_complete": mcpBool("Include cards in a column with the complete role."),
				"limit":            map[string]any{"type": "integer", "description": "Maximum results; defaults to 50.", "minimum": 1, "maximum": 100},
			}), Scopes: []string{domain.NotesAPIScopeRead}, Annotations: mcpReadOnlyAnnotations,
		},
		{
			Name: "get_kanban_card", Description: "Read one Kanban card by exact card_id from an exact board_id or the configured default board, including Markdown details and workflow context.",
			InputSchema: mcpObjectSchema(map[string]any{
				"board_id": mcpStr("Optional exact board ID; defaults to the configured default board."),
				"card_id":  mcpStr("Exact card ID returned by a Kanban read tool."),
			}, "card_id"), Scopes: []string{domain.NotesAPIScopeRead}, Annotations: mcpReadOnlyAnnotations,
		},
		{
			Name: "create_kanban_card", Description: "Create a Kanban card. Call only after the user explicitly asks to capture a task. Uses an exact destination or the configured default board and intake column.",
			InputSchema: mcpObjectSchema(map[string]any{
				"board_id":         mcpStr("Optional exact board ID; defaults to the configured default board."),
				"column_id":        mcpStr("Optional exact destination column ID; defaults to intake then first column."),
				"title":            mcpStr("Required card title."),
				"details_markdown": mcpStr("Optional Markdown details."),
				"due_date":         mcpStr("Optional YYYY-MM-DD due date."),
				"tags":             mcpStringArray("Optional card tags."),
			}, "title"), Scopes: []string{domain.NotesAPIScopeWrite}, Annotations: mcpSafeWriteAnnotations,
		},
		{
			Name: "update_kanban_card", Description: "Patch supplied fields on one exact Kanban card without rewriting unrelated card or board data.",
			InputSchema: mcpObjectSchema(map[string]any{
				"board_id":         mcpStr("Optional exact board ID; defaults to the configured default board."),
				"card_id":          mcpStr("Exact card ID returned by a Kanban read tool."),
				"title":            mcpStr("Optional replacement title; cannot be empty."),
				"details_markdown": mcpStr("Optional replacement Markdown details."),
				"due_date":         mcpStr("Optional YYYY-MM-DD due date; empty clears it."),
				"tags":             mcpStringArray("Optional replacement tags; empty clears them."),
			}, "card_id"), Scopes: []string{domain.NotesAPIScopeWrite}, Annotations: mcpSafeWriteAnnotations,
		},
		{
			Name: "append_kanban_worklog", Description: "Append a server-dated progress, verification, blocker, or outcome entry while preserving the card's original Markdown details. Before a human handoff, use a blocker entry to preserve the decision, approval, or review needed.",
			InputSchema: mcpObjectSchema(map[string]any{
				"board_id":       mcpStr("Optional exact board ID; defaults to the configured default board."),
				"card_id":        mcpStr("Exact card ID returned by a Kanban read tool."),
				"entry_markdown": mcpStr("Markdown work-log entry to append."),
				"kind":           mcpEnum("Work-log entry kind.", "progress", "verification", "blocker", "outcome"),
			}, "card_id", "entry_markdown", "kind"), Scopes: []string{domain.NotesAPIScopeWrite}, Annotations: mcpSafeWriteAnnotations,
		},
		{
			Name: "move_kanban_card", Description: "Move or reorder one exact Kanban card in an exact destination column. For a human handoff, prefer the human role for unfinished decisions or approvals and the review role for completed work awaiting validation, falling back to the other configured role. Then continue with the next intake card rather than waiting. The server does not impose a workflow order.",
			InputSchema: mcpObjectSchema(map[string]any{
				"board_id":         mcpStr("Optional exact board ID; defaults to the configured default board."),
				"card_id":          mcpStr("Exact card ID returned by a Kanban read tool."),
				"target_column_id": mcpStr("Exact destination column ID returned by a Kanban read tool."),
				"target_index":     map[string]any{"type": "integer", "description": "Optional zero-based destination index; defaults to the end.", "minimum": 0},
			}, "card_id", "target_column_id"), Scopes: []string{domain.NotesAPIScopeWrite}, Annotations: mcpSafeWriteAnnotations,
		},
	}
}

func mcpToolList() []map[string]any {
	defs := mcpToolDefs()
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		item := map[string]any{
			"name":        d.Name,
			"description": d.Description,
			"inputSchema": d.InputSchema,
		}
		if len(d.Annotations) > 0 {
			item["annotations"] = d.Annotations
		}
		out = append(out, item)
	}
	return out
}

func mcpToolByName(name string) (mcpToolDef, bool) {
	for _, d := range mcpToolDefs() {
		if d.Name == name {
			return d, true
		}
	}
	return mcpToolDef{}, false
}
