package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

// TestMCPOAuthEndToEndFlow exercises the full remote MCP path against a real
// store: DCR -> authorize/consent -> token (PKCE) -> POST /v1/mcp tools, plus
// scope enforcement, code single-use, unauthorized discovery, and refresh.
func TestMCPOAuthEndToEndFlow(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_mcp_flow", Email: "flow@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))
	sessionRaw := "mcp-flow-session"
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_mcp_flow", UserID: user.ID, TokenHash: hashToken(sessionRaw), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))

	docsRoot := t.TempDir()
	mustWrite(t, filepath.Join(docsRoot, "README.md"), "# Hank\nproject knowledge for search\n")

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.ConfigureMCP(MCPConfig{Enabled: true, PublicBaseURL: "https://hank.test", DocsDir: docsRoot})
	ts := httptest.NewServer(server.http.Handler)
	defer ts.Close()

	redirectURI := "https://chatgpt.com/connector_platform_oauth_redirect"

	// --- discovery metadata ---
	var prm map[string]any
	mcpGetJSON(t, ts, "/.well-known/oauth-protected-resource", &prm)
	if prm["resource"] != "https://hank.test/v1/mcp" {
		t.Fatalf("protected-resource metadata resource = %v", prm["resource"])
	}
	var asm map[string]any
	mcpGetJSON(t, ts, "/.well-known/oauth-authorization-server", &asm)
	if asm["token_endpoint"] != "https://hank.test/v1/oauth/mcp/token" {
		t.Fatalf("authorization-server metadata token_endpoint = %v", asm["token_endpoint"])
	}

	// --- dynamic client registration ---
	var reg struct {
		ClientID string `json:"client_id"`
	}
	mcpPostJSON(t, ts, "/v1/oauth/mcp/register", map[string]any{
		"redirect_uris": []string{redirectURI},
		"client_name":   "ChatGPT",
	}, http.StatusCreated, &reg)
	if reg.ClientID == "" {
		t.Fatalf("registration returned no client_id")
	}

	// --- authorize + consent ---
	verifier := "test-verifier-abcdefghijklmnopqrstuvwxyz-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	form := url.Values{}
	form.Set("response_type", "code")
	form.Set("client_id", reg.ClientID)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_challenge", challenge)
	form.Set("code_challenge_method", "S256")
	form.Set("state", "state-xyz")
	form.Set("decision", "allow")
	form.Set("csrf_token", "csrftok")
	for _, sc := range []string{"docs:read", "notes:read", "notes:append", "notes:write"} {
		form.Add("scope_grant", sc) // deliberately NOT granting notes:delete
	}

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	areq, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/oauth/mcp/authorize", strings.NewReader(form.Encode()))
	areq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	areq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionRaw})
	areq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrftok"})
	aresp, err := noRedirect.Do(areq)
	if err != nil {
		t.Fatalf("authorize POST: %v", err)
	}
	defer aresp.Body.Close()
	if aresp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(aresp.Body)
		t.Fatalf("authorize status = %d body = %s", aresp.StatusCode, body)
	}
	loc, _ := url.Parse(aresp.Header.Get("Location"))
	code := loc.Query().Get("code")
	if code == "" || loc.Query().Get("state") != "state-xyz" {
		t.Fatalf("authorize redirect missing code/state: %s", aresp.Header.Get("Location"))
	}

	// --- token exchange (PKCE) ---
	tokenForm := url.Values{}
	tokenForm.Set("grant_type", "authorization_code")
	tokenForm.Set("code", code)
	tokenForm.Set("client_id", reg.ClientID)
	tokenForm.Set("redirect_uri", redirectURI)
	tokenForm.Set("code_verifier", verifier)
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	mcpPostForm(t, ts, "/v1/oauth/mcp/token", tokenForm, http.StatusOK, &tok)
	if tok.AccessToken == "" || tok.TokenType != "Bearer" {
		t.Fatalf("token response = %+v", tok)
	}
	if strings.Contains(tok.Scope, "notes:delete") {
		t.Fatalf("granted scope unexpectedly includes delete: %q", tok.Scope)
	}

	// code is single-use
	var reuse map[string]any
	if status := mcpPostFormStatus(t, ts, "/v1/oauth/mcp/token", tokenForm, &reuse); status != http.StatusBadRequest {
		t.Fatalf("authorization code reuse should fail, got %d", status)
	}

	// --- unauthorized MCP endpoint advertises discovery ---
	unauth, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	uresp, err := http.DefaultClient.Do(unauth)
	if err != nil {
		t.Fatalf("unauth mcp: %v", err)
	}
	uresp.Body.Close()
	if uresp.StatusCode != http.StatusUnauthorized || !strings.Contains(uresp.Header.Get("WWW-Authenticate"), "resource_metadata") {
		t.Fatalf("expected 401 + WWW-Authenticate, got %d / %q", uresp.StatusCode, uresp.Header.Get("WWW-Authenticate"))
	}

	call := func(body string) map[string]any {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("mcp call: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode mcp response: %v", err)
		}
		return out
	}

	if init := call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`); init["result"] == nil {
		t.Fatalf("initialize failed: %v", init)
	}

	created := call(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_note","arguments":{"title":"From MCP","content":"hello from mcp"}}}`)
	if mcpToolIsError(created) {
		t.Fatalf("create_note failed: %v", created)
	}
	var noteResp struct {
		NoteID string `json:"note_id"`
	}
	if err := json.Unmarshal([]byte(mcpResultText(created)), &noteResp); err != nil || noteResp.NoteID == "" {
		t.Fatalf("create_note result = %q", mcpResultText(created))
	}

	got := call(fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_note","arguments":{"note_id":%q}}}`, noteResp.NoteID))
	if !strings.Contains(mcpResultText(got), "hello from mcp") {
		t.Fatalf("get_note result = %q", mcpResultText(got))
	}

	// delete_note must be denied: notes:delete was not granted.
	del := call(fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"delete_note","arguments":{"note_id":%q}}}`, noteResp.NoteID))
	if !mcpToolIsError(del) || !strings.Contains(mcpResultText(del), "not authorized") {
		t.Fatalf("delete_note should be denied for missing scope: %v", del)
	}

	sd := call(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_docs","arguments":{"query":"knowledge"}}}`)
	if !strings.Contains(mcpResultText(sd), "README.md") {
		t.Fatalf("search_docs result = %q", mcpResultText(sd))
	}

	// --- refresh rotation ---
	refreshForm := url.Values{}
	refreshForm.Set("grant_type", "refresh_token")
	refreshForm.Set("refresh_token", tok.RefreshToken)
	refreshForm.Set("client_id", reg.ClientID)
	var tok2 struct {
		AccessToken string `json:"access_token"`
	}
	mcpPostForm(t, ts, "/v1/oauth/mcp/token", refreshForm, http.StatusOK, &tok2)
	if tok2.AccessToken == "" || tok2.AccessToken == tok.AccessToken {
		t.Fatalf("refresh should issue a new access token")
	}
	// old refresh token cannot be reused after rotation
	var reused map[string]any
	if status := mcpPostFormStatus(t, ts, "/v1/oauth/mcp/token", refreshForm, &reused); status != http.StatusBadRequest {
		t.Fatalf("rotated refresh token reuse should fail, got %d", status)
	}
}

// --- test helpers ---

func mcpGetJSON(t *testing.T, ts *httptest.Server, path string, out any) {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status = %d body = %s", path, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func mcpPostJSON(t *testing.T, ts *httptest.Server, path string, body any, wantStatus int, out any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s status = %d (want %d) body = %s", path, resp.StatusCode, wantStatus, b)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
}

func mcpPostForm(t *testing.T, ts *httptest.Server, path string, form url.Values, wantStatus int, out any) {
	t.Helper()
	if status := mcpPostFormStatus(t, ts, path, form, out); status != wantStatus {
		t.Fatalf("POST %s status = %d, want %d", path, status, wantStatus)
	}
}

func mcpPostFormStatus(t *testing.T, ts *httptest.Server, path string, form url.Values, out any) int {
	t.Helper()
	resp, err := http.PostForm(ts.URL+path, form)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func mcpResultText(resp map[string]any) string {
	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	first, _ := content[0].(map[string]any)
	s, _ := first["text"].(string)
	return s
}

func mcpToolIsError(resp map[string]any) bool {
	result, _ := resp["result"].(map[string]any)
	b, _ := result["isError"].(bool)
	return b
}
