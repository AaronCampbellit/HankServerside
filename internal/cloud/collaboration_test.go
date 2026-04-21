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
)

func TestProfileNotesRequireExplicitHomeShare(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	ownerSession := domain.AppSession{ID: "sess_owner", UserID: owner.ID, TokenHash: hashToken("owner-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	memberSession := domain.AppSession{ID: "sess_member", UserID: member.ID, TokenHash: hashToken("member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	home := domain.Home{ID: "home_1", UserID: owner.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}

	must(t, db.CreateUser(ctx, owner))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, ownerSession))
	must(t, db.CreateSession(ctx, memberSession))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var invite struct {
		Token string `json:"token"`
	}
	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/home/members/invitations", map[string]any{
		"email": member.Email,
		"role":  domain.HomeRoleMember,
	}, &invite)
	requestJSON(t, testServer, "member-token", http.MethodPost, "/v1/home/invitations/accept", map[string]any{
		"token": invite.Token,
	}, nil)

	var created protocol.NotesSaveResponse
	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/me/notes", map[string]any{
		"note_id": "shared.md",
		"title":   "Shared",
		"content": "family checklist #shared",
	}, &created)

	response := requestJSONStatus(t, testServer, "member-token", http.MethodGet, "/v1/home/notes/shared.md", nil, http.StatusNotFound)
	response.Body.Close()

	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/home/notes/shared.md/shares", map[string]any{
		"user_id": member.ID,
	}, nil)

	var fetched protocol.NotesFetchResponse
	requestJSON(t, testServer, "member-token", http.MethodGet, "/v1/home/notes/shared.md", nil, &fetched)
	if fetched.Content != "family checklist #shared" {
		t.Fatalf("note content = %q, want %q", fetched.Content, "family checklist #shared")
	}
}

func TestRemovingMemberRevokesSharedNoteAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	ownerSession := domain.AppSession{ID: "sess_owner", UserID: owner.ID, TokenHash: hashToken("owner-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	memberSession := domain.AppSession{ID: "sess_member", UserID: member.ID, TokenHash: hashToken("member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	home := domain.Home{ID: "home_1", UserID: owner.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}

	must(t, db.CreateUser(ctx, owner))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{
		HomeID:    home.ID,
		UserID:    member.ID,
		Role:      domain.HomeRoleMember,
		CreatedAt: now,
		UpdatedAt: now,
	}))
	must(t, db.CreateSession(ctx, ownerSession))
	must(t, db.CreateSession(ctx, memberSession))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/me/notes", map[string]any{
		"note_id": "shared.md",
		"title":   "Shared",
		"content": "family checklist",
	}, nil)
	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/home/notes/shared.md/shares", map[string]any{
		"user_id": member.ID,
	}, nil)

	requestJSON(t, testServer, "owner-token", http.MethodDelete, "/v1/home/members/"+member.ID, nil, nil)

	response := requestJSONStatus(t, testServer, "member-token", http.MethodGet, "/v1/home/notes/shared.md", nil, http.StatusNotFound)
	response.Body.Close()
}

func TestOwnerCanListAndRevokePendingInvitations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	ownerSession := domain.AppSession{ID: "sess_owner", UserID: owner.ID, TokenHash: hashToken("owner-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	home := domain.Home{ID: "home_1", UserID: owner.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}

	must(t, db.CreateUser(ctx, owner))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, ownerSession))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var created struct {
		InvitationID string `json:"invitation_id"`
	}
	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/home/members/invitations", map[string]any{
		"email": "member@example.com",
		"role":  domain.HomeRoleMember,
	}, &created)

	var list struct {
		Invitations []domain.HomeInvitation `json:"invitations"`
	}
	requestJSON(t, testServer, "owner-token", http.MethodGet, "/v1/home/members/invitations", nil, &list)
	if len(list.Invitations) != 1 || list.Invitations[0].ID != created.InvitationID {
		t.Fatalf("invitations = %#v, want %q", list.Invitations, created.InvitationID)
	}

	requestJSON(t, testServer, "owner-token", http.MethodDelete, "/v1/home/members/invitations/"+created.InvitationID, nil, nil)
	requestJSON(t, testServer, "owner-token", http.MethodGet, "/v1/home/members/invitations", nil, &list)
	if len(list.Invitations) != 0 {
		t.Fatalf("invitations after delete = %#v, want empty", list.Invitations)
	}
}

