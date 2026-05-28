package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
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

func TestNotificationSettingsAndAPNSDevices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_notify", Email: "notify@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	session := domain.AppSession{ID: "sess_notify", UserID: user.ID, TokenHash: "hash", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	settings, err := db.SaveNotificationSettings(ctx, domain.NotificationSettings{
		UserID:                   user.ID,
		StorageEnabled:           false,
		NotesEnabled:             true,
		DashboardEntitiesEnabled: false,
	})
	if err != nil {
		t.Fatalf("SaveNotificationSettings: %v", err)
	}
	if settings.StorageEnabled || !settings.NotesEnabled || settings.DashboardEntitiesEnabled {
		t.Fatalf("settings = %#v", settings)
	}

	loaded, err := db.GetNotificationSettings(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetNotificationSettings: %v", err)
	}
	if loaded != settings {
		t.Fatalf("loaded settings = %#v, want %#v", loaded, settings)
	}

	device, err := db.UpsertAPNSDevice(ctx, domain.APNSDevice{
		UserID:            user.ID,
		SessionID:         session.ID,
		DeviceID:          "device-1",
		Token:             "token-1",
		Environment:       "sandbox",
		BundleID:          "com.dropfile.Hank",
		EnabledCategories: json.RawMessage(`["notes"]`),
	})
	if err != nil {
		t.Fatalf("UpsertAPNSDevice: %v", err)
	}
	if device.DeviceID != "device-1" || string(device.EnabledCategories) != `["notes"]` {
		t.Fatalf("device = %#v", device)
	}

	devices, err := db.ListActiveAPNSDevicesForUsers(ctx, []string{user.ID})
	if err != nil {
		t.Fatalf("ListActiveAPNSDevicesForUsers: %v", err)
	}
	if len(devices) != 1 || devices[0].DeviceID != "device-1" {
		t.Fatalf("devices = %#v", devices)
	}

	if err := db.DeleteAPNSDevicesForSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteAPNSDevicesForSession: %v", err)
	}
	devices, err = db.ListActiveAPNSDevicesForUsers(ctx, []string{user.ID})
	if err != nil {
		t.Fatalf("ListActiveAPNSDevicesForUsers after delete: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("devices after delete = %#v", devices)
	}
}

func TestSecretEncryptionProtectsStoredTokens(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()
	if err := db.ConfigureSecretEncryption("test-secret-encryption-key"); err != nil {
		t.Fatalf("ConfigureSecretEncryption: %v", err)
	}

	now := time.Now().UTC()
	user := domain.User{ID: "usr_secret_store", Email: "secret-store@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	session := domain.AppSession{ID: "sess_secret_store", UserID: user.ID, TokenHash: "session-hash", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := db.UpsertOpenAIAccount(ctx, domain.OpenAIAccount{
		UserID:       user.ID,
		AccessToken:  "openai-access-token",
		RefreshToken: "openai-refresh-token",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("UpsertOpenAIAccount: %v", err)
	}
	if _, err := db.UpsertAPNSDevice(ctx, domain.APNSDevice{
		UserID:            user.ID,
		SessionID:         session.ID,
		DeviceID:          "device-secret",
		Token:             "apns-device-token",
		Environment:       "sandbox",
		BundleID:          "com.dropfile.Hank",
		EnabledCategories: json.RawMessage(`[]`),
	}); err != nil {
		t.Fatalf("UpsertAPNSDevice: %v", err)
	}
	if _, err := db.SaveUserProfileSecretVault(ctx, user.ID, nil, "local", json.RawMessage(`{"password":"vault-secret"}`)); err != nil {
		t.Fatalf("SaveUserProfileSecretVault: %v", err)
	}

	var storedAccess, storedRefresh, storedAPNS, storedVault string
	if err := db.db.QueryRowContext(ctx, `SELECT access_token, refresh_token FROM openai_accounts WHERE user_id = ?`, user.ID).Scan(&storedAccess, &storedRefresh); err != nil {
		t.Fatalf("raw openai query: %v", err)
	}
	if err := db.db.QueryRowContext(ctx, `SELECT token FROM apns_devices WHERE user_id = ? AND device_id = ?`, user.ID, "device-secret").Scan(&storedAPNS); err != nil {
		t.Fatalf("raw apns query: %v", err)
	}
	if err := db.db.QueryRowContext(ctx, `SELECT vault_json::text FROM user_profile_secret_vaults WHERE user_id = ?`, user.ID).Scan(&storedVault); err != nil {
		t.Fatalf("raw vault query: %v", err)
	}
	for label, value := range map[string]string{
		"access":  storedAccess,
		"refresh": storedRefresh,
		"apns":    storedAPNS,
		"vault":   storedVault,
	} {
		if strings.Contains(value, "token") || strings.Contains(value, "vault-secret") {
			t.Fatalf("%s stored plaintext secret: %s", label, value)
		}
	}

	account, err := db.GetOpenAIAccount(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetOpenAIAccount: %v", err)
	}
	if account.AccessToken != "openai-access-token" || account.RefreshToken != "openai-refresh-token" {
		t.Fatalf("decrypted account = %#v", account)
	}
	vault, err := db.GetUserProfileSecretVault(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserProfileSecretVault: %v", err)
	}
	if string(vault.Vault) != `{"password":"vault-secret"}` {
		t.Fatalf("decrypted vault = %s", vault.Vault)
	}
}

