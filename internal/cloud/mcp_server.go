package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

// MCP Streamable HTTP endpoint (JSON-RPC 2.0). Mirrors the Phase-1 stdio server
// but reuses the cloud notes service and project docs directly, gated by the
// per-user OAuth access token's scopes.

const mcpProtocolVersion = "2025-11-25"

var mcpSupportedProtocolVersions = map[string]bool{
	"2025-11-25": true,
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
}

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func (s *Server) handleMCPEndpoint(w http.ResponseWriter, r *http.Request) {
	if !s.mcpEnabled {
		http.NotFound(w, r)
		return
	}
	s.logger.Info("mcp endpoint request",
		"method", r.Method,
		"has_auth", r.Header.Get("Authorization") != "",
		"accept", r.Header.Get("Accept"),
		"content_type", r.Header.Get("Content-Type"),
		"mcp_protocol", r.Header.Get("MCP-Protocol-Version"),
		"ua", r.UserAgent(),
	)
	if r.Method != http.MethodPost {
		// No server-initiated SSE stream is offered; all traffic is request/response over POST.
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireMCPAuth(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHTTPBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, jsonrpcErrorResponse(nil, -32700, "could not read request body"))
		return
	}
	trimmed := bytes.TrimSpace(body)

	// JSON-RPC batch (array) support.
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var raws []json.RawMessage
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			writeJSON(w, http.StatusBadRequest, jsonrpcErrorResponse(nil, -32700, "parse error"))
			return
		}
		var responses []any
		for _, raw := range raws {
			if resp, has := s.handleMCPMessage(r.Context(), auth, raw); has {
				responses = append(responses, resp)
			}
		}
		if len(responses) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, http.StatusOK, responses)
		return
	}

	resp, has := s.handleMCPMessage(r.Context(), auth, trimmed)
	if !has {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMCPMessage(ctx context.Context, auth mcpAuthContext, raw []byte) (any, bool) {
	var req jsonrpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return jsonrpcErrorResponse(nil, -32700, "parse error"), true
	}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		return jsonrpcResultResponse(req.ID, s.mcpInitializeResult(req.Params)), true
	case "notifications/initialized":
		return nil, false
	case "ping":
		return jsonrpcResultResponse(req.ID, map[string]any{}), true
	case "tools/list":
		return jsonrpcResultResponse(req.ID, map[string]any{"tools": mcpToolList()}), true
	case "tools/call":
		return jsonrpcResultResponse(req.ID, s.mcpToolsCall(ctx, auth, req.Params)), true
	default:
		if isNotification {
			return nil, false
		}
		return jsonrpcErrorResponse(req.ID, -32601, "Method not found: "+req.Method), true
	}
}

func (s *Server) mcpInitializeResult(params json.RawMessage) map[string]any {
	version := mcpProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &p); err == nil && mcpSupportedProtocolVersions[p.ProtocolVersion] {
			version = p.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "hank-mcp", "version": "0.1.0"},
		"instructions": "Hank project context. Use list_docs/search_docs/read_doc to read HankServerside " +
			"documentation, and the *_note tools to read and write your Hank notes (e.g. save a plan with " +
			"create_note, or read one with get_note and act on it).",
	}
}

// --- tool results / JSON-RPC envelope helpers ---

func jsonrpcResultResponse(id json.RawMessage, result any) map[string]any {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

func jsonrpcErrorResponse(id json.RawMessage, code int, message string) map[string]any {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}}
}

func mcpToolText(text string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": false}
}

func mcpToolError(message string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": message}}, "isError": true}
}

// --- tool dispatch ---

func (s *Server) mcpToolsCall(ctx context.Context, auth mcpAuthContext, params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return mcpToolError("invalid tools/call params")
	}
	def, ok := mcpToolByName(p.Name)
	if !ok {
		return mcpToolError("Unknown tool: " + p.Name)
	}
	if !mcpAuthHasAnyScope(auth, def.Scopes) {
		return mcpToolError("This connection is not authorized for " + p.Name + " (missing required scope).")
	}
	text, err := s.executeMCPTool(ctx, auth, p.Name, p.Arguments)
	if err != nil {
		return mcpToolError(mcpFriendlyError(err))
	}
	return mcpToolText(text)
}

func mcpAuthHasAnyScope(auth mcpAuthContext, scopes []string) bool {
	for _, sc := range scopes {
		if auth.hasScope(sc) {
			return true
		}
	}
	return false
}

func mcpFriendlyError(err error) string {
	if errors.Is(err, store.ErrNotFound) {
		return "not found"
	}
	return err.Error()
}

