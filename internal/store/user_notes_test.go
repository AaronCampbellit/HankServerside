package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/testutil"
)

func TestUserNotesStoreFullMetadataAndPostgresNotify(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := OpenMigrating(ctx, testutil.PostgreSQLTestURL(t))
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

func TestSaveUserNoteRepairsLegacyPageTypeConstraint(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := OpenMigrating(ctx, testutil.PostgreSQLTestURL(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.exec(ctx, `ALTER TABLE user_notes DROP CONSTRAINT IF EXISTS user_notes_page_type_check`); err != nil {
		t.Fatalf("drop constraint: %v", err)
	}
	if _, err := db.exec(ctx, `ALTER TABLE user_notes ADD CONSTRAINT user_notes_page_type_check CHECK (page_type IN ('text', 'board'))`); err != nil {
		t.Fatalf("add legacy constraint: %v", err)
	}

	now := time.Now().UTC()
	user := domain.User{ID: "usr_notes_kanban_repair", Email: "notes-kanban-repair@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	note := domain.UserNote{
		ID:            "note_kanban_repair",
		NoteID:        "board.md",
		OwnerUserID:   user.ID,
		Title:         "Board",
		Content:       "# Board",
		BodyMarkdown:  "# Board",
		BodyFormat:    "markdown",
		PageType:      "kanban",
		BoardJSON:     `{"columns":[]}`,
		Revision:      "rev-kanban",
		Checksum:      "sum-kanban",
		CRDTStateJSON: "{}",
		CollabVersion: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
		UpdatedBy:     user.ID,
	}
	if err := db.SaveUserNoteWithOperations(ctx, note, []domain.NoteOperation{{
		NoteID:         note.ID,
		OpID:           "op-kanban",
		ActorUserID:    user.ID,
		SessionID:      "test",
		BaseVersion:    0,
		AppliedVersion: 1,
		OpJSON:         `{"type":"replace_snapshot","page_type":"kanban"}`,
		CreatedAt:      now,
	}}); err != nil {
		t.Fatalf("SaveUserNoteWithOperations repaired legacy constraint: %v", err)
	}

	fetched, err := db.GetProfileNote(ctx, user.ID, "board.md")
	if err != nil {
		t.Fatalf("GetProfileNote: %v", err)
	}
	if fetched.PageType != "kanban" {
		t.Fatalf("page_type = %q, want kanban", fetched.PageType)
	}
}
