package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

// These tests cover the DB-independent MCP logic and run without Postgres.

func TestMCPVerifyPKCES256(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk1234567890abcdef"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if !verifyPKCES256(verifier, challenge) {
		t.Fatalf("expected PKCE verification to pass")
	}
	if verifyPKCES256("wrong-verifier", challenge) {
		t.Fatalf("expected PKCE verification to fail for wrong verifier")
	}
	if verifyPKCES256(verifier, "not-the-challenge") {
		t.Fatalf("expected PKCE verification to fail for wrong challenge")
	}
}

func TestMCPScopeHelpers(t *testing.T) {
	got := mcpFilterScopes([]string{"notes:read", "bogus", "notes:read", "docs:read", ""})
	want := []string{"notes:read", "docs:read"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("mcpFilterScopes = %v, want %v", got, want)
	}
	if mcpParseScopeParam("  ") != nil {
		t.Fatalf("empty scope should parse to nil")
	}
	inter := mcpIntersectScopes([]string{"a", "notes:read", "notes:write"}, []string{"notes:read"})
	if strings.Join(inter, ",") != "notes:read" {
		t.Fatalf("intersect = %v", inter)
	}
	auth := mcpAuthContext{Token: domain.MCPToken{Scopes: []string{"notes:append"}}}
	if !mcpAuthHasAnyScope(auth, []string{"notes:append", "notes:write"}) {
		t.Fatalf("expected any-of scope match")
	}
	if mcpAuthHasAnyScope(auth, []string{"notes:delete"}) {
		t.Fatalf("did not expect delete scope")
	}
}

func TestMCPValidRedirectURI(t *testing.T) {
	cases := map[string]bool{
		"https://chatgpt.com/connector_platform_oauth_redirect": true,
		"https://claude.ai/api/mcp/auth_callback":               true,
		"http://localhost:8080/callback":                        true,
		"http://127.0.0.1/cb":                                   true,
		"http://evil.example.com/cb":                            false,
		"claudeai://mcp/callback":                               true,
		"":                                                      false,
		"https://host/cb#frag":                                  false,
	}
	for uri, want := range cases {
		if got := mcpValidRedirectURI(uri); got != want {
			t.Errorf("mcpValidRedirectURI(%q) = %v, want %v", uri, got, want)
		}
	}
}

