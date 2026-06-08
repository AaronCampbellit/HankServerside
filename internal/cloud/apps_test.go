package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

func TestAppsListRequiresHomeMembership(t *testing.T) {
	ctx := context.Background()
	db, testServer, tokens := setupAppsHTTPOnly(t, ctx)

	now := time.Now().UTC()
	must(t, db.UpsertHomeApp(ctx, domain.HomeAgentApp{
		HomeID:              "home_apps",
		AppID:               "hermes",
		Name:                "Hermes",
		Version:             "1.0.0",
		PublicConfigJSON:    `{}`,
		SecretFieldsSetJSON: `{}`,
		Status:              "installed",
		UpdatedAt:           now,
		UpdatedBy:           "usr_apps_admin",
	}))

	member := requestJSONStatus(t, testServer, tokens.member, http.MethodGet, "/v1/home/apps", nil, http.StatusOK)
	defer member.Body.Close()
	var payload struct {
		Apps []domain.HomeAgentApp `json:"apps"`
	}
	if err := json.NewDecoder(member.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Apps) != 1 || payload.Apps[0].AppID != "hermes" {
		t.Fatalf("apps payload = %#v", payload)
	}

	outsider := requestJSONStatus(t, testServer, tokens.outsider, http.MethodGet, "/v1/home/apps", nil, http.StatusNotFound)
	outsider.Body.Close()
}

func TestAppsImportPreviewRequiresAdminAndOnlineAgent(t *testing.T) {
	ctx := context.Background()
	_, testServer, tokens := setupAppsHTTPOnly(t, ctx)

	member := doRawRequest(t, testServer, tokens.member, http.MethodPost, "/v1/home/apps/import/preview", []byte("package"))
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member preview status = %d body=%s", member.StatusCode, member.Body)
	}
	admin := doRawRequest(t, testServer, tokens.admin, http.MethodPost, "/v1/home/apps/import/preview", []byte("package"))
	if admin.StatusCode != http.StatusConflict {
		t.Fatalf("offline preview status = %d body=%s", admin.StatusCode, admin.Body)
	}
}

func TestAppsImportPreviewRoutesPackageToAgent(t *testing.T) {
	ctx := context.Background()
	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "test done")

	resultCh := make(chan testJSONResponse, 1)
	go func() {
		resultCh <- doRawRequest(t, testServer, sessionToken, http.MethodPost, "/v1/home/apps/import/preview", []byte("package-bytes"))
	}()

	envelope := readAgentCommandEnvelope(t, ctx, agentConn, protocol.CommandAppsPackagePreview)
	request := decodeRoutedCommandBody[protocol.AppsPackagePreviewRequest](t, envelope)
	if request.StagingID == "" || request.DownloadURL == "" || request.DownloadToken == "" {
		t.Fatalf("preview request = %#v", request)
	}
	packageRequest, err := http.NewRequest(http.MethodGet, request.DownloadURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	packageRequest.Header.Set("Authorization", "Bearer agent-token")
	packageRequest.Header.Set("X-Hank-Agent-ID", agentID)
	packageRequest.Header.Set("X-Hank-App-Package-Token", request.DownloadToken)
	packageResponse, err := http.DefaultClient.Do(packageRequest)
	if err != nil {
		t.Fatal(err)
	}
	packageData, _ := io.ReadAll(packageResponse.Body)
	packageResponse.Body.Close()
	if packageResponse.StatusCode != http.StatusOK || string(packageData) != "package-bytes" {
		t.Fatalf("package download status=%d body=%q", packageResponse.StatusCode, string(packageData))
	}

	writeAgentResponse(t, ctx, agentConn, envelope, agentID, homeID, protocol.AppsPackagePreviewResponse{
		StagingID: request.StagingID,
		App:       protocol.AppSummary{ID: "hermes", Name: "Hermes", Version: "1.0.0", Status: "preview"},
	})
	result := <-resultCh
	if result.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", result.StatusCode, result.Body)
	}
}

func TestAppsActivatePersistsReturnedAppMetadata(t *testing.T) {
	ctx := context.Background()
	db, testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgentWithDB(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "test done")

	resultCh := make(chan testJSONResponse, 1)
	go func() {
		resultCh <- doJSONRequest(t, testServer, sessionToken, http.MethodPost, "/v1/home/apps/import/activate", map[string]any{
			"staging_id": "stage_1",
			"enable":     true,
		})
	}()

	envelope := readAgentCommandEnvelope(t, ctx, agentConn, protocol.CommandAppsPackageActivate)
	request := decodeRoutedCommandBody[protocol.AppsPackageActivateRequest](t, envelope)
	if request.StagingID != "stage_1" || !request.Enable {
		t.Fatalf("activate request = %#v", request)
	}
	writeAgentResponse(t, ctx, agentConn, envelope, agentID, homeID, protocol.AppsPackageActivateResponse{
		App: protocol.AppSummary{
			ID:              "hermes",
			Name:            "Hermes",
			Version:         "1.0.0",
			Enabled:         true,
			Status:          "installed",
			PublicConfig:    json.RawMessage(`{"api_base_url":"https://hermes.local"}`),
			SecretFieldsSet: map[string]bool{"api_key": true},
		},
	})
	result := <-resultCh
	if result.StatusCode != http.StatusOK {
		t.Fatalf("activate status = %d body=%s", result.StatusCode, result.Body)
	}
	app, err := db.GetHomeApp(ctx, homeID, "hermes")
	if err != nil {
		t.Fatalf("GetHomeApp: %v", err)
	}
	if !app.Enabled || app.PublicConfigJSON != `{"api_base_url":"https://hermes.local"}` || app.SecretFieldsSetJSON != `{"api_key":true}` {
		t.Fatalf("persisted app = %#v", app)
	}
}