func TestNotesCollaborationBroadcastsOpsAndRevocation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	ownerSession := domain.AppSession{ID: "sess_owner", UserID: owner.ID, TokenHash: hashToken("owner-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	memberSession := domain.AppSession{ID: "sess_member", UserID: member.ID, TokenHash: hashToken("member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	home := domain.Home{ID: "home_1", UserID: owner.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}

	must(t, db.CreateUser(ctx, owner))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{
		HomeID:    home.ID,
		UserID:    member.ID,
		Role:      domain.HomeRoleMember,
		CreatedAt: now,
		UpdatedAt: now,
	}))
	must(t, db.CreateSession(ctx, ownerSession))
	must(t, db.CreateSession(ctx, memberSession))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/me/notes", map[string]any{
		"note_id": "shared.md",
		"title":   "Shared",
		"content": "hello",
	}, nil)
	requestJSON(t, testServer, "owner-token", http.MethodPost, "/v1/home/notes/shared.md/shares", map[string]any{
		"user_id": member.ID,
	}, nil)

	ownerConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/app?session_token=owner-token"), nil)
	if err != nil {
		t.Fatalf("owner websocket dial: %v", err)
	}
	defer ownerConn.Close(websocket.StatusNormalClosure, "done")

	memberConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/app?session_token=member-token"), nil)
	if err != nil {
		t.Fatalf("member websocket dial: %v", err)
	}
	defer memberConn.Close(websocket.StatusNormalClosure, "done")

	joinOwner, err := protocol.NewEnvelope(protocol.TypeAppCommand, "join_owner", "", home.ID, protocol.RoutedCommand{
		Command: "notes.collab.join",
		Body:    mustEncodeBody(t, protocol.NoteCollaborationJoinRequest{NoteID: "shared.md", SessionID: "owner-live"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	must(t, wsjson.Write(ctx, ownerConn, joinOwner))
	readUntilRequestID(t, ctx, ownerConn, "join_owner")

	joinMember, err := protocol.NewEnvelope(protocol.TypeAppCommand, "join_member", "", home.ID, protocol.RoutedCommand{
		Command: "notes.collab.join",
		Body:    mustEncodeBody(t, protocol.NoteCollaborationJoinRequest{NoteID: "shared.md", SessionID: "member-live"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	must(t, wsjson.Write(ctx, memberConn, joinMember))
	readUntilRequestID(t, ctx, memberConn, "join_member")
	readUntilEvent(t, ctx, ownerConn, "notes.collab.presence")

	submit, err := protocol.NewEnvelope(protocol.TypeAppCommand, "submit_ops", "", home.ID, protocol.RoutedCommand{
		Command: "notes.collab.submit_ops",
		Body: mustEncodeBody(t, protocol.NoteCollaborationSubmitOpsRequest{
			NoteID:      "shared.md",
			SessionID:   "owner-live",
			BaseVersion: 1,
			Ops: []protocol.NoteCollaborationOperation{{
				OpID:  "op-1",
				Type:  "text_insert",
				Index: 5,
				Text:  " world",
			}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	must(t, wsjson.Write(ctx, ownerConn, submit))
	readUntilRequestID(t, ctx, ownerConn, "submit_ops")

	event := readUntilEvent(t, ctx, memberConn, "notes.collab.ops")
	payload, err := protocol.DecodePayload[protocol.AppEvent](event)
	if err != nil {
		t.Fatal(err)
	}
	var ops protocol.NoteCollaborationOpsEvent
	if err := json.Unmarshal(payload.Body, &ops); err != nil {
		t.Fatal(err)
	}
	if len(ops.Ops) != 1 || ops.Ops[0].Text != " world" {
		t.Fatalf("unexpected ops payload: %#v", ops)
	}

	requestJSON(t, testServer, "owner-token", http.MethodDelete, "/v1/home/notes/shared.md/shares/"+member.ID, nil, nil)
	revoked := readUntilEvent(t, ctx, memberConn, "notes.collab.revoked")
	revokedPayload, err := protocol.DecodePayload[protocol.AppEvent](revoked)
	if err != nil {
		t.Fatal(err)
	}
	var revokedBody protocol.NoteCollaborationRevokedEvent
	if err := json.Unmarshal(revokedPayload.Body, &revokedBody); err != nil {
		t.Fatal(err)
	}
	if revokedBody.NoteID != "shared.md" {
		t.Fatalf("revoked note_id = %q, want %q", revokedBody.NoteID, "shared.md")
	}
}

func TestMemberCannotUpdateServiceProfile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: owner.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}
	ownerSession := domain.AppSession{ID: "sess_owner", UserID: owner.ID, TokenHash: hashToken("owner-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	memberSession := domain.AppSession{ID: "sess_member", UserID: member.ID, TokenHash: hashToken("member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}

	must(t, db.CreateUser(ctx, owner))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{
		HomeID:    home.ID,
		UserID:    member.ID,
		Role:      domain.HomeRoleMember,
		CreatedAt: now,
		UpdatedAt: now,
	}))
	must(t, db.CreateSession(ctx, ownerSession))
	must(t, db.CreateSession(ctx, memberSession))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	response := requestJSONStatus(t, testServer, "member-token", http.MethodPut, "/v1/home/service-profiles/smb", map[string]any{
		"public_config": map[string]any{"host": "nas.local", "share": "docs"},
	}, http.StatusForbidden)
	defer response.Body.Close()
}

func TestHomePermissionsDenyMemberNotesAccessWithoutOverride(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	admin := domain.User{ID: "usr_admin", Email: "admin@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: admin.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}
	adminSession := domain.AppSession{ID: "sess_admin", UserID: admin.ID, TokenHash: hashToken("admin-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	memberSession := domain.AppSession{ID: "sess_member", UserID: member.ID, TokenHash: hashToken("member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}

	must(t, db.CreateUser(ctx, admin))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{
		HomeID:    home.ID,
		UserID:    member.ID,
		Role:      domain.HomeRoleMember,
		CreatedAt: now,
		UpdatedAt: now,
	}))
	must(t, db.UpsertHomePermissions(ctx, domain.HomePermissions{
		HomeID:               home.ID,
		HomeAssistantEnabled: true,
		FilesEnabled:         true,
		NotesEnabled:         false,
		UpdatedAt:            now,
		UpdatedBy:            admin.ID,
	}))
	must(t, db.CreateSession(ctx, adminSession))
	must(t, db.CreateSession(ctx, memberSession))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	response := requestJSONStatus(t, testServer, "member-token", http.MethodGet, "/v1/home/notes", nil, http.StatusForbidden)
	defer response.Body.Close()
}

func TestHomeMemberPermissionsOverrideRestoresNotesAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	admin := domain.User{ID: "usr_admin", Email: "admin@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: admin.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}
	adminSession := domain.AppSession{ID: "sess_admin", UserID: admin.ID, TokenHash: hashToken("admin-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	memberSession := domain.AppSession{ID: "sess_member", UserID: member.ID, TokenHash: hashToken("member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	override := true

	must(t, db.CreateUser(ctx, admin))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{
		HomeID:    home.ID,
		UserID:    member.ID,
		Role:      domain.HomeRoleMember,
		CreatedAt: now,
		UpdatedAt: now,
	}))
	must(t, db.UpsertHomePermissions(ctx, domain.HomePermissions{
		HomeID:               home.ID,
		HomeAssistantEnabled: true,
		FilesEnabled:         true,
		NotesEnabled:         false,
		UpdatedAt:            now,
		UpdatedBy:            admin.ID,
	}))
	must(t, db.UpsertHomeMemberPermissions(ctx, domain.HomeMemberPermissions{
		HomeID:       home.ID,
		UserID:       member.ID,
		NotesEnabled: &override,
		UpdatedAt:    now,
		UpdatedBy:    admin.ID,
	}))
	must(t, db.CreateSession(ctx, adminSession))
	must(t, db.CreateSession(ctx, memberSession))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var payload struct {
		Notes []protocol.NoteSummary `json:"notes"`
	}
	requestJSON(t, testServer, "member-token", http.MethodGet, "/v1/home/notes", nil, &payload)
	if len(payload.Notes) != 0 {
		t.Fatalf("notes = %#v, want empty slice", payload.Notes)
	}
}

func TestNotesCommandsUseCloudStoreWhenAgentOffline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", Email: "user@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: user.ID, Name: "Offline Home", CreatedAt: now, UpdatedAt: now}
	session := domain.AppSession{ID: "sess_1", UserID: user.ID, TokenHash: hashToken("session-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}

	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateHome(ctx, home))
	must(t, db.CreateSession(ctx, session))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	appConn, _, err := websocket.Dial(ctx, wsURL(testServer.URL, "/ws/app?session_token=session-token"), nil)
	if err != nil {
		t.Fatalf("app websocket dial: %v", err)
	}
	defer appConn.Close(websocket.StatusNormalClosure, "done")

	save, err := protocol.NewEnvelope(protocol.TypeAppCommand, "req_notes_save", "", home.ID, protocol.RoutedCommand{
		Command: "notes.save",
		Body: mustEncodeBody(t, protocol.NotesSaveRequest{
			NoteID:  "shared.md",
			Title:   "Shared",
			Content: "offline-safe note",
		}),
	})
	if err != nil {
		t.Fatalf("NewEnvelope save: %v", err)
	}
	if err := wsjson.Write(ctx, appConn, save); err != nil {
		t.Fatalf("app save write: %v", err)
	}

	var saveResponse protocol.Envelope
	if err := wsjson.Read(ctx, appConn, &saveResponse); err != nil {
		t.Fatalf("app save response read: %v", err)
	}
	if saveResponse.Type != protocol.TypeAppResponse {
		t.Fatalf("save response type = %q, want %q", saveResponse.Type, protocol.TypeAppResponse)
	}

	fetch, err := protocol.NewEnvelope(protocol.TypeAppCommand, "req_notes_fetch", "", home.ID, protocol.RoutedCommand{
		Command: "notes.fetch",
		Body:    mustEncodeBody(t, protocol.NotesFetchRequest{NoteID: "shared.md"}),
	})
	if err != nil {
		t.Fatalf("NewEnvelope fetch: %v", err)
	}
	if err := wsjson.Write(ctx, appConn, fetch); err != nil {
		t.Fatalf("app fetch write: %v", err)
	}

	var fetchResponse protocol.Envelope
	if err := wsjson.Read(ctx, appConn, &fetchResponse); err != nil {
		t.Fatalf("app fetch response read: %v", err)
	}
	if fetchResponse.Type != protocol.TypeAppResponse {
		t.Fatalf("fetch response type = %q, want %q", fetchResponse.Type, protocol.TypeAppResponse)
	}

	fetched, err := protocol.DecodePayload[protocol.NotesFetchResponse](fetchResponse)
	if err != nil {
		t.Fatalf("DecodePayload fetch: %v", err)
	}
	if fetched.Content != "offline-safe note" {
		t.Fatalf("note content = %q, want %q", fetched.Content, "offline-safe note")
	}
}

func requestJSONStatus(t *testing.T, server *httptest.Server, sessionToken string, method string, path string, body any, wantStatus int) *http.Response {
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

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("request %s %s status = %d, want %d body=%s", method, path, response.StatusCode, wantStatus, string(data))
	}
	return response
}

func mustEncodeBody(t *testing.T, payload any) json.RawMessage {
	t.Helper()
	body, err := protocol.EncodeBody(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func readUntilRequestID(t *testing.T, ctx context.Context, conn *websocket.Conn, requestID string) protocol.Envelope {
	t.Helper()
	for {
		var envelope protocol.Envelope
		if err := wsjson.Read(ctx, conn, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.RequestID == requestID {
			return envelope
		}
	}
}

func readUntilEvent(t *testing.T, ctx context.Context, conn *websocket.Conn, wantEvent string) protocol.Envelope {
	t.Helper()
	for {
		var envelope protocol.Envelope
		if err := wsjson.Read(ctx, conn, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Type != protocol.TypeAppEvent {
			continue
		}
		event, err := protocol.DecodePayload[protocol.AppEvent](envelope)
		if err != nil {
			t.Fatal(err)
		}
		if event.Event == wantEvent {
			return envelope
		}
	}
}
