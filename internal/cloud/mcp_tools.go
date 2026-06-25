package cloud

import "github.com/dropfile/hankremote/internal/domain"

// mcpToolDef describes one MCP tool: its advertised schema and the scopes that
// authorize it (any-of). The execution lives in executeMCPTool.
type mcpToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
	Scopes      []string
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

func mcpToolDefs() []mcpToolDef {
	return []mcpToolDef{
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
	}
}

func mcpToolList() []map[string]any {
	defs := mcpToolDefs()
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]any{
			"name":        d.Name,
			"description": d.Description,
			"inputSchema": d.InputSchema,
		})
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