func TestMCPDocsIndex(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "# Readme\nThe notes api token lives here\n")
	mustWrite(t, filepath.Join(root, "docs", "architecture.md"), "# Arch\nthe notes service design\n")
	mustWrite(t, filepath.Join(root, ".env.cloud"), "SECRET=should-never-be-exposed\n")
	mustWrite(t, filepath.Join(root, "docs", "secret.bin"), "binary")
	outside := filepath.Join(t.TempDir(), "outside.md")
	mustWrite(t, outside, "outside secret")
	if err := os.Symlink(outside, filepath.Join(root, "docs", "outside.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// code-reference/ source snapshot: .go is exposed, but non-text files are not.
	mustWrite(t, filepath.Join(root, "code-reference", "internal", "cloud", "server.go"), "package cloud\n// the notes handler lives here\n")
	mustWrite(t, filepath.Join(root, "code-reference", "internal", "cloud", "server.test.bin"), "binary")

	idx := newMCPDocsIndex(root)
	paths := idx.listPaths()
	joined := strings.Join(paths, ",")
	if !strings.Contains(joined, "README.md") || !strings.Contains(joined, "docs/architecture.md") {
		t.Fatalf("listPaths missing expected docs: %v", paths)
	}
	if !strings.Contains(joined, "code-reference/internal/cloud/server.go") {
		t.Fatalf("listPaths missing code-reference source: %v", paths)
	}
	if strings.Contains(joined, ".env.cloud") || strings.Contains(joined, "secret.bin") || strings.Contains(joined, "server.test.bin") || strings.Contains(joined, "outside.md") {
		t.Fatalf("listPaths exposed disallowed file: %v", paths)
	}

	body, err := idx.read("README.md")
	if err != nil || !strings.Contains(body, "Readme") {
		t.Fatalf("read README: %v / %q", err, body)
	}
	if body, err := idx.read("code-reference/internal/cloud/server.go"); err != nil || !strings.Contains(body, "package cloud") {
		t.Fatalf("read code-reference source: %v / %q", err, body)
	}
	if _, err := idx.read("../etc/passwd"); err == nil {
		t.Fatalf("expected path traversal to be refused")
	}
	if _, err := idx.read(".env.cloud"); err == nil {
		t.Fatalf("expected non-allowlisted file to be refused")
	}
	if _, err := idx.read("docs/outside.md"); err == nil {
		t.Fatalf("expected symlinked document to be refused")
	}
	out, err := idx.search("notes", 5)
	if err != nil || !strings.Contains(out, "README.md") {
		t.Fatalf("search: %v / %q", err, out)
	}
	if _, err := idx.search("  ", 5); err == nil {
		t.Fatalf("expected empty query to error")
	}
}

func TestMCPToolListAndLookup(t *testing.T) {
	defs := mcpToolDefs()
	if len(defs) != 22 {
		t.Fatalf("expected 22 tools, got %d", len(defs))
	}
	for _, name := range []string{"list_docs", "search_docs", "read_doc", "create_note", "delete_note", "append_note", "list_context_sources", "list_context_files", "search_context", "read_context_file", "list_kanban_boards", "list_kanban_cards", "get_kanban_card", "create_kanban_card", "update_kanban_card", "append_kanban_worklog", "move_kanban_card"} {
		if _, ok := mcpToolByName(name); !ok {
			t.Fatalf("tool %q not found", name)
		}
	}
	if _, ok := mcpToolByName("nope"); ok {
		t.Fatalf("did not expect to find bogus tool")
	}
	// append_note must accept either append or write scope.
	def, _ := mcpToolByName("append_note")
	if len(def.Scopes) != 2 {
		t.Fatalf("append_note scopes = %v", def.Scopes)
	}
	for _, tool := range mcpToolList() {
		schema, _ := tool["inputSchema"].(map[string]any)
		if schema["type"] != "object" {
			t.Fatalf("tool %v has non-object inputSchema", tool["name"])
		}
		name, _ := tool["name"].(string)
		if strings.Contains(name, "kanban") {
			annotations, _ := tool["annotations"].(map[string]any)
			readOnly, _ := annotations["readOnlyHint"].(bool)
			isRead := strings.HasPrefix(name, "list_") || strings.HasPrefix(name, "get_")
			if readOnly != isRead {
				t.Fatalf("tool %s annotations = %#v", name, annotations)
			}
			if !isRead && annotations["destructiveHint"] != false {
				t.Fatalf("tool %s must be non-destructive: %#v", name, annotations)
			}
		}
	}
	readDef, _ := mcpToolByName("list_kanban_boards")
	writeDef, _ := mcpToolByName("create_kanban_card")
	if strings.Join(readDef.Scopes, ",") != domain.NotesAPIScopeRead || strings.Join(writeDef.Scopes, ",") != domain.NotesAPIScopeWrite {
		t.Fatalf("Kanban scopes read=%v write=%v", readDef.Scopes, writeDef.Scopes)
	}
	listCardsDef, _ := mcpToolByName("list_kanban_cards")
	worklogDef, _ := mcpToolByName("append_kanban_worklog")
	moveDef, _ := mcpToolByName("move_kanban_card")
	workflowDescriptions := strings.ToLower(strings.Join([]string{
		listCardsDef.Description,
		worklogDef.Description,
		moveDef.Description,
	}, " "))
	for _, required := range []string{"human", "review", "intake", "continue"} {
		if !strings.Contains(workflowDescriptions, required) {
			t.Fatalf("Kanban workflow descriptions missing %q: %s", required, workflowDescriptions)
		}
	}
}

func TestMCPInitializeAndDispatchNoDB(t *testing.T) {
	s := &Server{}

	// version negotiation echoes a supported requested version
	res := s.mcpInitializeResult(json.RawMessage(`{"protocolVersion":"2025-03-26"}`))
	if res["protocolVersion"] != "2025-03-26" {
		t.Fatalf("expected echoed protocol version, got %v", res["protocolVersion"])
	}
	res = s.mcpInitializeResult(json.RawMessage(`{"protocolVersion":"1999-01-01"}`))
	if res["protocolVersion"] != mcpProtocolVersion {
		t.Fatalf("expected default protocol version, got %v", res["protocolVersion"])
	}
	if !strings.Contains(res["instructions"].(string), "Kanban") {
		t.Fatalf("initialize instructions do not advertise Kanban tools: %v", res["instructions"])
	}
	instructions := strings.ToLower(res["instructions"].(string))
	for _, required := range []string{"human approval", "needs human", "review", "next ordered intake card", "rather than waiting"} {
		if !strings.Contains(instructions, required) {
			t.Fatalf("initialize instructions missing %q: %s", required, instructions)
		}
	}

	auth := mcpAuthContext{}
	// tools/list returns the tool list
	resp, has := s.handleMCPMessage(context.Background(), auth, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if !has {
		t.Fatalf("tools/list should produce a response")
	}
	if _, ok := resp.(map[string]any)["result"]; !ok {
		t.Fatalf("tools/list missing result: %v", resp)
	}
	// initialized notification produces no response
	if _, has := s.handleMCPMessage(context.Background(), auth, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); has {
		t.Fatalf("notification should not produce a response")
	}
	// ping
	if _, has := s.handleMCPMessage(context.Background(), auth, []byte(`{"jsonrpc":"2.0","id":2,"method":"ping"}`)); !has {
		t.Fatalf("ping should produce a response")
	}
	// unknown method -> error response
	resp, _ = s.handleMCPMessage(context.Background(), auth, []byte(`{"jsonrpc":"2.0","id":3,"method":"bogus"}`))
	if _, ok := resp.(map[string]any)["error"]; !ok {
		t.Fatalf("unknown method should be a JSON-RPC error: %v", resp)
	}
}

type staticMCPKanbanStore struct {
	notes    []domain.UserNote
	settings domain.UserProfileSettings
}

func (s staticMCPKanbanStore) ListProfileNotes(context.Context, string, bool) ([]domain.UserNote, error) {
	return s.notes, nil
}

func (s staticMCPKanbanStore) GetUserProfileSettings(context.Context, string) (domain.UserProfileSettings, error) {
	return s.settings, nil
}

type staticMCPKanbanNotes struct{ fetched protocol.NotesFetchResponse }

func (s *staticMCPKanbanNotes) FetchProfile(context.Context, string, string) (protocol.NotesFetchResponse, error) {
	return s.fetched, nil
}

func (s *staticMCPKanbanNotes) SaveProfile(context.Context, string, string, protocol.NotesSaveRequest) (protocol.NotesSaveResponse, error) {
	return protocol.NotesSaveResponse{}, errors.New("unexpected save")
}

func TestMCPDispatchesKanbanReadWithoutDB(t *testing.T) {
	board := testMCPKanbanBoard()
	kanbanStore := staticMCPKanbanStore{
		notes:    []domain.UserNote{{NoteID: "work", OwnerUserID: "u1", Title: "Work", PageType: protocol.NotePageTypeKanban}},
		settings: domain.UserProfileSettings{Settings: json.RawMessage(`{"kanban_default_board_id":"work"}`)},
	}
	kanbanNotes := &staticMCPKanbanNotes{fetched: protocol.NotesFetchResponse{NoteID: "work", Title: "Work", Revision: "1", PageType: protocol.NotePageTypeKanban, Board: board}}
	s := &Server{kanban: newMCPKanbanService(kanbanStore, kanbanNotes, time.Now)}
	auth := mcpAuthContext{User: domain.User{ID: "u1"}, Token: domain.MCPToken{Scopes: []string{domain.NotesAPIScopeRead}}}

	result := s.mcpToolsCall(context.Background(), auth, json.RawMessage(`{"name":"list_kanban_boards","arguments":{}}`))
	if result["isError"] == true || !strings.Contains(mcpFirstText(result), `"board_id": "work"`) {
		t.Fatalf("list_kanban_boards = %#v", result)
	}
}

func TestMCPKanbanAuditMetadataContainsOnlyIdentifiers(t *testing.T) {
	result := mcpKanbanCardResult{
		BoardID: "work", CardID: "card-1", ColumnID: "review",
		Title: "Secret title", DetailsMarkdown: "Secret details", Tags: []string{"Secret"},
	}
	metadata := mcpKanbanAuditMetadata("move_kanban_card", "chatgpt", result, map[string]string{"source_column_id": "active"})
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{"Secret title", "Secret details", "Secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("audit metadata leaked card content: %s", text)
		}
	}
	for _, identifier := range []string{"work", "card-1", "review", "active", "chatgpt"} {
		if !strings.Contains(text, identifier) {
			t.Fatalf("audit metadata missing %q: %s", identifier, text)
		}
	}
}

func TestMCPKanbanArgumentsDecodeSnakeCaseFields(t *testing.T) {
	var create mcpKanbanCreateArgs
	if err := decodeMCPToolArgs(json.RawMessage(`{"board_id":"work","column_id":"ideas","title":"Task","details_markdown":"Details","due_date":"2026-07-25","tags":["Hank"]}`), &create); err != nil {
		t.Fatal(err)
	}
	if create.BoardID != "work" || create.ColumnID != "ideas" || create.DetailsMarkdown != "Details" || create.DueDate != "2026-07-25" {
		t.Fatalf("create args = %#v", create)
	}
	var move mcpKanbanMoveArgs
	if err := decodeMCPToolArgs(json.RawMessage(`{"board_id":"work","card_id":"card-1","target_column_id":"review","target_index":2}`), &move); err != nil {
		t.Fatal(err)
	}
	if move.BoardID != "work" || move.CardID != "card-1" || move.TargetColumnID != "review" || move.TargetIndex == nil || *move.TargetIndex != 2 {
		t.Fatalf("move args = %#v", move)
	}
}

func TestMCPExecuteDocsToolsNoDB(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "# Readme\nsearchable docs content\n")
	s := &Server{mcpDocs: newMCPDocsIndex(root)}
	auth := mcpAuthContext{
		User:  domain.User{ID: "u1"},
		Token: domain.MCPToken{Scopes: []string{domain.MCPScopeDocsRead}},
	}

	// scope gating: docs tool allowed, notes tool denied
	res := s.mcpToolsCall(context.Background(), auth, json.RawMessage(`{"name":"list_docs","arguments":{}}`))
	if res["isError"] == true {
		t.Fatalf("list_docs should succeed: %v", res)
	}
	res = s.mcpToolsCall(context.Background(), auth, json.RawMessage(`{"name":"create_note","arguments":{"title":"x","content":"y"}}`))
	if res["isError"] != true {
		t.Fatalf("create_note should be denied without notes:write scope")
	}
	res = s.mcpToolsCall(context.Background(), auth, json.RawMessage(`{"name":"search_docs","arguments":{"query":"searchable"}}`))
	text := mcpFirstText(res)
	if !strings.Contains(text, "README.md") {
		t.Fatalf("search_docs missing hit: %q", text)
	}
}

