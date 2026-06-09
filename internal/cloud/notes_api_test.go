package cloud

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

func TestAppendNoteContentDefaultsToSingleNewline(t *testing.T) {
	appended, err := appendNoteContent("first line", protocol.NotesAppendRequest{Content: "second line"})
	if err != nil {
		t.Fatalf("appendNoteContent: %v", err)
	}
	if appended != "first line\nsecond line" {
		t.Fatalf("appended content = %q", appended)
	}
}

func TestAppendNoteContentHonorsBodyMarkdownAndExplicitSeparator(t *testing.T) {
	separator := "\n\n"
	appended, err := appendNoteContent("first paragraph", protocol.NotesAppendRequest{
		BodyMarkdown: "second paragraph",
		Separator:    &separator,
	})
	if err != nil {
		t.Fatalf("appendNoteContent: %v", err)
	}
	if appended != "first paragraph\n\nsecond paragraph" {
		t.Fatalf("appended content = %q", appended)
	}
}

func TestAppendNoteContentRequiresContent(t *testing.T) {
	_, err := appendNoteContent("first line", protocol.NotesAppendRequest{})
	if !errors.Is(err, errNoteAppendContentRequired) {
		t.Fatalf("appendNoteContent err = %v, want errNoteAppendContentRequired", err)
	}
}

func TestExternalProfileNotesAPIReadSearchTagsAndAppend(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_external_profile_notes", Email: "external-profile-notes@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	token := "external-profile-token"
	must(t, db.CreateUser(ctx, user))
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_external_profile_notes", UserID: user.ID, TokenHash: hashToken(token), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var save protocol.NotesSaveResponse
	requestJSON(t, testServer, token, http.MethodPost, "/v1/me/notes", map[string]any{
		"note_id":       "external.md",
		"title":         "External API",
		"body_markdown": "#todo buy milk\nremember the receipt",
	}, &save)

	var search protocol.NotesSearchResponse
	requestJSON(t, testServer, token, http.MethodGet, "/v1/me/notes/search?q=milk&limit=5", nil, &search)
	if len(search.Results) != 1 || search.Results[0].NoteID != "external.md" || !strings.Contains(search.Results[0].Preview, "milk") {
		t.Fatalf("search results = %#v, want external.md milk hit", search.Results)
	}

	var tags protocol.NotesTagsResponse
	requestJSON(t, testServer, token, http.MethodGet, "/v1/me/notes/tags", nil, &tags)
	if len(tags.Tags) != 1 || tags.Tags[0].Tag != "todo" || tags.Tags[0].Count != 1 {
		t.Fatalf("tags = %#v, want one todo tag", tags.Tags)
	}

	var rollup protocol.NotesTagRollupResponse
	requestJSON(t, testServer, token, http.MethodGet, "/v1/me/notes/tag-rollup?tag=todo", nil, &rollup)
	if len(rollup.Items) != 1 || rollup.Items[0].NoteID != "external.md" || !strings.Contains(rollup.Items[0].LineText, "buy milk") {
		t.Fatalf("tag rollup = %#v, want external.md todo line", rollup.Items)
	}

	var appendResponse protocol.NotesSaveResponse
	requestJSON(t, testServer, token, http.MethodPost, "/v1/me/notes/external.md/append", map[string]any{
		"body_markdown":     "synced from another app",
		"expected_revision": save.Revision,
	}, &appendResponse)
	if appendResponse.Revision == save.Revision {
		t.Fatalf("append revision = original revision %q, want new revision", appendResponse.Revision)
	}

	var fetched protocol.NotesFetchResponse
	requestJSON(t, testServer, token, http.MethodGet, "/v1/me/notes/external.md", nil, &fetched)
	if fetched.BodyMarkdown != "#todo buy milk\nremember the receipt\nsynced from another app" {
		t.Fatalf("fetched body = %q", fetched.BodyMarkdown)
	}

	response := requestJSONStatus(t, testServer, token, http.MethodPost, "/v1/me/notes/external.md/append", map[string]any{
		"content":           "stale write",
		"expected_revision": save.Revision,
	}, http.StatusConflict)
	response.Body.Close()
}

func TestExternalHomeNotesAPIUsesSharedVisibilityAndHomePermission(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := storeForTest(t)
	defer db.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_external_home_owner", Email: "external-home-owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_external_home_member", Email: "external-home-member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_external_notes", UserID: owner.ID, Name: "External Notes Home", CreatedAt: now, UpdatedAt: now}
	must(t, db.CreateUser(ctx, owner))
	must(t, db.CreateUser(ctx, member))
	must(t, db.CreateHome(ctx, home))
	must(t, db.AddHomeMembership(ctx, domain.HomeMembership{HomeID: home.ID, UserID: member.ID, Role: domain.HomeRoleMember, CreatedAt: now, UpdatedAt: now}))
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_external_home_owner", UserID: owner.ID, TokenHash: hashToken("external-owner-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))
	must(t, db.CreateSession(ctx, domain.AppSession{ID: "sess_external_home_member", UserID: member.ID, TokenHash: hashToken("external-member-token"), ExpiresAt: now.Add(time.Hour), CreatedAt: now}))

	server := NewServer("127.0.0.1:0", db, time.Hour, 5*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.http.Handler)
	defer testServer.Close()

	var save protocol.NotesSaveResponse
	requestJSON(t, testServer, "external-owner-token", http.MethodPost, "/v1/me/notes", map[string]any{
		"note_id": "shared-external.md",
		"title":   "Shared External",
		"content": "#handoff shared milk note",
	}, &save)
	requestJSON(t, testServer, "external-owner-token", http.MethodPost, "/v1/home/notes/shared-external.md/shares", map[string]any{
		"user_id": member.ID,
	}, nil)

	var search protocol.NotesSearchResponse
	requestJSON(t, testServer, "external-member-token", http.MethodGet, "/v1/home/notes/search?q=milk", nil, &search)
	if len(search.Results) != 1 || search.Results[0].NoteID != "shared-external.md" {
		t.Fatalf("home search results = %#v, want shared-external.md", search.Results)
	}

	requestJSON(t, testServer, "external-member-token", http.MethodPost, "/v1/home/notes/shared-external.md/append", map[string]any{
		"content":           "member update",
		"expected_revision": save.Revision,
	}, nil)

	var fetched protocol.NotesFetchResponse
	requestJSON(t, testServer, "external-owner-token", http.MethodGet, "/v1/home/notes/shared-external.md", nil, &fetched)
	if fetched.Content != "#handoff shared milk note\nmember update" {
		t.Fatalf("shared note content = %q", fetched.Content)
	}

	must(t, db.UpsertHomePermissions(ctx, domain.HomePermissions{
		HomeID:               home.ID,
		HomeAssistantEnabled: true,
		FilesEnabled:         true,
		NotesEnabled:         false,
		UpdatedAt:            now,
		UpdatedBy:            owner.ID,
	}))
	response := requestJSONStatus(t, testServer, "external-member-token", http.MethodGet, "/v1/home/notes/search?q=milk", nil, http.StatusForbidden)
	response.Body.Close()
}
