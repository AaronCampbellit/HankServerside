package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"golang.org/x/crypto/bcrypt"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
	"github.com/dropfile/hankremote/internal/testutil"
)

func TestAppCommandRoutesToAgentAndBack(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", Email: "user@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: user.ID, Name: "Test Home", CreatedAt: now, UpdatedAt: now}
	agent := domain.Agent{ID: "agent_1", HomeID: home.ID, Name: "Test Agent", Status: domain.AgentStatusOffline, CreatedAt: now, UpdatedAt: now}
	agentRawToken := "agent-token"
	sessionRawToken := "session-token"
	agentToken := domain.AgentToken{ID: "agtok_1", HomeID: home.ID, AgentID: agent.ID, TokenHash: hashToken(agentRawToken), CreatedAt: now}
	session := domain.AppSession{ID: "sess_1", UserID: user.ID, TokenHash: hashToken(sessionRawToken), ExpiresAt: now.Add(time.Hour), CreatedAt: now}

	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.UpsertAgent(ctx, agent))
	must(t, db.CreateAgentToken(ctx, agentToken))
	must(t, db.CreateSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	agentConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/agent?agent_id="+agent.ID+"&token="+agentRawToken), nil)
	if err != nil {
		t.Fatalf("agent websocket dial: %v", err)
	}
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	register, err := protocol.NewEnvelope(protocol.TypeAgentRegister, "", agent.ID, "", protocol.AgentRegister{AgentID: agent.ID, HomeName: home.Name})
	if err != nil {
		t.Fatalf("NewEnvelope register: %v", err)
	}
	if err := wsjson.Write(ctx, agentConn, register); err != nil {
		t.Fatalf("agent register write: %v", err)
	}

	var registered protocol.Envelope
	if err := wsjson.Read(ctx, agentConn, &registered); err != nil {
		t.Fatalf("agent read registered: %v", err)
	}
	if registered.Type != protocol.TypeAgentRegistered {
		t.Fatalf("registered type = %q, want %q", registered.Type, protocol.TypeAgentRegistered)
	}

	go func() {
		for {
			var envelope protocol.Envelope
			if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
				return
			}
			if envelope.Type != protocol.TypeCloudCommand {
				continue
			}

			command, err := protocol.DecodePayload[protocol.RoutedCommand](envelope)
			if err != nil {
				return
			}
			if command.Command != protocol.CommandSystemPing {
				return
			}

			response, err := protocol.NewEnvelope(protocol.TypeCloudResponse, envelope.RequestID, agent.ID, home.ID, protocol.SystemPingResponse{
				Message: "pong",
				Time:    time.Now().UTC(),
			})
			if err != nil {
				return
			}
			_ = wsjson.Write(ctx, agentConn, response)
		}
	}()

	appConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/app?session_token="+sessionRawToken), nil)
	if err != nil {
		t.Fatalf("app websocket dial: %v", err)
	}
	defer appConn.Close(websocket.StatusNormalClosure, "done")

	command, err := protocol.NewEnvelope(protocol.TypeAppCommand, "req_1", "", "", protocol.RoutedCommand{
		Command: protocol.CommandSystemPing,
	})
	if err != nil {
		t.Fatalf("NewEnvelope app command: %v", err)
	}
	if err := wsjson.Write(ctx, appConn, command); err != nil {
		t.Fatalf("app command write: %v", err)
	}

	var response protocol.Envelope
	if err := wsjson.Read(ctx, appConn, &response); err != nil {
		t.Fatalf("app response read: %v", err)
	}
	if response.Type != protocol.TypeAppResponse {
		t.Fatalf("app response type = %q, want %q", response.Type, protocol.TypeAppResponse)
	}

	payload, err := protocol.DecodePayload[protocol.SystemPingResponse](response)
	if err != nil {
		t.Fatalf("DecodePayload response: %v", err)
	}
	if payload.Message != "pong" {
		t.Fatalf("response message = %q, want %q", payload.Message, "pong")
	}
}

