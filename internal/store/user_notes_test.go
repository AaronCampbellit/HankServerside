package store

import (
	"context"
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