func TestAppsConfigApplyRoutesToAgent(t *testing.T) {
	ctx := context.Background()
	db, testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgentWithDB(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "test done")

	resultCh := make(chan testJSONResponse, 1)
	go func() {
		resultCh <- doJSONRequest(t, testServer, sessionToken, http.MethodPut, "/v1/home/apps/hermes/config", map[string]any{
			"public_config": map[string]any{"api_base_url": "https://hermes.local"},
			"secrets":       map[string]any{"api_key": "secret"},
			"enable":        true,
		})
	}()

	envelope := readAgentCommandEnvelope(t, ctx, agentConn, protocol.CommandAppsConfigApply)
	request := decodeRoutedCommandBody[protocol.AppsConfigApplyRequest](t, envelope)
	if request.AppID != "hermes" || string(request.PublicConfig) != `{"api_base_url":"https://hermes.local"}` || request.Enable == nil || !*request.Enable {
		t.Fatalf("config apply request = %#v public=%s", request, request.PublicConfig)
	}
	writeAgentResponse(t, ctx, agentConn, envelope, agentID, homeID, protocol.AppsConfigApplyResponse{
		App: protocol.AppSummary{
			ID:              "hermes",
			Name:            "Hermes",
			Version:         "1.0.0",
			Enabled:         true,
			Status:          "installed",
			PublicConfig:    json.RawMessage(`{"api_base_url":"https://hermes.local"}`),
			SecretFieldsSet: map[string]bool{"api_key": true},
		},
	})
	result := <-resultCh
	if result.StatusCode != http.StatusOK {
		t.Fatalf("config status = %d body=%s", result.StatusCode, result.Body)
	}
	app, err := db.GetHomeApp(ctx, homeID, "hermes")
	if err != nil {
		t.Fatalf("GetHomeApp: %v", err)
	}
	if app.SecretFieldsSetJSON != `{"api_key":true}` {
		t.Fatalf("persisted app = %#v", app)
	}
}

type appsHTTPOnlyTokens struct {
	admin    string
	member   string
	outsider string
}

func setupAppsHTTPOnly(t *testing.T, ctx context.Context) (*store.Store, *httptest.Server, appsHTTPOnlyTokens) {
	t.Helper()
	db := storeForTest(t)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	admin := domain.User{ID: "usr_apps_admin", Email: "apps-admin@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_apps_member", Email: "apps-member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	outsider := domain.User{ID: "usr_apps_outsider", Email: "apps-outsider@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_apps", UserID: admin.ID, Name: "Apps Home", CreatedAt: now, UpdatedAt: now}
	agent := domain.Agent{ID: "agent_apps", HomeID: home.ID, Name: "Apps Agent", Status: domain.AgentStatusOffline, CreatedAt: now, UpdatedAt: now}
	for _, user := range []domain.User{admin, member, outsider} {
		must(t, db.CreateUser(ctx, user))
	}
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{HomeID: home.ID, UserID: member.ID, Role: domain.HomeRoleMember, CreatedAt: now, UpdatedAt: now}))
	must(t, db.UpsertAgent(ctx, agent))

	tokens := appsHTTPOnlyTokens{admin: "apps-admin-token", member: "apps-member-token", outsider: "apps-outsider-token"}
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_apps_admin", UserID: admin.ID, TokenHash: hashToken(tokens.admin), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_apps_member", UserID: member.ID, TokenHash: hashToken(tokens.member), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_apps_outsider", UserID: outsider.ID, TokenHash: hashToken(tokens.outsider), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	return db, httptest.NewServer(server.http.Handler), tokens
}

func doRawRequest(t *testing.T, server *httptest.Server, sessionToken string, method string, path string, body []byte) testJSONResponse {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if sessionToken != "" {
		request.Header.Set("Authorization", "Bearer "+sessionToken)
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return testJSONResponse{StatusCode: response.StatusCode, Body: string(data)}
}

func readAgentCommandEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn, command string) protocol.Envelope {
	t.Helper()
	var envelope protocol.Envelope
	if err := wsjson.Read(ctx, conn, &envelope); err != nil {
		t.Fatalf("agent read command: %v", err)
	}
	if envelope.Type != protocol.TypeCloudCommand {
		t.Fatalf("envelope type = %q, want %q", envelope.Type, protocol.TypeCloudCommand)
	}
	var routed protocol.RoutedCommand
	if err := json.Unmarshal(envelope.Payload, &routed); err != nil {
		t.Fatalf("decode routed command: %v", err)
	}
	if routed.Command != command {
		t.Fatalf("command = %q, want %q", routed.Command, command)
	}
	return envelope
}

func decodeRoutedCommandBody[T any](t *testing.T, envelope protocol.Envelope) T {
	t.Helper()
	var routed protocol.RoutedCommand
	if err := json.Unmarshal(envelope.Payload, &routed); err != nil {
		t.Fatalf("decode routed command: %v", err)
	}
	var body T
	if err := json.Unmarshal(routed.Body, &body); err != nil {
		t.Fatalf("decode routed body: %v", err)
	}
	return body
}

func writeAgentResponse(t *testing.T, ctx context.Context, conn *websocket.Conn, request protocol.Envelope, agentID string, homeID string, payload any) {
	t.Helper()
	response, err := protocol.NewEnvelope(protocol.TypeCloudResponse, request.RequestID, agentID, homeID, payload)
	if err != nil {
		t.Fatalf("NewEnvelope response: %v", err)
	}
	if err := wsjson.Write(ctx, conn, response); err != nil {
		t.Fatalf("agent response write: %v", err)
	}
}
