package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrUnsupportedMultiHome = errors.New("multiple homes are no longer supported in a single deployment")

type Store struct {
	db              *sql.DB
	databaseURL     string
	vectorAvailable bool
}

type AgentTokenRecord struct {
	Token domain.AgentToken
	Agent domain.Agent
	Home  domain.Home
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open(driverName, databaseURL)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db, databaseURL: databaseURL}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	store.vectorAvailable = store.enableVectorExtension(ctx)

	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) VectorAvailable() bool {
	return s != nil && s.vectorAvailable
}

func (s *Store) enableVectorExtension(ctx context.Context) bool {
	var available bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector')`).Scan(&available); err != nil || !available {
		return false
	}
	if _, err := s.db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return false
	}
	return true
}

func (s *Store) migrate(ctx context.Context) error {
	vectorColumn := ""
	if s.vectorAvailable {
		vectorColumn = ",\n\t\t\tembedding VECTOR(768) NULL"
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS homes (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS home_memberships (
			home_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY(home_id, user_id),
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`INSERT INTO home_memberships (home_id, user_id, role, created_at, updated_at)
			SELECT h.id, h.user_id, 'admin', h.created_at, h.updated_at
			FROM homes h
			WHERE NOT EXISTS (
				SELECT 1
				FROM home_memberships hm
				WHERE hm.home_id = h.id AND hm.user_id = h.user_id
			);`,
		`CREATE TABLE IF NOT EXISTS home_invitations (
			id TEXT PRIMARY KEY,
			home_id TEXT NOT NULL,
			email TEXT NOT NULL,
			role TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			accepted_at TIMESTAMP NULL,
			expires_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id)
		);`,
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			home_id TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			last_seen_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id)
		);`,
		`CREATE TABLE IF NOT EXISTS agent_tokens (
			id TEXT PRIMARY KEY,
			home_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			revoked_at TIMESTAMP NULL,
			expires_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(agent_id) REFERENCES agents(id)
		);`,
		`CREATE TABLE IF NOT EXISTS app_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMP NOT NULL,
			revoked_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS notification_settings (
			user_id TEXT PRIMARY KEY,
			storage_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			notes_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			dashboard_entities_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS apns_devices (
			user_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			token TEXT NOT NULL,
			environment TEXT NOT NULL,
			bundle_id TEXT NOT NULL,
			enabled_categories JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			last_registered_at TIMESTAMP NOT NULL,
			PRIMARY KEY(user_id, device_id),
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(session_id) REFERENCES app_sessions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS user_profile_backups (
			user_id TEXT PRIMARY KEY,
			revision INTEGER NOT NULL,
			snapshot_json TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS home_notes (
			home_id TEXT NOT NULL,
			note_id TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			page_type TEXT NOT NULL,
			board_json TEXT NOT NULL DEFAULT '',
			revision TEXT NOT NULL,
			checksum TEXT NOT NULL,
			deleted_at TIMESTAMP NULL,
			updated_at TIMESTAMP NOT NULL,
			updated_by TEXT NOT NULL,
			PRIMARY KEY(home_id, note_id),
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(updated_by) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS user_notes (
			id TEXT PRIMARY KEY,
			note_id TEXT NOT NULL,
			owner_user_id TEXT NOT NULL,
			home_id TEXT NULL,
			parent_id TEXT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			body_markdown TEXT NOT NULL DEFAULT '',
			body_format TEXT NOT NULL DEFAULT 'markdown',
			page_type TEXT NOT NULL,
			board_json TEXT NOT NULL DEFAULT '',
			revision TEXT NOT NULL,
			checksum TEXT NOT NULL,
			crdt_state_json TEXT NOT NULL DEFAULT '',
			collab_version INTEGER NOT NULL DEFAULT 0,
			deleted_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			updated_by TEXT NOT NULL,
			FOREIGN KEY(owner_user_id) REFERENCES users(id),
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(updated_by) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS note_shares (
			note_id TEXT NOT NULL,
			home_id TEXT NOT NULL,
			target_user_id TEXT NOT NULL,
			shared_by TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY(note_id, target_user_id),
			FOREIGN KEY(note_id) REFERENCES user_notes(id),
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(target_user_id) REFERENCES users(id),
			FOREIGN KEY(shared_by) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS note_operations (
			note_id TEXT NOT NULL,
			op_id TEXT NOT NULL,
			actor_user_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			base_version INTEGER NOT NULL,
			applied_version INTEGER NOT NULL,
			op_json TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			PRIMARY KEY(note_id, op_id),
			FOREIGN KEY(note_id) REFERENCES user_notes(id),
			FOREIGN KEY(actor_user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS note_attachments (
			id TEXT PRIMARY KEY,
			note_id TEXT NOT NULL,
			home_id TEXT NULL,
			owner_user_id TEXT NOT NULL,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size_bytes BIGINT NOT NULL DEFAULT 0,
			checksum_sha256 TEXT NOT NULL DEFAULT '',
			storage_key TEXT NOT NULL,
			deleted_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(note_id) REFERENCES user_notes(id) ON DELETE CASCADE,
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(owner_user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS user_profile_settings (
			user_id TEXT PRIMARY KEY,
			revision INTEGER NOT NULL DEFAULT 0,
			settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS user_profile_secret_vaults (
			user_id TEXT PRIMARY KEY,
			revision INTEGER NOT NULL DEFAULT 0,
			key_id TEXT NOT NULL DEFAULT '',
			vault_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS home_note_sync_state (
			home_id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			last_manifest_at TIMESTAMP NULL,
			last_pull_at TIMESTAMP NULL,
			last_push_at TIMESTAMP NULL,
			status TEXT NOT NULL,
			last_error TEXT NOT NULL DEFAULT '',
			pending_pull_count INTEGER NOT NULL DEFAULT 0,
			pending_push_count INTEGER NOT NULL DEFAULT 0,
			last_successful_sync_at TIMESTAMP NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id)
		);`,
		`CREATE TABLE IF NOT EXISTS home_service_profiles (
			home_id TEXT NOT NULL,
			service_type TEXT NOT NULL,
			public_config_json TEXT NOT NULL DEFAULT '',
			secret_version INTEGER NOT NULL DEFAULT 0,
			applied_version INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			updated_by TEXT NOT NULL,
			last_backup_at TIMESTAMP NULL,
			last_error TEXT NOT NULL DEFAULT '',
			PRIMARY KEY(home_id, service_type),
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(updated_by) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS home_permissions (
			home_id TEXT PRIMARY KEY,
			homeassistant_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			files_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			notes_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			updated_at TIMESTAMP NOT NULL,
			updated_by TEXT NOT NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(updated_by) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS home_member_permissions (
			home_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			homeassistant_enabled BOOLEAN NULL,
			files_enabled BOOLEAN NULL,
			notes_enabled BOOLEAN NULL,
			updated_at TIMESTAMP NOT NULL,
			updated_by TEXT NOT NULL,
			PRIMARY KEY(home_id, user_id),
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(updated_by) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_sessions (
			id TEXT PRIMARY KEY,
			home_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			last_message_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			status TEXT NOT NULL,
			content_json TEXT NOT NULL,
			model_name TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(session_id) REFERENCES assistant_sessions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_runs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			message_id TEXT NOT NULL,
			state TEXT NOT NULL,
			requires_client_tools BOOLEAN NOT NULL DEFAULT FALSE,
			requires_confirmation BOOLEAN NOT NULL DEFAULT FALSE,
			pending_action_json TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP NULL,
			FOREIGN KEY(session_id) REFERENCES assistant_sessions(id),
			FOREIGN KEY(message_id) REFERENCES assistant_messages(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_attachments (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			client_attachment_id TEXT NOT NULL,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			kind TEXT NOT NULL,
			size_bytes BIGINT NOT NULL DEFAULT 0,
			checksum_sha256 TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			committed_at TIMESTAMP NULL,
			UNIQUE(session_id, client_attachment_id),
			FOREIGN KEY(session_id) REFERENCES assistant_sessions(id) ON DELETE CASCADE,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_tool_calls (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			tool_scope TEXT NOT NULL,
			arguments_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			result_json JSONB NULL,
			status TEXT NOT NULL,
			started_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP NULL,
			FOREIGN KEY(run_id) REFERENCES assistant_runs(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_settings (
			home_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			profile_notes_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			home_notes_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			files_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			calendar_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			homeassistant_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			project_docs_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			conversations_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			system_prompt TEXT NOT NULL DEFAULT '',
			max_context_items INTEGER NOT NULL DEFAULT 8,
			chat_model TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			updated_by TEXT NOT NULL,
			PRIMARY KEY(home_id, user_id),
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(updated_by) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_documents (
			id TEXT PRIMARY KEY,
			home_id TEXT NOT NULL,
			user_id TEXT NULL,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_key TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL,
			path TEXT NOT NULL DEFAULT '',
			canonical_uri TEXT NOT NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			search_text TEXT NOT NULL,
			embedding_model TEXT NOT NULL DEFAULT '',
			embedding_version TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_chunks (
			id TEXT PRIMARY KEY,
			document_id TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			content TEXT NOT NULL,
			token_count INTEGER NOT NULL,
			embedding_json TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL DEFAULT '',
			embedding_version TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL` + vectorColumn + `,
			FOREIGN KEY(document_id) REFERENCES assistant_documents(id) ON DELETE CASCADE,
			UNIQUE(document_id, chunk_index)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_file_index (
			id TEXT PRIMARY KEY,
			home_id TEXT NOT NULL,
			service_profile_id TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL,
			name TEXT NOT NULL,
			is_directory BOOLEAN NOT NULL DEFAULT FALSE,
			size_bytes BIGINT NOT NULL DEFAULT 0,
			modified_at TIMESTAMP NULL,
			search_text TEXT NOT NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			embedding_json TEXT NOT NULL DEFAULT '',
			embedding_model TEXT NOT NULL DEFAULT '',
			embedding_version TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL` + vectorColumn + `,
			FOREIGN KEY(home_id) REFERENCES homes(id),
			UNIQUE(home_id, path)
		);`,
		`CREATE TABLE IF NOT EXISTS openai_accounts (
			user_id TEXT PRIMARY KEY,
			provider_user_id TEXT NOT NULL DEFAULT '',
			auth_provider TEXT NOT NULL DEFAULT '',
			chatgpt_plan_type TEXT NOT NULL DEFAULT '',
			access_token TEXT NOT NULL DEFAULT '',
			refresh_token TEXT NOT NULL DEFAULT '',
			token_type TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS openai_oauth_states (
			state_hash TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			code_verifier TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS assistant_calendar_entries (
			id TEXT PRIMARY KEY,
			home_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			external_event_id TEXT NOT NULL,
			calendar_id TEXT NOT NULL,
			title TEXT NOT NULL,
			location TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			starts_at TIMESTAMP NOT NULL,
			ends_at TIMESTAMP NOT NULL,
			is_all_day BOOLEAN NOT NULL DEFAULT FALSE,
			search_text TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(home_id) REFERENCES homes(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_homes_user_id ON homes(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_home_memberships_user_id ON home_memberships(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_home_invitations_email ON home_invitations(email);`,
		`CREATE INDEX IF NOT EXISTS idx_agents_home_id ON agents(home_id);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_tokens_agent_id ON agent_tokens(agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_app_sessions_user_id ON app_sessions(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_apns_devices_session ON apns_devices(session_id);`,
		`CREATE INDEX IF NOT EXISTS idx_apns_devices_token ON apns_devices(token);`,
		`CREATE INDEX IF NOT EXISTS idx_home_notes_updated_at ON home_notes(home_id, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_user_notes_owner_updated_at ON user_notes(owner_user_id, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_user_notes_home_updated_at ON user_notes(home_id, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_note_shares_home_user ON note_shares(home_id, target_user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_note_operations_note_version ON note_operations(note_id, applied_version);`,
		`CREATE INDEX IF NOT EXISTS idx_note_attachments_note ON note_attachments(note_id, created_at ASC);`,
		`ALTER TABLE user_notes ADD COLUMN IF NOT EXISTS parent_id TEXT NULL;`,
		`ALTER TABLE user_notes ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE user_notes ADD COLUMN IF NOT EXISTS body_markdown TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE user_notes ADD COLUMN IF NOT EXISTS body_format TEXT NOT NULL DEFAULT 'markdown';`,
		`UPDATE user_notes SET body_markdown = content WHERE body_markdown = '';`,
		`UPDATE user_notes SET body_format = 'markdown' WHERE body_format = '';`,
		`CREATE INDEX IF NOT EXISTS idx_user_notes_owner_parent_order ON user_notes(owner_user_id, parent_id, sort_order);`,
		`CREATE INDEX IF NOT EXISTS idx_user_notes_home_parent_order ON user_notes(home_id, parent_id, sort_order) WHERE home_id IS NOT NULL;`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_notes_owner_note_id ON user_notes(owner_user_id, note_id);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_notes_home_note_id ON user_notes(home_id, note_id) WHERE home_id IS NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_sessions_user_updated ON assistant_sessions(user_id, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_messages_session_created ON assistant_messages(session_id, created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_attachments_session ON assistant_attachments(session_id, created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_tool_calls_run ON assistant_tool_calls(run_id, started_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_documents_home_type ON assistant_documents(home_id, source_type, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_chunks_document ON assistant_chunks(document_id, chunk_index ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_assistant_file_index_home_updated ON assistant_file_index(home_id, updated_at DESC);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_assistant_calendar_external ON assistant_calendar_entries(user_id, device_id, external_event_id);`,
		`ALTER TABLE openai_accounts ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE openai_accounts ADD COLUMN IF NOT EXISTS chatgpt_plan_type TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE assistant_settings ADD COLUMN IF NOT EXISTS project_docs_enabled BOOLEAN NOT NULL DEFAULT TRUE;`,
		`ALTER TABLE assistant_settings ADD COLUMN IF NOT EXISTS conversations_enabled BOOLEAN NOT NULL DEFAULT TRUE;`,
		`ALTER TABLE assistant_settings ADD COLUMN IF NOT EXISTS chat_model TEXT NOT NULL DEFAULT '';`,
		`UPDATE home_memberships SET role = 'admin' WHERE role = 'owner';`,
	}

	if s.vectorAvailable {
		statements = append(statements,
			`ALTER TABLE assistant_chunks ADD COLUMN IF NOT EXISTS embedding VECTOR(768) NULL;`,
			`ALTER TABLE assistant_file_index ADD COLUMN IF NOT EXISTS embedding VECTOR(768) NULL;`,
		)
	}

	for _, statement := range statements {
		if _, err := s.exec(ctx, statement); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	if err := s.migrateLegacyHomeNotes(ctx); err != nil {
		return fmt.Errorf("migrate legacy home notes: %w", err)
	}

	if err := s.validateSingletonHome(ctx); err != nil {
		return err
	}

	if err := s.seedDefaultHomePermissions(ctx); err != nil {
		return fmt.Errorf("seed default home permissions: %w", err)
	}

	return nil
}

func (s *Store) CreateUser(ctx context.Context, user domain.User) error {
	_, err := s.exec(
		ctx,
		`INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	row := s.queryRow(ctx, `SELECT id, email, password_hash, created_at, updated_at FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	row := s.queryRow(ctx, `SELECT id, email, password_hash, created_at, updated_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) CreateHome(ctx context.Context, home domain.Home) error {
	tx, err := s.beginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO homes (id, user_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		home.ID,
		home.UserID,
		home.Name,
		home.CreatedAt,
		home.UpdatedAt,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO home_memberships (home_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		home.ID,
		home.UserID,
		domain.HomeRoleAdmin,
		home.CreatedAt,
		home.UpdatedAt,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetHomeByID(ctx context.Context, id string) (domain.Home, error) {
	row := s.queryRow(ctx, `SELECT id, user_id, name, created_at, updated_at FROM homes WHERE id = ?`, id)
	return scanHome(row)
}

func (s *Store) GetSingletonHome(ctx context.Context) (domain.Home, error) {
	row := s.queryRow(ctx, `SELECT id, user_id, name, created_at, updated_at FROM homes ORDER BY created_at ASC LIMIT 1`)
	return scanHome(row)
}

func (s *Store) GetSingletonHomeForUser(ctx context.Context, userID string) (domain.Home, error) {
	row := s.queryRow(ctx, `SELECT h.id, h.user_id, h.name, h.created_at, h.updated_at
		FROM homes h
		JOIN home_memberships hm ON hm.home_id = h.id
		WHERE hm.user_id = ?
		ORDER BY h.created_at ASC
		LIMIT 1`, userID)
	return scanHome(row)
}

func (s *Store) ListHomesByUser(ctx context.Context, userID string) ([]domain.Home, error) {
	home, err := s.GetSingletonHomeForUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []domain.Home{home}, nil
}

func (s *Store) GetHomeForUser(ctx context.Context, homeID string, userID string) (domain.Home, error) {
	home, err := s.GetSingletonHomeForUser(ctx, userID)
	if err != nil {
		return domain.Home{}, err
	}
	if home.ID != homeID {
		return domain.Home{}, ErrNotFound
	}
	return home, nil
}

func (s *Store) BootstrapSingletonHome(ctx context.Context, user domain.User, name string) (domain.Home, bool, error) {
	tx, err := s.beginTx(ctx, nil)
	if err != nil {
		return domain.Home{}, false, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, user_id, name, created_at, updated_at FROM homes ORDER BY created_at ASC LIMIT 2`)
	if err != nil {
		return domain.Home{}, false, err
	}
	defer rows.Close()

	var homes []domain.Home
	for rows.Next() {
		var home domain.Home
		if err := rows.Scan(&home.ID, &home.UserID, &home.Name, &home.CreatedAt, &home.UpdatedAt); err != nil {
			return domain.Home{}, false, err
		}
		homes = append(homes, home)
	}
	if err := rows.Err(); err != nil {
		return domain.Home{}, false, err
	}
	if len(homes) > 1 {
		return domain.Home{}, false, ErrUnsupportedMultiHome
	}
	if len(homes) == 1 {
		return homes[0], false, tx.Commit()
	}

	now := time.Now().UTC()
	home := domain.Home{
		ID:        newSingletonHomeID(),
		UserID:    user.ID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO homes (id, user_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		home.ID,
		home.UserID,
		home.Name,
		home.CreatedAt,
		home.UpdatedAt,
	); err != nil {
		return domain.Home{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO home_memberships (home_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		home.ID,
		user.ID,
		domain.HomeRoleAdmin,
		now,
		now,
	); err != nil {
		return domain.Home{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO home_permissions (home_id, homeassistant_enabled, files_enabled, notes_enabled, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		home.ID,
		true,
		true,
		true,
		now,
		user.ID,
	); err != nil {
		return domain.Home{}, false, err
	}
	return home, true, tx.Commit()
}

func (s *Store) RenameSingletonHome(ctx context.Context, homeID string, name string) (domain.Home, error) {
	now := time.Now().UTC()
	result, err := s.exec(ctx, `UPDATE homes SET name = ?, updated_at = ? WHERE id = ?`, name, now, homeID)
	if err != nil {
		return domain.Home{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.Home{}, err
	}
	if rows == 0 {
		return domain.Home{}, ErrNotFound
	}
	return s.GetHomeByID(ctx, homeID)
}

func (s *Store) UpsertAgent(ctx context.Context, agent domain.Agent) error {
	_, err := s.exec(
		ctx,
		`INSERT INTO agents (id, home_id, name, status, last_seen_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		 home_id = excluded.home_id,
		 name = excluded.name,
		 status = excluded.status,
		 last_seen_at = excluded.last_seen_at,
		 updated_at = excluded.updated_at`,
		agent.ID,
		agent.HomeID,
		agent.Name,
		agent.Status,
		agent.LastSeenAt,
		agent.CreatedAt,
		agent.UpdatedAt,
	)
	return err
}

func (s *Store) GetAgentByID(ctx context.Context, agentID string) (domain.Agent, error) {
	row := s.queryRow(ctx, `SELECT id, home_id, name, status, last_seen_at, created_at, updated_at FROM agents WHERE id = ?`, agentID)
	return scanAgent(row)
}

func (s *Store) GetAgentByHomeID(ctx context.Context, homeID string) (domain.Agent, error) {
	row := s.queryRow(ctx, `SELECT id, home_id, name, status, last_seen_at, created_at, updated_at FROM agents WHERE home_id = ? ORDER BY created_at ASC LIMIT 1`, homeID)
	return scanAgent(row)
}

func (s *Store) SetAgentStatus(ctx context.Context, agentID string, status string, lastSeenAt *time.Time) error {
	_, err := s.exec(
		ctx,
		`UPDATE agents SET status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		status,
		lastSeenAt,
		time.Now().UTC(),
		agentID,
	)
	return err
}

func (s *Store) CreateAgentToken(ctx context.Context, token domain.AgentToken) error {
	_, err := s.exec(
		ctx,
		`INSERT INTO agent_tokens (id, home_id, agent_id, token_hash, revoked_at, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token.ID,
		token.HomeID,
		token.AgentID,
		token.TokenHash,
		token.RevokedAt,
		token.ExpiresAt,
		token.CreatedAt,
	)
	return err
}

func (s *Store) RevokeAgentToken(ctx context.Context, tokenID string) error {
	_, err := s.exec(ctx, `UPDATE agent_tokens SET revoked_at = ? WHERE id = ?`, time.Now().UTC(), tokenID)
	return err
}

func (s *Store) RevokeAgentTokenForHome(ctx context.Context, homeID string, tokenID string) error {
	result, err := s.exec(ctx, `UPDATE agent_tokens SET revoked_at = ? WHERE id = ? AND home_id = ?`, time.Now().UTC(), tokenID, homeID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAgentTokenForHome(ctx context.Context, homeID string, tokenID string) error {
	result, err := s.exec(ctx, `DELETE FROM agent_tokens WHERE id = ? AND home_id = ?`, tokenID, homeID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListAgentTokensByHome(ctx context.Context, homeID string) ([]domain.AgentToken, error) {
	rows, err := s.query(
		ctx,
		`SELECT id, home_id, agent_id, token_hash, revoked_at, expires_at, created_at
		FROM agent_tokens
		WHERE home_id = ?
		ORDER BY created_at DESC`,
		homeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []domain.AgentToken
	for rows.Next() {
		token, err := scanAgentToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (s *Store) ValidateAgentToken(ctx context.Context, tokenHash string) (AgentTokenRecord, error) {
	row := s.queryRow(
		ctx,
		`SELECT
			at.id, at.home_id, at.agent_id, at.token_hash, at.revoked_at, at.expires_at, at.created_at,
			a.id, a.home_id, a.name, a.status, a.last_seen_at, a.created_at, a.updated_at,
			h.id, h.user_id, h.name, h.created_at, h.updated_at
		FROM agent_tokens at
		JOIN agents a ON a.id = at.agent_id
		JOIN homes h ON h.id = at.home_id
		WHERE at.token_hash = ?`,
		tokenHash,
	)

	record, err := scanAgentTokenRecord(row)
	if err != nil {
		return AgentTokenRecord{}, err
	}

	now := time.Now().UTC()
	if record.Token.RevokedAt != nil {
		return AgentTokenRecord{}, ErrNotFound
	}
	if record.Token.ExpiresAt != nil && record.Token.ExpiresAt.Before(now) {
		return AgentTokenRecord{}, ErrNotFound
	}

	return record, nil
}

func (s *Store) CreateSession(ctx context.Context, session domain.AppSession) error {
	_, err := s.exec(
		ctx,
		`INSERT INTO app_sessions (id, user_id, token_hash, expires_at, revoked_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.UserID,
		session.TokenHash,
		session.ExpiresAt,
		session.RevokedAt,
		session.CreatedAt,
	)
	return err
}

func (s *Store) GetSessionByHash(ctx context.Context, tokenHash string) (domain.AppSession, error) {
	row := s.queryRow(ctx, `SELECT id, user_id, token_hash, expires_at, revoked_at, created_at FROM app_sessions WHERE token_hash = ?`, tokenHash)
	session, err := scanSession(row)
	if err != nil {
		return domain.AppSession{}, err
	}

	now := time.Now().UTC()
	if session.RevokedAt != nil || session.ExpiresAt.Before(now) {
		return domain.AppSession{}, ErrNotFound
	}

	return session, nil
}

func (s *Store) GetSessionByID(ctx context.Context, sessionID string) (domain.AppSession, error) {
	row := s.queryRow(ctx, `SELECT id, user_id, token_hash, expires_at, revoked_at, created_at FROM app_sessions WHERE id = ?`, sessionID)
	session, err := scanSession(row)
	if err != nil {
		return domain.AppSession{}, err
	}

	now := time.Now().UTC()
	if session.RevokedAt != nil || session.ExpiresAt.Before(now) {
		return domain.AppSession{}, ErrNotFound
	}

	return session, nil
}

func (s *Store) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := s.exec(ctx, `UPDATE app_sessions SET revoked_at = ? WHERE id = ?`, time.Now().UTC(), sessionID)
	return err
}

func scanUser(scanner interface{ Scan(dest ...any) error }) (domain.User, error) {
	var user domain.User
	err := scanner.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	return user, err
}

func scanHome(scanner interface{ Scan(dest ...any) error }) (domain.Home, error) {
	var home domain.Home
	err := scanner.Scan(&home.ID, &home.UserID, &home.Name, &home.CreatedAt, &home.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Home{}, ErrNotFound
	}
	return home, err
}

func scanAgent(scanner interface{ Scan(dest ...any) error }) (domain.Agent, error) {
	var agent domain.Agent
	var lastSeen sql.NullTime
	err := scanner.Scan(&agent.ID, &agent.HomeID, &agent.Name, &agent.Status, &lastSeen, &agent.CreatedAt, &agent.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Agent{}, ErrNotFound
	}
	if err != nil {
		return domain.Agent{}, err
	}
	if lastSeen.Valid {
		agent.LastSeenAt = &lastSeen.Time
	}
	return agent, nil
}

func scanSession(scanner interface{ Scan(dest ...any) error }) (domain.AppSession, error) {
	var session domain.AppSession
	var revokedAt sql.NullTime
	err := scanner.Scan(&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt, &revokedAt, &session.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AppSession{}, ErrNotFound
	}
	if err != nil {
		return domain.AppSession{}, err
	}
	if revokedAt.Valid {
		session.RevokedAt = &revokedAt.Time
	}
	return session, nil
}

func scanAgentTokenRecord(scanner interface{ Scan(dest ...any) error }) (AgentTokenRecord, error) {
	var record AgentTokenRecord
	var tokenRevokedAt sql.NullTime
	var tokenExpiresAt sql.NullTime
	var agentLastSeenAt sql.NullTime

	err := scanner.Scan(
		&record.Token.ID,
		&record.Token.HomeID,
		&record.Token.AgentID,
		&record.Token.TokenHash,
		&tokenRevokedAt,
		&tokenExpiresAt,
		&record.Token.CreatedAt,
		&record.Agent.ID,
		&record.Agent.HomeID,
		&record.Agent.Name,
		&record.Agent.Status,
		&agentLastSeenAt,
		&record.Agent.CreatedAt,
		&record.Agent.UpdatedAt,
		&record.Home.ID,
		&record.Home.UserID,
		&record.Home.Name,
		&record.Home.CreatedAt,
		&record.Home.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentTokenRecord{}, ErrNotFound
	}
	if err != nil {
		return AgentTokenRecord{}, err
	}
	if tokenRevokedAt.Valid {
		record.Token.RevokedAt = &tokenRevokedAt.Time
	}
	if tokenExpiresAt.Valid {
		record.Token.ExpiresAt = &tokenExpiresAt.Time
	}
	if agentLastSeenAt.Valid {
		record.Agent.LastSeenAt = &agentLastSeenAt.Time
	}
	return record, nil
}

func scanAgentToken(scanner interface{ Scan(dest ...any) error }) (domain.AgentToken, error) {
	var token domain.AgentToken
	var revokedAt sql.NullTime
	var expiresAt sql.NullTime
	err := scanner.Scan(&token.ID, &token.HomeID, &token.AgentID, &token.TokenHash, &revokedAt, &expiresAt, &token.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AgentToken{}, ErrNotFound
	}
	if err != nil {
		return domain.AgentToken{}, err
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	return token, nil
}

func (s *Store) validateSingletonHome(ctx context.Context) error {
	rows, err := s.query(ctx, `SELECT id FROM homes ORDER BY created_at ASC LIMIT 2`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count > 1 {
		return fmt.Errorf("%w: found more than one home in homes table; consolidate to a single deployment home before starting this version", ErrUnsupportedMultiHome)
	}
	return nil
}

func (s *Store) seedDefaultHomePermissions(ctx context.Context) error {
	_, err := s.exec(ctx, `INSERT INTO home_permissions (home_id, homeassistant_enabled, files_enabled, notes_enabled, updated_at, updated_by)
		SELECT h.id, TRUE, TRUE, TRUE, h.updated_at, hm.user_id
		FROM homes h
		JOIN home_memberships hm ON hm.home_id = h.id
		WHERE hm.role = ?
			AND NOT EXISTS (
				SELECT 1 FROM home_permissions hp WHERE hp.home_id = h.id
			)
		ORDER BY h.created_at ASC
		LIMIT 1`, domain.HomeRoleAdmin)
	return err
}

func newSingletonHomeID() string {
	return "home_default"
}