func TestAppWebSocketTicketCanBeIssuedAndConsumed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, _, _, sessionRawToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	go func() {
		for {
			var envelope protocol.Envelope
			if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
				return
			}
			if envelope.Type != protocol.TypeCloudCommand {
				continue
			}

			command, err := protocol.DecodePayload[protocol.RoutedCommand](envelope)
			if err != nil || command.Command != protocol.CommandSystemPing {
				return
			}

			response, err := protocol.NewEnvelope(protocol.TypeCloudResponse, envelope.RequestID, envelope.AgentID, envelope.HomeID, protocol.SystemPingResponse{
				Message: "pong",
				Time:    time.Now().UTC(),
			})
			if err != nil {
				return
			}
			_ = wsjson.Write(ctx, agentConn, response)
		}
	}()

	var ticketResponse struct {
		Ticket        string    `json:"ticket"`
		ExpiresAt     time.Time `json:"expires_at"`
		WebSocketPath string    `json:"websocket_path"`
	}
	requestJSON(t, testServer, sessionRawToken, http.MethodPost, "/v1/ws/app-ticket", nil, &ticketResponse)

	if ticketResponse.Ticket == "" {
		t.Fatal("expected app websocket ticket")
	}
	if ticketResponse.WebSocketPath == "" {
		t.Fatal("expected websocket path in ticket response")
	}

	appConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, ticketResponse.WebSocketPath), nil)
	if err != nil {
		t.Fatalf("ticket websocket dial: %v", err)
	}
	defer appConn.Close(websocket.StatusNormalClosure, "done")

	command, err := protocol.NewEnvelope(protocol.TypeAppCommand, "req_ticket_1", "", "", protocol.RoutedCommand{
		Command: protocol.CommandSystemPing,
	})
	if err != nil {
		t.Fatalf("NewEnvelope app command: %v", err)
	}
	if err := wsjson.Write(ctx, appConn, command); err != nil {
		t.Fatalf("ticket app command write: %v", err)
	}

	var response protocol.Envelope
	if err := wsjson.Read(ctx, appConn, &response); err != nil {
		t.Fatalf("ticket app response read: %v", err)
	}
	if response.Type != protocol.TypeAppResponse {
		t.Fatalf("app response type = %q, want %q", response.Type, protocol.TypeAppResponse)
	}

	payload, err := protocol.DecodePayload[protocol.SystemPingResponse](response)
	if err != nil {
		t.Fatalf("DecodePayload response: %v", err)
	}
	if payload.Message != "pong" {
		t.Fatalf("response message = %q, want %q", payload.Message, "pong")
	}
}

func TestRequestIDHeaderIsEchoed(t *testing.T) {
	t.Parallel()

	db := storeForTest(t)
	defer db.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	request, err := http.NewRequest(http.MethodGet, testServer.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("healthz request: %v", err)
	}
	request.Header.Set("X-Request-ID", "req_manual")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("healthz response: %v", err)
	}
	defer response.Body.Close()

	if got := response.Header.Get("X-Request-ID"); got != "req_manual" {
		t.Fatalf("X-Request-ID = %q, want %q", got, "req_manual")
	}
}

func TestLoginPageIsServed(t *testing.T) {
	t.Parallel()

	db := storeForTest(t)
	defer db.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	response, err := http.Get(testServer.URL + "/")
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("login content-type = %q", contentType)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("login body read: %v", err)
	}
	if !strings.Contains(string(body), "Hank Remote Login") {
		t.Fatalf("login body missing title: %s", string(body))
	}
}

func TestRegistrationDisabledAfterFirstAdminBootstrap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	firstBody := strings.NewReader(`{"email":"first@example.com","password":"change-me-123"}`)
	firstRequest, err := http.NewRequest(http.MethodPost, testServer.URL+"/v1/auth/register", firstBody)
	if err != nil {
		t.Fatalf("first register request: %v", err)
	}
	firstRequest.Header.Set("Content-Type", "application/json")

	firstResponse, err := http.DefaultClient.Do(firstRequest)
	if err != nil {
		t.Fatalf("first register response: %v", err)
	}
	defer firstResponse.Body.Close()
	if firstResponse.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(firstResponse.Body)
		t.Fatalf("first register status = %d, want %d body=%s", firstResponse.StatusCode, http.StatusCreated, string(data))
	}
	if _, err := db.GetSingletonHomeForUser(ctx, mustRegisteredUserID(t, firstResponse)); err != nil {
		t.Fatalf("first registered user should have singleton home membership: %v", err)
	}

	secondBody := strings.NewReader(`{"email":"second@example.com","password":"change-me-123"}`)
	secondRequest, err := http.NewRequest(http.MethodPost, testServer.URL+"/v1/auth/register", secondBody)
	if err != nil {
		t.Fatalf("second register request: %v", err)
	}
	secondRequest.Header.Set("Content-Type", "application/json")

	secondResponse, err := http.DefaultClient.Do(secondRequest)
	if err != nil {
		t.Fatalf("second register response: %v", err)
	}
	defer secondResponse.Body.Close()
	if secondResponse.StatusCode != http.StatusForbidden {
		data, _ := io.ReadAll(secondResponse.Body)
		t.Fatalf("second register status = %d, want %d body=%s", secondResponse.StatusCode, http.StatusForbidden, string(data))
	}
	for _, cookie := range secondResponse.Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			t.Fatalf("second registration unexpectedly set a session cookie")
		}
	}
	if _, err := db.GetUserByEmail(ctx, "second@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second user lookup error = %v, want ErrNotFound", err)
	}
}

