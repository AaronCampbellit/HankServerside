package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/testutil"
)

func TestMigrateLegacyHomeNotesIntoUserNotesAndShares(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databaseURL := testutil.PostgreSQLTestURL(t)

	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member", Email: "member@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: owner.ID, Name: "Family Home", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, owner); err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := db.CreateUser(ctx, member); err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if err := db.CreateHome(ctx, home); err != nil {
		t.Fatalf("CreateHome: %v", err)
	}
	if err := db.AddHomeMembership(ctx, domain.HomeMembership{
		HomeID:    home.ID,
		UserID:    member.ID,
		Role:      domain.HomeRoleMember,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("AddHomeMembership: %v", err)
	}
	if err := db.UpsertHomeNote(ctx, domain.HomeNote{
		HomeID:    home.ID,
		NoteID:    "legacy.md",
		Title:     "Legacy",
		Content:   "migrated body",
		PageType:  "text",
		Revision:  "rev-1",
		Checksum:  "sum-1",
		UpdatedAt: now,
		UpdatedBy: owner.ID,
	}); err != nil {
		t.Fatalf("UpsertHomeNote: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer reopened.Close()

	note, err := reopened.GetOwnedHomeNote(ctx, home.ID, owner.ID, "legacy.md")
	if err != nil {
		t.Fatalf("GetOwnedHomeNote: %v", err)
	}
	if note.Content != "migrated body" {
		t.Fatalf("note content = %q, want %q", note.Content, "migrated body")
	}

	shares, err := reopened.ListNoteShares(ctx, note.ID)
	if err != nil {
		t.Fatalf("ListNoteShares: %v", err)
	}
	if len(shares) != 1 || shares[0].TargetUserID != member.ID {
		t.Fatalf("shares = %#v, want share for %q", shares, member.ID)
	}
}

func TestUserNotesStoreFullMetadataAndPostgresNotify(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := Open(ctx, testutil.PostgreSQLTestURL(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	notifications, err := db.Listen(ctx, NotificationChannelNotes)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	now := time.Now().UTC()
	user := domain.User{ID: "usr_notes_meta", Email: "notes-meta@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	note := domain.UserNote{
		ID:            "note_meta",
		NoteID:        "child.md",
		OwnerUserID:   user.ID,
		ParentID:      "parent.md",
		SortOrder:     7,
		Title:         "Child",
		Content:       "# Child",
		BodyMarkdown:  "# Child",
		BodyFormat:    "markdown",
		PageType:      "text",
		Revision:      "rev-meta",
		Checksum:      "sum-meta",
		CRDTStateJSON: "{}",
		CollabVersion: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
		UpdatedBy:     user.ID,
	}
	if err := db.SaveUserNoteWithOperations(ctx, note, []domain.NoteOperation{{
		NoteID:         note.ID,
		OpID:           "op-meta",
		ActorUserID:    user.ID,
		SessionID:      "test",
		BaseVersion:    0,
		AppliedVersion: 1,
		OpJSON:         `{"type":"text_replace","text":"# Child"}`,
		CreatedAt:      now,
	}}); err != nil {
		t.Fatalf("SaveUserNoteWithOperations: %v", err)
	}

	fetched, err := db.GetProfileNote(ctx, user.ID, "child.md")
	if err != nil {
		t.Fatalf("GetProfileNote: %v", err)
	}
	if fetched.ParentID != "parent.md" || fetched.SortOrder != 7 || fetched.BodyMarkdown != "# Child" || fetched.BodyFormat != "markdown" {
		t.Fatalf("metadata = parent:%q order:%d markdown:%q format:%q", fetched.ParentID, fetched.SortOrder, fetched.BodyMarkdown, fetched.BodyFormat)
	}

	select {
	case notification := <-notifications:
		var payload struct {
			Event         string `json:"event"`
			NoteID        string `json:"note_id"`
			OwnerUserID   string `json:"owner_user_id"`
			CollabVersion int64  `json:"collab_version"`
		}
		if err := json.Unmarshal(notification.Payload, &payload); err != nil {
			t.Fatalf("notification payload json: %v", err)
		}
		if payload.Event != "notes.changed" || payload.NoteID != "child.md" || payload.OwnerUserID != user.ID || payload.CollabVersion != 1 {
			t.Fatalf("notification payload = %+v", payload)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for note notification")
	}
}