func TestNotificationRecipientQueries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "usr_owner_notify", Email: "owner-notify@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	admin := domain.User{ID: "usr_admin_notify", Email: "admin-notify@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "usr_member_notify", Email: "member-notify@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	home := domain.Home{ID: "home_notify", UserID: owner.ID, Name: "Notify Home", CreatedAt: now, UpdatedAt: now}
	for _, user := range []domain.User{owner, admin, member} {
		if err := db.CreateUser(ctx, user); err != nil {
			t.Fatalf("CreateUser %s: %v", user.ID, err)
		}
	}
	if err := db.CreateHome(ctx, home); err != nil {
		t.Fatalf("CreateHome: %v", err)
	}
	if err := db.AddHomeMembership(ctx, domain.HomeMembership{HomeID: home.ID, UserID: admin.ID, Role: domain.HomeRoleAdmin, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("AddHomeMembership admin: %v", err)
	}
	if err := db.AddHomeMembership(ctx, domain.HomeMembership{HomeID: home.ID, UserID: member.ID, Role: domain.HomeRoleMember, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("AddHomeMembership member: %v", err)
	}

	storageRecipients, err := db.ListStorageNotificationUserIDs(ctx, home.ID)
	if err != nil {
		t.Fatalf("ListStorageNotificationUserIDs: %v", err)
	}
	assertSameStrings(t, storageRecipients, []string{owner.ID, admin.ID})

	note := domain.UserNote{
		ID:            "note_notify_internal",
		NoteID:        "note-notify.md",
		OwnerUserID:   owner.ID,
		HomeID:        home.ID,
		Title:         "Notify",
		Content:       "body",
		BodyMarkdown:  "body",
		BodyFormat:    "markdown",
		PageType:      "text",
		Revision:      "rev-1",
		Checksum:      "sum-1",
		CRDTStateJSON: "{}",
		CollabVersion: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
		UpdatedBy:     owner.ID,
	}
	if err := db.SaveUserNoteWithOperations(ctx, note, nil); err != nil {
		t.Fatalf("SaveUserNoteWithOperations: %v", err)
	}
	if err := db.AddNoteShare(ctx, domain.NoteShare{NoteID: note.ID, HomeID: home.ID, TargetUserID: member.ID, SharedBy: owner.ID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("AddNoteShare member: %v", err)
	}
	noteRecipients, err := db.ListNoteNotificationUserIDs(ctx, note.ID, owner.ID)
	if err != nil {
		t.Fatalf("ListNoteNotificationUserIDs: %v", err)
	}
	assertSameStrings(t, noteRecipients, []string{member.ID})

	if _, err := db.SaveUserProfileSettings(ctx, member.ID, nil, json.RawMessage(`{"dashboard_tiles":[{"entity_id":"light.kitchen","is_enabled":true}]}`)); err != nil {
		t.Fatalf("SaveUserProfileSettings member: %v", err)
	}
	if _, err := db.SaveUserProfileSettings(ctx, owner.ID, nil, json.RawMessage(`{"dashboard_tiles":[{"entity_id":"light.kitchen","is_enabled":false}]}`)); err != nil {
		t.Fatalf("SaveUserProfileSettings owner: %v", err)
	}
	dashboardRecipients, err := db.ListDashboardEntityNotificationUserIDs(ctx, home.ID, "light.kitchen")
	if err != nil {
		t.Fatalf("ListDashboardEntityNotificationUserIDs: %v", err)
	}
	assertSameStrings(t, dashboardRecipients, []string{member.ID})
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

func assertSameStrings(t *testing.T, got []string, want []string) {
	t.Helper()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
}