func TestDashboardPagesRedirectWhenUnauthenticated(t *testing.T) {
	t.Parallel()

	db := storeForTest(t)
	defer db.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	paths := []string{
		"/dashboard",
		"/dashboard/home-users",
		"/dashboard/service-profiles",
		"/dashboard/sync-status",
		"/dashboard/storage",
		"/dashboard/hank",
		"/dashboard/assistant-settings",
		"/dashboard/profile-notes",
		"/dashboard/file-transfers",
		"/dashboard/accept-invitation",
	}

	for _, routePath := range paths {
		response, err := client.Get(testServer.URL + routePath)
		if err != nil {
			t.Fatalf("%s request: %v", routePath, err)
		}
		response.Body.Close()

		if response.StatusCode != http.StatusSeeOther {
			t.Fatalf("%s status = %d, want %d", routePath, response.StatusCode, http.StatusSeeOther)
		}
		if location := response.Header.Get("Location"); location != "/" {
			t.Fatalf("%s redirect location = %q, want %q", routePath, location, "/")
		}
	}
}

func TestDashboardPagesRequireHomeMembership(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	admin := domain.User{ID: "usr_admin", Email: "admin@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	outsider := domain.User{ID: "usr_outsider", Email: "outsider@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: admin.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}

	must(t, db.CreateUser(ctx, admin))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateUser(ctx, outsider))
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{
		HomeID:    home.ID,
		UserID:    member.ID,
		Role:      domain.HomeRoleMember,
		CreatedAt: now,
		UpdatedAt: now,
	}))
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_member", UserID: member.ID, TokenHash: hashToken("member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_outsider", UserID: outsider.ID, TokenHash: hashToken("outsider-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	normalPages := []string{
		"/dashboard",
		"/dashboard/home-users",
		"/dashboard/service-profiles",
		"/dashboard/sync-status",
		"/dashboard/hank",
		"/dashboard/assistant-settings",
		"/dashboard/profile-notes",
		"/dashboard/file-transfers",
	}

	for _, routePath := range normalPages {
		response := requestDashboardPage(t, testServer, routePath, "member-token")
		if response.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("member %s status = %d, want %d body=%s", routePath, response.StatusCode, http.StatusOK, string(data))
		}
		response.Body.Close()
	}

	for _, routePath := range normalPages {
		response := requestDashboardPage(t, testServer, routePath, "outsider-token")
		if response.StatusCode != http.StatusForbidden {
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("outsider %s status = %d, want %d body=%s", routePath, response.StatusCode, http.StatusForbidden, string(data))
		}
		response.Body.Close()
	}

	acceptResponse := requestDashboardPage(t, testServer, "/dashboard/accept-invitation", "outsider-token")
	if acceptResponse.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(acceptResponse.Body)
		acceptResponse.Body.Close()
		t.Fatalf("outsider accept-invitation status = %d, want %d body=%s", acceptResponse.StatusCode, http.StatusOK, string(data))
	}
	acceptResponse.Body.Close()

	storageResponse := requestDashboardPage(t, testServer, "/dashboard/storage", "member-token")
	if storageResponse.StatusCode != http.StatusForbidden {
		data, _ := io.ReadAll(storageResponse.Body)
		storageResponse.Body.Close()
		t.Fatalf("member storage status = %d, want %d body=%s", storageResponse.StatusCode, http.StatusForbidden, string(data))
	}
	storageResponse.Body.Close()
}

