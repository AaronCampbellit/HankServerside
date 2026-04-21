package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/testutil"
)

func TestValidateAgentTokenAndRevokeForHome(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", Email: "user@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	home := domain.Home{ID: "home_1", UserID: user.ID, Name: "Test Home", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateHome(ctx, home); err != nil {
		t.Fatalf("CreateHome: %v", err)
	}

	agent := domain.Agent{ID: "agent_1", HomeID: home.ID, Name: "Agent", Status: domain.AgentStatusOffline, CreatedAt: now, UpdatedAt: now}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	token := domain.AgentToken{ID: "agtok_1", HomeID: home.ID, AgentID: agent.ID, TokenHash: "token-hash", CreatedAt: now}
	if err := db.CreateAgentToken(ctx, token); err != nil {
		t.Fatalf("CreateAgentToken: %v", err)
	}

	record, err := db.ValidateAgentToken(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("ValidateAgentToken: %v", err)
	}
	if record.Agent.ID != agent.ID || record.Home.ID != home.ID {
		t.Fatalf("unexpected token record: %#v", record)
	}

	if err := db.RevokeAgentTokenForHome(ctx, home.ID, token.ID); err != nil {
		t.Fatalf("RevokeAgentTokenForHome: %v", err)
	}

	if _, err := db.ValidateAgentToken(ctx, token.TokenHash); err != ErrNotFound {
		t.Fatalf("ValidateAgentToken after revoke = %v, want ErrNotFound", err)
	}
}

func TestCreateHomeSeedsOwnerMembership(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	home := domain.Home{ID: "home_owner", UserID: user.ID, Name: "Owner Home", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateHome(ctx, home); err != nil {
		t.Fatalf("CreateHome: %v", err)
	}

	membership, err := db.GetHomeMembership(ctx, home.ID, user.ID)
	if err != nil {
		t.Fatalf("GetHomeMembership: %v", err)
	}
	if membership.Role != domain.HomeRoleAdmin {
		t.Fatalf("membership role = %q, want %q", membership.Role, domain.HomeRoleAdmin)
	}

	homes, err := db.ListHomesByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListHomesByUser: %v", err)
	}
	if len(homes) != 1 || homes[0].ID != home.ID {
		t.Fatalf("homes = %#v, want %q", homes, home.ID)
	}
}

func TestBootstrapSingletonHomeCreatesAdminAndPermissions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_admin", Email: "admin@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	home, created, err := db.BootstrapSingletonHome(ctx, user, "Home")
	if err != nil {
		t.Fatalf("BootstrapSingletonHome: %v", err)
	}
	if !created {
		t.Fatal("expected bootstrap to create the singleton home")
	}
	if home.Name != "Home" {
		t.Fatalf("home name = %q, want %q", home.Name, "Home")
	}

	membership, err := db.GetHomeMembership(ctx, home.ID, user.ID)
	if err != nil {
		t.Fatalf("GetHomeMembership: %v", err)
	}
	if membership.Role != domain.HomeRoleAdmin {
		t.Fatalf("membership role = %q, want %q", membership.Role, domain.HomeRoleAdmin)
	}

	permissions, err := db.GetHomePermissions(ctx, home.ID)
	if err != nil {
		t.Fatalf("GetHomePermissions: %v", err)
	}
	if !permissions.HomeAssistantEnabled || !permissions.FilesEnabled || !permissions.NotesEnabled {
		t.Fatalf("expected all default permissions enabled, got %#v", permissions)
	}
}

func TestOpenFailsWhenMultipleHomesExist(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	url := testutil.PostgreSQLTestURL(t)
	raw, err := sql.Open(driverName, url)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer raw.Close()

	now := time.Now().UTC()
	statements := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE homes (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE home_memberships (home_id TEXT NOT NULL, user_id TEXT NOT NULL, role TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, PRIMARY KEY(home_id, user_id))`,
		fmt.Sprintf(`INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES
			('usr_1', 'one@example.com', 'hash', '%s', '%s'),
			('usr_2', 'two@example.com', 'hash', '%s', '%s')`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)),
		fmt.Sprintf(`INSERT INTO homes (id, user_id, name, created_at, updated_at) VALUES
			('home_1', 'usr_1', 'One', '%s', '%s'),
			('home_2', 'usr_2', 'Two', '%s', '%s')`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)),
		fmt.Sprintf(`INSERT INTO home_memberships (home_id, user_id, role, created_at, updated_at) VALUES
			('home_1', 'usr_1', 'admin', '%s', '%s'),
			('home_2', 'usr_2', 'admin', '%s', '%s')`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)),
	}
	for _, statement := range statements {
		if _, err := raw.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed statement failed: %v", err)
		}
	}

	if _, err := Open(ctx, url); err == nil {
		t.Fatal("expected Open to fail when multiple homes exist")
	} else if !errors.Is(err, ErrUnsupportedMultiHome) {
		t.Fatalf("Open error = %v, want ErrUnsupportedMultiHome", err)
	}
}

func TestListAndDeletePendingHomeInvitations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_1", UserID: user.ID, Name: "Home", CreatedAt: now, UpdatedAt: now}
	invitation := domain.HomeInvitation{
		ID:        "invite_1",
		HomeID:    home.ID,
		Email:     "member@example.com",
		Role:      domain.HomeRoleMember,
		TokenHash: "hash",
		CreatedAt: now,
	}

	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateHome(ctx, home); err != nil {
		t.Fatalf("CreateHome: %v", err)
	}
	if err := db.CreateHomeInvitation(ctx, invitation); err != nil {
		t.Fatalf("CreateHomeInvitation: %v", err)
	}

	invitations, err := db.ListPendingHomeInvitations(ctx, home.ID)
	if err != nil {
		t.Fatalf("ListPendingHomeInvitations: %v", err)
	}
	if len(invitations) != 1 || invitations[0].ID != invitation.ID {
		t.Fatalf("invitations = %#v, want invitation %q", invitations, invitation.ID)
	}

	if err := db.DeletePendingHomeInvitation(ctx, home.ID, invitation.ID); err != nil {
		t.Fatalf("DeletePendingHomeInvitation: %v", err)
	}

	invitations, err = db.ListPendingHomeInvitations(ctx, home.ID)
	if err != nil {
		t.Fatalf("ListPendingHomeInvitations after delete: %v", err)
	}
	if len(invitations) != 0 {
		t.Fatalf("pending invitations = %#v, want empty", invitations)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	db, err := Open(context.Background(), testutil.PostgreSQLTestURL(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return db
}