func (s *Server) executeMCPTool(ctx context.Context, auth mcpAuthContext, name string, rawArgs json.RawMessage) (string, error) {
	userID := auth.User.ID
	profileNotes := func() ([]domain.UserNote, error) {
		return s.store.ListProfileNotes(ctx, userID, false)
	}
	requireVisibleProfileNote := func(noteID string) error {
		notes, err := profileNotes()
		if err != nil {
			return err
		}
		if !mcpNoteVisible(notes, noteID) {
			return store.ErrNotFound
		}
		return nil
	}
	switch name {
	case "list_docs":
		paths := s.mcpDocs.listPaths()
		if len(paths) == 0 {
			return "No documents are exposed. Check the MCP docs directory configuration.", nil
		}
		return "Exposed documents (" + strconv.Itoa(len(paths)) + "):\n\n" + joinLines(paths), nil
	case "search_docs":
		var a struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		return s.mcpDocs.search(a.Query, a.Limit)
	case "read_doc":
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		return s.mcpDocs.read(a.Path)
	case "list_notes":
		notes, err := profileNotes()
		if err != nil {
			return "", err
		}
		return jsonText(map[string]any{"notes": noteSummaries(mcpVisibleProfileNotes(notes))})
	case "search_notes":
		var a struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if a.Limit <= 0 {
			a.Limit = 20
		}
		notes, err := profileNotes()
		if err != nil {
			return "", err
		}
		return jsonText(map[string]any{"results": searchNotes(mcpVisibleProfileNotes(notes), a.Query, a.Limit, "")})
	case "list_note_tags":
		notes, err := profileNotes()
		if err != nil {
			return "", err
		}
		return jsonText(map[string]any{"tags": noteTags(mcpVisibleProfileNotes(notes))})
	case "get_note":
		var a struct {
			NoteID string `json:"note_id"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if a.NoteID == "" {
			return "", errors.New("note_id is required")
		}
		if err := requireVisibleProfileNote(a.NoteID); err != nil {
			return "", err
		}
		note, err := s.notes.FetchProfile(ctx, userID, a.NoteID)
		if err != nil {
			return "", err
		}
		return jsonText(note)
	case "create_note":
		var a struct {
			Title      string `json:"title"`
			Content    string `json:"content"`
			BodyFormat string `json:"body_format"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if a.Title == "" {
			return "", errors.New("title is required")
		}
		resp, err := s.notes.SaveProfile(ctx, userID, "", protocol.NotesSaveRequest{
			Title:      a.Title,
			Content:    a.Content,
			BodyFormat: a.BodyFormat,
		})
		if err != nil {
			return "", err
		}
		s.auditMCPWrite(ctx, auth, "create_note", resp.NoteID)
		return jsonText(resp)
	case "update_note":
		var a struct {
			NoteID           string `json:"note_id"`
			Content          string `json:"content"`
			Title            string `json:"title"`
			ExpectedRevision string `json:"expected_revision"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if a.NoteID == "" {
			return "", errors.New("note_id is required")
		}
		if err := requireVisibleProfileNote(a.NoteID); err != nil {
			return "", err
		}
		resp, err := s.notes.SaveProfile(ctx, userID, a.NoteID, protocol.NotesSaveRequest{
			NoteID:           a.NoteID,
			Title:            a.Title,
			Content:          a.Content,
			ExpectedRevision: a.ExpectedRevision,
		})
		if err != nil {
			return "", err
		}
		s.auditMCPWrite(ctx, auth, "update_note", resp.NoteID)
		return jsonText(resp)
	case "append_note":
		var a struct {
			NoteID           string  `json:"note_id"`
			Content          string  `json:"content"`
			Separator        *string `json:"separator"`
			ExpectedRevision string  `json:"expected_revision"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if a.NoteID == "" {
			return "", errors.New("note_id is required")
		}
		if err := requireVisibleProfileNote(a.NoteID); err != nil {
			return "", err
		}
		resp, err := s.notes.AppendProfile(ctx, userID, a.NoteID, protocol.NotesAppendRequest{
			Content:          a.Content,
			Separator:        a.Separator,
			ExpectedRevision: a.ExpectedRevision,
		})
		if err != nil {
			return "", err
		}
		s.auditMCPWrite(ctx, auth, "append_note", resp.NoteID)
		return jsonText(resp)
	case "delete_note":
		var a struct {
			NoteID string `json:"note_id"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if a.NoteID == "" {
			return "", errors.New("note_id is required")
		}
		if err := requireVisibleProfileNote(a.NoteID); err != nil {
			return "", err
		}
		if err := s.notes.DeleteProfile(ctx, userID, a.NoteID); err != nil {
			return "", err
		}
		s.auditMCPWrite(ctx, auth, "delete_note", a.NoteID)
		return jsonText(map[string]any{"ok": true, "note_id": a.NoteID})
	default:
		return "", errors.New("unknown tool: " + name)
	}
}

func mcpVisibleProfileNotes(notes []domain.UserNote) []domain.UserNote {
	visible := make([]domain.UserNote, 0, len(notes))
	for _, note := range notes {
		if mcpNoteVisible(notes, note.NoteID) {
			visible = append(visible, note)
		}
	}
	return visible
}

func mcpNoteVisible(notes []domain.UserNote, noteID string) bool {
	byID := make(map[string]domain.UserNote, len(notes))
	for _, note := range notes {
		byID[note.NoteID] = note
	}
	note, ok := byID[noteID]
	if !ok || note.DeletedAt != nil || note.MCPExcluded {
		return false
	}
	if note.ParentID == "" {
		return true
	}
	parent, ok := byID[note.ParentID]
	if !ok || parent.DeletedAt != nil {
		return true
	}
	if normalizePageType(parent.PageType) != protocol.NotePageTypeNotebook {
		return true
	}
	return !parent.MCPExcluded
}

func (s *Server) auditMCPWrite(ctx context.Context, auth mcpAuthContext, tool string, noteID string) {
	s.audit(ctx, "mcp.tool_called", auditSeverityInfo, auth.User.ID, "", "", requestIDFromContext(ctx), "mcp_note", noteID, map[string]any{
		"tool":      tool,
		"client_id": auth.Token.ClientID,
	})
}

func jsonText(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func joinLines(items []string) string {
	var b bytes.Buffer
	for i, it := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(it)
	}
	return b.String()
}