func TestDashboardStorageLinksAreAdminOnly(t *testing.T) {
	t.Parallel()

	pages := []string{
		"dashboard.html",
		"home-users.html",
		"service-profiles.html",
		"sync-status.html",
		"storage.html",
		"hank.html",
		"assistant-settings.html",
		"profile-notes.html",
		"file-transfers.html",
		"accept-invitation.html",
	}
	for _, page := range pages {
		data, err := fs.ReadFile(uiAssets, "ui/"+page)
		if err != nil {
			t.Fatalf("%s read: %v", page, err)
		}
		body := string(data)
		storageLinks := strings.Count(body, `href="/dashboard/storage"`)
		if storageLinks == 0 {
			t.Fatalf("%s has no storage dashboard link", page)
		}
		adminOnlyLinks := strings.Count(body, `href="/dashboard/storage" data-admin-only="true" hidden`)
		if adminOnlyLinks != storageLinks {
			t.Fatalf("%s storage admin-only links = %d, want %d", page, adminOnlyLinks, storageLinks)
		}
		adminScriptStart := strings.Index(body, `src="/assets/admin-nav.js`)
		if adminScriptStart == -1 {
			t.Fatalf("%s missing admin nav visibility script", page)
		}
		adminScriptEnd := strings.Index(body[adminScriptStart:], ">")
		if adminScriptEnd == -1 || !strings.Contains(body[adminScriptStart:adminScriptStart+adminScriptEnd], "defer") {
			t.Fatalf("%s admin nav visibility script is not deferred", page)
		}
	}

	if _, err := fs.ReadFile(uiAssets, "ui/admin-nav.js"); err != nil {
		t.Fatalf("admin-nav.js read: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/assets/admin-nav.js", nil)
	response := httptest.NewRecorder()
	serveUIAsset(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("admin-nav.js asset status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/javascript") {
		t.Fatalf("admin-nav.js content-type = %q", contentType)
	}
}

func TestOpenAIStatusReportsConfigAndLinkedAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_openai", Email: "openai@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	session := domain.AppSession{ID: "sess_openai", UserID: user.ID, TokenHash: hashToken("openai-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	server.ConfigureOpenAI("client-id", "client-secret", "https://remote.example.com/v1/oauth/openai/callback", "openid profile email")
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var status openAIAccountStatusResponse
	requestJSON(t, testServer, "openai-token", http.MethodGet, "/v1/oauth/openai/status", nil, &status)
	if !status.Configured {
		t.Fatalf("configured = false, want true; missing=%v", status.Missing)
	}
	if status.Linked {
		t.Fatal("linked = true before account exists")
	}

	expiresAt := now.Add(2 * time.Hour)
	must(t, db.UpsertOpenAIAccount(ctx, domain.OpenAIAccount{
		UserID:       user.ID,
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Scope:        "openid profile",
		ExpiresAt:    &expiresAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))

	status = openAIAccountStatusResponse{}
	requestJSON(t, testServer, "openai-token", http.MethodGet, "/v1/oauth/openai/status", nil, &status)
	if !status.Linked {
		t.Fatal("linked = false after account exists")
	}
	if status.Scope != "openid profile" {
		t.Fatalf("scope = %q, want %q", status.Scope, "openid profile")
	}
	if status.ExpiresAt == nil {
		t.Fatal("expires_at is nil")
	}
}

func TestLoginSetsStrictHTTPOnlySessionCookie(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("change-me-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("password hash: %v", err)
	}
	user := domain.User{ID: "usr_cookie", Email: "cookie@example.com", PasswordHash: string(passwordHash), CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, user))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	body := bytes.NewBufferString(`{"email":"cookie@example.com","password":"change-me-123"}`)
	request, err := http.NewRequest(http.MethodPost, testServer.URL+"/v1/auth/login", body)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("login response: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	cookies := response.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected %q cookie", sessionCookieName)
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("session cookie should be HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie SameSite = %v, want %v", sessionCookie.SameSite, http.SameSiteStrictMode)
	}
	if sessionCookie.Path != "/" {
		t.Fatalf("session cookie path = %q, want /", sessionCookie.Path)
	}
}

func TestAppWebSocketTicketIsSingleUse(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, _, _, sessionRawToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	var ticketResponse struct {
		Ticket        string `json:"ticket"`
		WebSocketPath string `json:"websocket_path"`
	}
	requestJSON(t, testServer, sessionRawToken, http.MethodPost, "/v1/ws/app-ticket", nil, &ticketResponse)

	firstConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, ticketResponse.WebSocketPath), nil)
	if err != nil {
		t.Fatalf("first ticket websocket dial: %v", err)
	}
	_ = firstConn.Close(websocket.StatusNormalClosure, "done")

	_, response, err := websocket.Dial(ctx, wsURL(testServer.URL, ticketResponse.WebSocketPath), nil)
	if err == nil {
		t.Fatal("expected second ticket websocket dial to fail")
	}
	if response == nil {
		t.Fatal("expected unauthorized websocket response")
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second ticket websocket status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestReadyzReturnsServiceUnavailableWhenStoreIsClosed(t *testing.T) {
	t.Parallel()

	db := storeForTest(t)
	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	response, err := http.Get(testServer.URL + "/readyz")
	if err != nil {
		t.Fatalf("readyz request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want %d", response.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestMetricsEndpointRendersCounters(t *testing.T) {
	t.Parallel()

	db := storeForTest(t)
	defer db.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	request, err := http.NewRequest(http.MethodGet, testServer.URL+"/v1/me", nil)
	if err != nil {
		t.Fatalf("me request: %v", err)
	}
	_, _ = http.DefaultClient.Do(request)

	response, err := http.Get(testServer.URL + "/metrics")
	if err != nil {
		t.Fatalf("metrics request: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("metrics body read: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "hank_remote_online_agents") {
		t.Fatalf("metrics missing online agent gauge: %s", text)
	}
	if !strings.Contains(text, `hank_remote_auth_failures_total{kind="app_http_unauthorized"} 1`) {
		t.Fatalf("metrics missing auth failure counter: %s", text)
	}
}

func TestRegistrationRateLimitIsEnforced(t *testing.T) {
	t.Parallel()

	db := storeForTest(t)
	defer db.Close()

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	for i := 0; i < 10; i++ {
		body := fmt.Sprintf(`{"email":"user-%d@example.com","password":"change-me-123"}`, i)
		request, err := http.NewRequest(http.MethodPost, testServer.URL+"/v1/auth/register", strings.NewReader(body))
		if err != nil {
			t.Fatalf("register request %d: %v", i, err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", "203.0.113.10")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("register response %d: %v", i, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("register status %d = %d, want %d", i, response.StatusCode, http.StatusCreated)
		}
	}

	request, err := http.NewRequest(http.MethodPost, testServer.URL+"/v1/auth/register", strings.NewReader(`{"email":"blocked@example.com","password":"change-me-123"}`))
	if err != nil {
		t.Fatalf("rate limit request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-For", "203.0.113.10")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("rate limit response: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit status = %d, want %d", response.StatusCode, http.StatusTooManyRequests)
	}
}

func TestListAgentTokensForHome(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", Email: "user@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: user.ID, Name: "Test Home", CreatedAt: now, UpdatedAt: now}
	agent := domain.Agent{ID: "agent_1", HomeID: home.ID, Name: "Test Agent", Status: domain.AgentStatusOffline, CreatedAt: now, UpdatedAt: now}
	sessionRawToken := "session-token"
	session := domain.AppSession{ID: "sess_1", UserID: user.ID, TokenHash: hashToken(sessionRawToken), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	token := domain.AgentToken{ID: "agtok_1", HomeID: home.ID, AgentID: agent.ID, TokenHash: hashToken("agent-token"), CreatedAt: now}

	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.UpsertAgent(ctx, agent))
	must(t, db.CreateSession(ctx, session))
	must(t, db.CreateAgentToken(ctx, token))

	server := NewServer("127.0.0.1:0", db, time.Hour, time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var payload struct {
		Tokens []domain.AgentToken `json:"tokens"`
	}
	requestJSON(t, testServer, sessionRawToken, http.MethodGet, "/v1/home/agent/tokens", nil, &payload)

	if len(payload.Tokens) != 1 {
		t.Fatalf("token count = %d, want 1", len(payload.Tokens))
	}
	if payload.Tokens[0].ID != token.ID || payload.Tokens[0].AgentID != agent.ID {
		t.Fatalf("listed token = %#v", payload.Tokens[0])
	}
	if payload.Tokens[0].TokenHash != "" {
		t.Fatalf("token hash should be omitted from JSON response")
	}
}

func TestRequestTimeoutReturnsAppError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", Email: "user@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: user.ID, Name: "Test Home", CreatedAt: now, UpdatedAt: now}
	agent := domain.Agent{ID: "agent_1", HomeID: home.ID, Name: "Test Agent", Status: domain.AgentStatusOffline, CreatedAt: now, UpdatedAt: now}
	agentRawToken := "agent-token"
	sessionRawToken := "session-token"
	agentToken := domain.AgentToken{ID: "agtok_1", HomeID: home.ID, AgentID: agent.ID, TokenHash: hashToken(agentRawToken), CreatedAt: now}
	session := domain.AppSession{ID: "sess_1", UserID: user.ID, TokenHash: hashToken(sessionRawToken), ExpiresAt: now.Add(time.Hour), CreatedAt: now}

	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.UpsertAgent(ctx, agent))
	must(t, db.CreateAgentToken(ctx, agentToken))
	must(t, db.CreateSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, 150*time.Millisecond, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	agentConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/agent?agent_id="+agent.ID+"&token="+agentRawToken), nil)
	if err != nil {
		t.Fatalf("agent websocket dial: %v", err)
	}
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	register, err := protocol.NewEnvelope(protocol.TypeAgentRegister, "", agent.ID, "", protocol.AgentRegister{AgentID: agent.ID, HomeName: home.Name})
	if err != nil {
		t.Fatalf("NewEnvelope register: %v", err)
	}
	if err := wsjson.Write(ctx, agentConn, register); err != nil {
		t.Fatalf("agent register write: %v", err)
	}

	var registered protocol.Envelope
	if err := wsjson.Read(ctx, agentConn, &registered); err != nil {
		t.Fatalf("agent read registered: %v", err)
	}

	appConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/app?session_token="+sessionRawToken), nil)
	if err != nil {
		t.Fatalf("app websocket dial: %v", err)
	}
	defer appConn.Close(websocket.StatusNormalClosure, "done")

	command, err := protocol.NewEnvelope(protocol.TypeAppCommand, "req_timeout", "", home.ID, protocol.RoutedCommand{
		Command: protocol.CommandSystemPing,
	})
	if err != nil {
		t.Fatalf("NewEnvelope app command: %v", err)
	}
	if err := wsjson.Write(ctx, appConn, command); err != nil {
		t.Fatalf("app command write: %v", err)
	}

	var response protocol.Envelope
	if err := wsjson.Read(ctx, appConn, &response); err != nil {
		t.Fatalf("app response read: %v", err)
	}
	if response.Type != protocol.TypeAppError {
		t.Fatalf("app response type = %q, want %q", response.Type, protocol.TypeAppError)
	}
	if response.Error == nil || response.Error.Code != "request_timeout" {
		t.Fatalf("app error = %#v, want request_timeout", response.Error)
	}
}

func TestFileDownloadTransferStreamsOverHTTP(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	go func() {
		for {
			var envelope protocol.Envelope
			if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
				return
			}

			if envelope.Type != protocol.TypeFileTransferOpen {
				continue
			}

			open, err := protocol.DecodePayload[protocol.FileTransferOpen](envelope)
			if err != nil || open.Operation != protocol.FileTransferOperationDownload {
				return
			}

			ready, _ := protocol.NewEnvelope(protocol.TypeFileTransferReady, envelope.RequestID, agentID, homeID, protocol.FileTransferReady{
				Operation: open.Operation,
				Path:      open.Path,
				Offset:    open.Offset,
				Size:      int64(len("hello world")),
			})
			_ = wsjson.Write(ctx, agentConn, ready)

			chunk1, _ := protocol.NewEnvelope(protocol.TypeFileTransferData, envelope.RequestID, agentID, homeID, protocol.FileTransferChunk{
				Offset:        open.Offset,
				ContentBase64: base64.StdEncoding.EncodeToString([]byte("hello ")),
			})
			_ = wsjson.Write(ctx, agentConn, chunk1)

			chunk2, _ := protocol.NewEnvelope(protocol.TypeFileTransferData, envelope.RequestID, agentID, homeID, protocol.FileTransferChunk{
				Offset:        open.Offset + int64(len("hello ")),
				ContentBase64: base64.StdEncoding.EncodeToString([]byte("world")),
			})
			_ = wsjson.Write(ctx, agentConn, chunk2)

			complete, _ := protocol.NewEnvelope(protocol.TypeFileTransferComplete, envelope.RequestID, agentID, homeID, protocol.FileTransferComplete{
				Operation: open.Operation,
				Path:      open.Path,
				Offset:    int64(len("hello world")),
				Size:      int64(len("hello world")),
			})
			_ = wsjson.Write(ctx, agentConn, complete)
		}
	}()

	setupResponse := struct {
		URL string `json:"url"`
	}{}
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/files/downloads", map[string]string{"path": "docs/report.txt"}, &setupResponse)

	response, err := http.Get(testServer.URL + setupResponse.URL)
	if err != nil {
		t.Fatalf("download GET: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, want 200", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("download body read: %v", err)
	}
	if string(body) != "hello world" {
		t.Fatalf("download body = %q, want %q", string(body), "hello world")
	}
}

func TestFileUploadTransferStreamsOverHTTP(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	var uploaded bytes.Buffer
	go func() {
		for {
			var envelope protocol.Envelope
			if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
				return
			}

			switch envelope.Type {
			case protocol.TypeFileTransferOpen:
				open, err := protocol.DecodePayload[protocol.FileTransferOpen](envelope)
				if err != nil || open.Operation != protocol.FileTransferOperationUpload {
					return
				}
				ready, _ := protocol.NewEnvelope(protocol.TypeFileTransferReady, envelope.RequestID, agentID, homeID, protocol.FileTransferReady{
					Operation: open.Operation,
					Path:      open.Path,
					Offset:    open.Offset,
					Size:      open.Offset,
				})
				_ = wsjson.Write(ctx, agentConn, ready)

			case protocol.TypeFileTransferData:
				chunk, err := protocol.DecodePayload[protocol.FileTransferChunk](envelope)
				if err != nil {
					return
				}
				data, err := base64.StdEncoding.DecodeString(chunk.ContentBase64)
				if err != nil {
					return
				}
				_, _ = uploaded.Write(data)

			case protocol.TypeFileTransferComplete:
				complete, err := protocol.DecodePayload[protocol.FileTransferComplete](envelope)
				if err != nil || complete.Operation != protocol.FileTransferOperationUpload {
					return
				}
				reply, _ := protocol.NewEnvelope(protocol.TypeFileTransferComplete, envelope.RequestID, agentID, homeID, protocol.FileTransferComplete{
					Operation: complete.Operation,
					Path:      complete.Path,
					Offset:    int64(uploaded.Len()),
					Size:      int64(uploaded.Len()),
				})
				_ = wsjson.Write(ctx, agentConn, reply)
				return
			}
		}
	}()

	setupResponse := struct {
		URL string `json:"url"`
	}{}
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/files/uploads", map[string]string{"path": "docs/upload.txt"}, &setupResponse)

	putRequest, err := http.NewRequest(http.MethodPut, testServer.URL+setupResponse.URL, strings.NewReader("stream me"))
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	putResponse, err := http.DefaultClient.Do(putRequest)
	if err != nil {
		t.Fatalf("upload PUT: %v", err)
	}
	defer putResponse.Body.Close()

	if putResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(putResponse.Body)
		t.Fatalf("upload status = %d, want 200 body=%s", putResponse.StatusCode, string(body))
	}
	if uploaded.String() != "stream me" {
		t.Fatalf("uploaded body = %q, want %q", uploaded.String(), "stream me")
	}
}

func TestFileDownloadTransferResumeFromOffset(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	content := "hello world"
	go func() {
		for {
			var envelope protocol.Envelope
			if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
				return
			}
			if envelope.Type != protocol.TypeFileTransferOpen {
				continue
			}

			open, err := protocol.DecodePayload[protocol.FileTransferOpen](envelope)
			if err != nil || open.Operation != protocol.FileTransferOperationDownload {
				return
			}

			ready, _ := protocol.NewEnvelope(protocol.TypeFileTransferReady, envelope.RequestID, agentID, homeID, protocol.FileTransferReady{
				Operation: open.Operation,
				Path:      open.Path,
				Offset:    open.Offset,
				Size:      int64(len(content)),
			})
			_ = wsjson.Write(ctx, agentConn, ready)

			remaining := content[open.Offset:]
			chunk, _ := protocol.NewEnvelope(protocol.TypeFileTransferData, envelope.RequestID, agentID, homeID, protocol.FileTransferChunk{
				Offset:        open.Offset,
				ContentBase64: base64.StdEncoding.EncodeToString([]byte(remaining)),
			})
			_ = wsjson.Write(ctx, agentConn, chunk)

			complete, _ := protocol.NewEnvelope(protocol.TypeFileTransferComplete, envelope.RequestID, agentID, homeID, protocol.FileTransferComplete{
				Operation: open.Operation,
				Path:      open.Path,
				Offset:    int64(len(content)),
				Size:      int64(len(content)),
			})
			_ = wsjson.Write(ctx, agentConn, complete)
		}
	}()

	setupResponse := struct {
		URL string `json:"url"`
	}{}
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/files/downloads", map[string]string{"path": "docs/report.txt"}, &setupResponse)

	firstResponse, err := http.Get(testServer.URL + setupResponse.URL + "&offset=0")
	if err != nil {
		t.Fatalf("first download GET: %v", err)
	}
	firstBody := make([]byte, 5)
	if _, err := io.ReadFull(firstResponse.Body, firstBody); err != nil {
		t.Fatalf("first partial read: %v", err)
	}
	_ = firstResponse.Body.Close()

	secondResponse, err := http.Get(testServer.URL + setupResponse.URL + "&offset=5")
	if err != nil {
		t.Fatalf("second download GET: %v", err)
	}
	defer secondResponse.Body.Close()

	remaining, err := io.ReadAll(secondResponse.Body)
	if err != nil {
		t.Fatalf("second download body read: %v", err)
	}
	if string(firstBody)+string(remaining) != content {
		t.Fatalf("resumed download = %q, want %q", string(firstBody)+string(remaining), content)
	}
}

func TestFileUploadTransferResumeFromOffset(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	var uploaded bytes.Buffer
	go func() {
		for {
			var envelope protocol.Envelope
			if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
				return
			}

			switch envelope.Type {
			case protocol.TypeFileTransferOpen:
				open, err := protocol.DecodePayload[protocol.FileTransferOpen](envelope)
				if err != nil || open.Operation != protocol.FileTransferOperationUpload {
					return
				}
				if int64(uploaded.Len()) > open.Offset {
					uploaded.Truncate(int(open.Offset))
				}
				ready, _ := protocol.NewEnvelope(protocol.TypeFileTransferReady, envelope.RequestID, agentID, homeID, protocol.FileTransferReady{
					Operation: open.Operation,
					Path:      open.Path,
					Offset:    int64(uploaded.Len()),
					Size:      int64(uploaded.Len()),
				})
				_ = wsjson.Write(ctx, agentConn, ready)

			case protocol.TypeFileTransferData:
				chunk, err := protocol.DecodePayload[protocol.FileTransferChunk](envelope)
				if err != nil {
					return
				}
				data, err := base64.StdEncoding.DecodeString(chunk.ContentBase64)
				if err != nil {
					return
				}
				if int64(uploaded.Len()) != chunk.Offset {
					return
				}
				_, _ = uploaded.Write(data)

			case protocol.TypeFileTransferComplete:
				complete, err := protocol.DecodePayload[protocol.FileTransferComplete](envelope)
				if err != nil || complete.Operation != protocol.FileTransferOperationUpload {
					return
				}
				reply, _ := protocol.NewEnvelope(protocol.TypeFileTransferComplete, envelope.RequestID, agentID, homeID, protocol.FileTransferComplete{
					Operation: complete.Operation,
					Path:      complete.Path,
					Offset:    int64(uploaded.Len()),
					Size:      int64(uploaded.Len()),
				})
				_ = wsjson.Write(ctx, agentConn, reply)
			}
		}
	}()

	setupResponse := struct {
		URL string `json:"url"`
	}{}
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/files/uploads", map[string]string{"path": "docs/upload.txt"}, &setupResponse)

	firstRequest, err := http.NewRequest(http.MethodPut, testServer.URL+setupResponse.URL+"&offset=0", strings.NewReader("stream "))
	if err != nil {
		t.Fatalf("first upload request: %v", err)
	}
	firstResponse, err := http.DefaultClient.Do(firstRequest)
	if err != nil {
		t.Fatalf("first upload PUT: %v", err)
	}
	_ = firstResponse.Body.Close()
	if firstResponse.StatusCode != http.StatusOK {
		t.Fatalf("first upload status = %d, want 200", firstResponse.StatusCode)
	}

	secondRequest, err := http.NewRequest(http.MethodPut, testServer.URL+setupResponse.URL+"&offset=7", strings.NewReader("me"))
	if err != nil {
		t.Fatalf("second upload request: %v", err)
	}
	secondResponse, err := http.DefaultClient.Do(secondRequest)
	if err != nil {
		t.Fatalf("second upload PUT: %v", err)
	}
	defer secondResponse.Body.Close()
	if secondResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(secondResponse.Body)
		t.Fatalf("second upload status = %d, want 200 body=%s", secondResponse.StatusCode, string(body))
	}

	if uploaded.String() != "stream me" {
		t.Fatalf("resumed upload body = %q, want %q", uploaded.String(), "stream me")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func wsURL(base string, path string) string {
	return "ws" + strings.TrimPrefix(base, "http") + path
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func storeForTest(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(context.Background(), testutil.PostgreSQLTestURL(t))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func setupServerAndAgent(t *testing.T, ctx context.Context) (*httptest.Server, string, string, string, *websocket.Conn) {
	t.Helper()

	db := storeForTest(t)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", Email: "user@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: user.ID, Name: "Test Home", CreatedAt: now, UpdatedAt: now}
	agent := domain.Agent{ID: "agent_1", HomeID: home.ID, Name: "Test Agent", Status: domain.AgentStatusOffline, CreatedAt: now, UpdatedAt: now}
	agentRawToken := "agent-token"
	sessionRawToken := "session-token"
	agentToken := domain.AgentToken{ID: "agtok_1", HomeID: home.ID, AgentID: agent.ID, TokenHash: hashToken(agentRawToken), CreatedAt: now}
	session := domain.AppSession{ID: "sess_1", UserID: user.ID, TokenHash: hashToken(sessionRawToken), ExpiresAt: now.Add(time.Hour), CreatedAt: now}

	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.UpsertAgent(ctx, agent))
	must(t, db.CreateAgentToken(ctx, agentToken))
	must(t, db.CreateSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)

	agentConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/agent?agent_id="+agent.ID+"&token="+agentRawToken), nil)
	if err != nil {
		t.Fatalf("agent websocket dial: %v", err)
	}

	register, err := protocol.NewEnvelope(protocol.TypeAgentRegister, "", agent.ID, "", protocol.AgentRegister{AgentID: agent.ID, HomeName: home.Name})
	if err != nil {
		t.Fatalf("NewEnvelope register: %v", err)
	}
	if err := wsjson.Write(ctx, agentConn, register); err != nil {
		t.Fatalf("agent register write: %v", err)
	}

	var registered protocol.Envelope
	if err := wsjson.Read(ctx, agentConn, &registered); err != nil {
		t.Fatalf("agent read registered: %v", err)
	}
	if registered.Type != protocol.TypeAgentRegistered {
		t.Fatalf("registered type = %q, want %q", registered.Type, protocol.TypeAgentRegistered)
	}

	return testServer, home.ID, agent.ID, sessionRawToken, agentConn
}

func requestJSON(t *testing.T, server *httptest.Server, sessionToken string, method string, path string, body any, out any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}

	request, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+sessionToken)
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("request %s %s status = %d body=%s", method, path, response.StatusCode, string(data))
	}

	if out != nil {
		if err := json.NewDecoder(response.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

func requestDashboardPage(t *testing.T, server *httptest.Server, path string, sessionToken string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sessionToken != "" {
		request.AddCookie(&http.Cookie{
			Name:  sessionCookieName,
			Value: sessionToken,
			Path:  "/",
		})
	}

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func mustRegisteredUserID(t *testing.T, response *http.Response) string {
	t.Helper()

	var payload struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		SessionID    string    `json:"session_id"`
		SessionToken string    `json:"session_token"`
		ExpiresAt    time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.User.ID == "" {
		t.Fatal("registered response missing user ID")
	}
	if payload.SessionID == "" || payload.SessionToken == "" || payload.ExpiresAt.IsZero() {
		t.Fatalf("registered response missing session fields: %#v", payload)
	}
	return payload.User.ID
}