func TestMCPExcludedVisibilityHelpers(t *testing.T) {
	notes := []domain.UserNote{
		{NoteID: "private-notebook", Title: "Private Notebook", PageType: "notebook", MCPExcluded: true},
		{NoteID: "child-hidden.md", Title: "Hidden Child", PageType: "text", ParentID: "private-notebook"},
		{NoteID: "private-note.md", Title: "Private Note", PageType: "text", MCPExcluded: true},
		{NoteID: "visible-notebook", Title: "Visible Notebook", PageType: "notebook"},
		{NoteID: "child-visible.md", Title: "Visible Child", PageType: "text", ParentID: "visible-notebook"},
		{NoteID: "visible-note.md", Title: "Visible Note", PageType: "text"},
	}

	visible := mcpVisibleProfileNotes(notes)
	gotIDs := make([]string, 0, len(visible))
	for _, note := range visible {
		gotIDs = append(gotIDs, note.NoteID)
	}
	if strings.Join(gotIDs, ",") != "visible-notebook,child-visible.md,visible-note.md" {
		t.Fatalf("mcpVisibleProfileNotes ids = %v", gotIDs)
	}

	for _, tc := range []struct {
		noteID string
		want   bool
	}{
		{noteID: "private-notebook", want: false},
		{noteID: "child-hidden.md", want: false},
		{noteID: "private-note.md", want: false},
		{noteID: "child-visible.md", want: true},
		{noteID: "visible-note.md", want: true},
		{noteID: "missing.md", want: false},
	} {
		if got := mcpNoteVisible(notes, tc.noteID); got != tc.want {
			t.Fatalf("mcpNoteVisible(%q) = %v, want %v", tc.noteID, got, tc.want)
		}
	}
}

func mcpFirstText(res map[string]any) string {
	content, _ := res["content"].([]map[string]any)
	if len(content) == 0 {
		return ""
	}
	s, _ := content[0]["text"].(string)
	return s
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
