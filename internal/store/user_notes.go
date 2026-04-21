package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/dropfile/hankremote/internal/domain"
)

type migratedNoteState struct {
	Title         string `json:"title"`
	Content       string `json:"content"`
	PageType      string `json:"page_type"`
	BoardJSON     string `json:"board_json,omitempty"`
	CollabVersion int64  `json:"collab_version"`
}

func (s *Store) migrateLegacyHomeNotes(ctx context.Context) error {
	rows, err := s.query(ctx, `SELECT
			hn.home_id,
			hn.note_id,
			hn.title,
			hn.content,
			hn.page_type,
			hn.board_json,
			hn.revision,
			hn.checksum,
			hn.deleted_at,
			hn.updated_at,
			hn.updated_by,
			CASE
				WHEN EXISTS (
					SELECT 1
					FROM home_memberships hm
					WHERE hm.home_id = hn.home_id AND hm.user_id = hn.updated_by
				) THEN hn.updated_by
				ELSE h.user_id
			END AS owner_user_id
		FROM home_notes hn
		JOIN homes h ON h.id = hn.home_id
		ORDER BY hn.updated_at ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type legacyRow struct {
		domain.HomeNote
		OwnerUserID string
	}

	var legacyNotes []legacyRow
	for rows.Next() {
		var note legacyRow
		var deletedAt sql.NullTime
		if err := rows.Scan(
			&note.HomeID,
			&note.NoteID,
			&note.Title,
			&note.Content,
			&note.PageType,
			&note.BoardJSON,
			&note.Revision,
			&note.Checksum,
			&deletedAt,
			&note.UpdatedAt,
			&note.UpdatedBy,
			&note.OwnerUserID,
		); err != nil {
			return err
		}
		if deletedAt.Valid {
			note.DeletedAt = &deletedAt.Time
		}
		legacyNotes = append(legacyNotes, note)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if len(legacyNotes) == 0 {
		return nil
	}

	tx, err := s.beginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, legacy := range legacyNotes {
		internalID := legacyNoteInternalID(legacy.HomeID, legacy.NoteID)
		exists, err := txRecordExists(ctx, tx, `SELECT 1 FROM user_notes WHERE id = ?`, internalID)
		if err != nil {
			return err
		}
		if !exists {
			noteID, err := uniqueMigratedNoteID(ctx, tx, legacy.OwnerUserID, legacy.HomeID, legacy.NoteID)
			if err != nil {
				return err
			}
			stateJSON, err := json.Marshal(migratedNoteState{
				Title:         legacy.Title,
				Content:       legacy.Content,
				PageType:      legacy.PageType,
				BoardJSON:     legacy.BoardJSON,
				CollabVersion: 0,
			})
			if err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, `INSERT INTO user_notes (
					id, note_id, owner_user_id, home_id, title, content, page_type, board_json,
					revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				internalID,
				noteID,
				legacy.OwnerUserID,
				legacy.HomeID,
				legacy.Title,
				legacy.Content,
				legacy.PageType,
				legacy.BoardJSON,
				legacy.Revision,
				legacy.Checksum,
				string(stateJSON),
				int64(0),
				legacy.DeletedAt,
				legacy.UpdatedAt,
				legacy.UpdatedAt,
				legacy.UpdatedBy,
			); err != nil {
				return err
			}
		}

		memberRows, err := tx.QueryContext(ctx, `SELECT user_id FROM home_memberships WHERE home_id = ?`, legacy.HomeID)
		if err != nil {
			return err
		}
		var memberIDs []string
		for memberRows.Next() {
			var userID string
			if err := memberRows.Scan(&userID); err != nil {
				memberRows.Close()
				return err
			}
			memberIDs = append(memberIDs, userID)
		}
		if err := memberRows.Close(); err != nil {
			return err
		}
		for _, userID := range memberIDs {
			if userID == legacy.OwnerUserID {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO note_shares (
					note_id, home_id, target_user_id, shared_by, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT DO NOTHING`,
				internalID,
				legacy.HomeID,
				userID,
				legacy.OwnerUserID,
				legacy.UpdatedAt,
				legacy.UpdatedAt,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func legacyNoteInternalID(homeID string, noteID string) string {
	return "note_" + homeID + ":" + noteID
}

func uniqueMigratedNoteID(ctx context.Context, tx *dbTx, ownerUserID string, homeID string, base string) (string, error) {
	candidate := strings.TrimSpace(base)
	if candidate == "" {
		candidate = "note"
	}
	ext := path.Ext(candidate)
	name := strings.TrimSuffix(candidate, ext)
	if name == "" {
		name = "note"
	}
	for attempt := 0; attempt < 1000; attempt++ {
		exists, err := txRecordExists(ctx, tx, `SELECT 1
			FROM user_notes
			WHERE (owner_user_id = ? AND note_id = ?)
				OR (home_id = ? AND note_id = ?)
			LIMIT 1`, ownerUserID, candidate, homeID, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-migrated-%d%s", name, attempt+1, ext)
	}
	return "", fmt.Errorf("unable to generate migrated note_id for owner %s note %s", ownerUserID, base)
}

func txRecordExists(ctx context.Context, tx *dbTx, query string, args ...any) (bool, error) {
	row := tx.QueryRowContext(ctx, query, args...)
	var value int
	err := row.Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ListProfileNotes(ctx context.Context, ownerUserID string, includeDeleted bool) ([]domain.UserNote, error) {
	query := `SELECT id, note_id, owner_user_id, home_id, title, content, page_type, board_json, revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
		FROM user_notes
		WHERE owner_user_id = ?`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.query(ctx, query, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []domain.UserNote
	for rows.Next() {
		note, err := scanUserNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (s *Store) ListVisibleHomeNotes(ctx context.Context, homeID string, userID string, includeDeleted bool) ([]domain.UserNote, error) {
	query := `SELECT DISTINCT un.id, un.note_id, un.owner_user_id, un.home_id, un.title, un.content, un.page_type, un.board_json, un.revision, un.checksum, un.crdt_state_json, un.collab_version, un.deleted_at, un.created_at, un.updated_at, un.updated_by
		FROM user_notes un
		LEFT JOIN note_shares ns ON ns.note_id = un.id AND ns.target_user_id = ?
		WHERE un.home_id = ?
			AND (un.owner_user_id = ? OR ns.target_user_id = ?)`
	args := []any{userID, homeID, userID, userID}
	if !includeDeleted {
		query += ` AND un.deleted_at IS NULL`
	}
	query += ` ORDER BY un.updated_at DESC`
	rows, err := s.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []domain.UserNote
	for rows.Next() {
		note, err := scanUserNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (s *Store) ListSyncedHomeNotes(ctx context.Context, homeID string, includeDeleted bool) ([]domain.UserNote, error) {
	query := `SELECT DISTINCT un.id, un.note_id, un.owner_user_id, un.home_id, un.title, un.content, un.page_type, un.board_json, un.revision, un.checksum, un.crdt_state_json, un.collab_version, un.deleted_at, un.created_at, un.updated_at, un.updated_by
		FROM user_notes un
		WHERE un.home_id = ?
			AND EXISTS (SELECT 1 FROM note_shares ns WHERE ns.note_id = un.id)`
	if !includeDeleted {
		query += ` AND un.deleted_at IS NULL`
	}
	query += ` ORDER BY un.updated_at DESC`
	rows, err := s.query(ctx, query, homeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []domain.UserNote
	for rows.Next() {
		note, err := scanUserNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (s *Store) GetUserNoteByID(ctx context.Context, noteID string) (domain.UserNote, error) {
	row := s.queryRow(ctx, `SELECT id, note_id, owner_user_id, home_id, title, content, page_type, board_json, revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
		FROM user_notes
		WHERE id = ?`, noteID)
	return scanUserNote(row)
}

func (s *Store) GetProfileNote(ctx context.Context, ownerUserID string, noteKey string) (domain.UserNote, error) {
	row := s.queryRow(ctx, `SELECT id, note_id, owner_user_id, home_id, title, content, page_type, board_json, revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
		FROM user_notes
		WHERE owner_user_id = ? AND note_id = ?`, ownerUserID, noteKey)
	return scanUserNote(row)
}

func (s *Store) GetHomeNoteVisibleToUser(ctx context.Context, homeID string, userID string, noteKey string) (domain.UserNote, error) {
	row := s.queryRow(ctx, `SELECT DISTINCT un.id, un.note_id, un.owner_user_id, un.home_id, un.title, un.content, un.page_type, un.board_json, un.revision, un.checksum, un.crdt_state_json, un.collab_version, un.deleted_at, un.created_at, un.updated_at, un.updated_by
		FROM user_notes un
		LEFT JOIN note_shares ns ON ns.note_id = un.id AND ns.target_user_id = ?
		WHERE un.home_id = ? AND un.note_id = ?
			AND (un.owner_user_id = ? OR ns.target_user_id = ?)`,
		userID, homeID, noteKey, userID, userID)
	return scanUserNote(row)
}

func (s *Store) GetHomeNoteByKey(ctx context.Context, homeID string, noteKey string) (domain.UserNote, error) {
	row := s.queryRow(ctx, `SELECT id, note_id, owner_user_id, home_id, title, content, page_type, board_json, revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
		FROM user_notes
		WHERE home_id = ? AND note_id = ?`, homeID, noteKey)
	return scanUserNote(row)
}

func (s *Store) GetOwnedHomeNote(ctx context.Context, homeID string, ownerUserID string, noteKey string) (domain.UserNote, error) {
	row := s.queryRow(ctx, `SELECT id, note_id, owner_user_id, home_id, title, content, page_type, board_json, revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
		FROM user_notes
		WHERE home_id = ? AND owner_user_id = ? AND note_id = ?`, homeID, ownerUserID, noteKey)
	return scanUserNote(row)
}

func (s *Store) UpsertUserNote(ctx context.Context, note domain.UserNote) error {
	_, err := s.exec(ctx, `INSERT INTO user_notes (
			id, note_id, owner_user_id, home_id, title, content, page_type, board_json,
			revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			note_id = excluded.note_id,
			owner_user_id = excluded.owner_user_id,
			home_id = excluded.home_id,
			title = excluded.title,
			content = excluded.content,
			page_type = excluded.page_type,
			board_json = excluded.board_json,
			revision = excluded.revision,
			checksum = excluded.checksum,
			crdt_state_json = excluded.crdt_state_json,
			collab_version = excluded.collab_version,
			deleted_at = excluded.deleted_at,
			updated_at = excluded.updated_at,
			updated_by = excluded.updated_by`,
		note.ID,
		note.NoteID,
		note.OwnerUserID,
		nullableText(note.HomeID),
		note.Title,
		note.Content,
		note.PageType,
		note.BoardJSON,
		note.Revision,
		note.Checksum,
		note.CRDTStateJSON,
		note.CollabVersion,
		note.DeletedAt,
		note.CreatedAt,
		note.UpdatedAt,
		note.UpdatedBy,
	)
	return err
}

func (s *Store) SaveUserNoteWithOperations(ctx context.Context, note domain.UserNote, operations []domain.NoteOperation) error {
	tx, err := s.beginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO user_notes (
				id, note_id, owner_user_id, home_id, title, content, page_type, board_json,
				revision, checksum, crdt_state_json, collab_version, deleted_at, created_at, updated_at, updated_by
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				note_id = excluded.note_id,
				owner_user_id = excluded.owner_user_id,
				home_id = excluded.home_id,
				title = excluded.title,
				content = excluded.content,
				page_type = excluded.page_type,
				board_json = excluded.board_json,
				revision = excluded.revision,
				checksum = excluded.checksum,
				crdt_state_json = excluded.crdt_state_json,
				collab_version = excluded.collab_version,
				deleted_at = excluded.deleted_at,
				updated_at = excluded.updated_at,
				updated_by = excluded.updated_by`,
		note.ID,
		note.NoteID,
		note.OwnerUserID,
		nullableText(note.HomeID),
		note.Title,
		note.Content,
		note.PageType,
		note.BoardJSON,
		note.Revision,
		note.Checksum,
		note.CRDTStateJSON,
		note.CollabVersion,
		note.DeletedAt,
		note.CreatedAt,
		note.UpdatedAt,
		note.UpdatedBy,
	); err != nil {
		return err
	}

	for _, operation := range operations {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_operations (
				note_id, op_id, actor_user_id, session_id, base_version, applied_version, op_json, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`,
			operation.NoteID,
			operation.OpID,
			operation.ActorUserID,
			operation.SessionID,
			operation.BaseVersion,
			operation.AppliedVersion,
			operation.OpJSON,
			operation.CreatedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListNoteShares(ctx context.Context, noteID string) ([]domain.NoteShareMember, error) {
	rows, err := s.query(ctx, `SELECT ns.note_id, ns.home_id, ns.target_user_id, ns.shared_by, ns.created_at, ns.updated_at, u.email
		FROM note_shares ns
		JOIN users u ON u.id = ns.target_user_id
		WHERE ns.note_id = ?
		ORDER BY ns.created_at ASC`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []domain.NoteShareMember
	for rows.Next() {
		share, err := scanNoteShareMember(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	return shares, rows.Err()
}

func (s *Store) AddNoteShare(ctx context.Context, share domain.NoteShare) error {
	_, err := s.exec(ctx, `INSERT INTO note_shares (note_id, home_id, target_user_id, shared_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(note_id, target_user_id) DO UPDATE SET
			home_id = excluded.home_id,
			shared_by = excluded.shared_by,
			updated_at = excluded.updated_at`,
		share.NoteID,
		share.HomeID,
		share.TargetUserID,
		share.SharedBy,
		share.CreatedAt,
		share.UpdatedAt,
	)
	return err
}

func (s *Store) RemoveNoteShare(ctx context.Context, noteID string, targetUserID string) error {
	result, err := s.exec(ctx, `DELETE FROM note_shares WHERE note_id = ? AND target_user_id = ?`, noteID, targetUserID)
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

func (s *Store) RemoveNoteSharesForHomeUser(ctx context.Context, homeID string, userID string) error {
	_, err := s.exec(ctx, `DELETE FROM note_shares WHERE home_id = ? AND target_user_id = ?`, homeID, userID)
	return err
}

func (s *Store) CountNoteShares(ctx context.Context, noteID string) (int, error) {
	row := s.queryRow(ctx, `SELECT COUNT(*) FROM note_shares WHERE note_id = ?`, noteID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ListNoteOperationsSince(ctx context.Context, noteID string, appliedVersion int64, limit int) ([]domain.NoteOperation, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.query(ctx, `SELECT note_id, op_id, actor_user_id, session_id, base_version, applied_version, op_json, created_at
		FROM note_operations
		WHERE note_id = ? AND applied_version > ?
		ORDER BY applied_version ASC
		LIMIT ?`, noteID, appliedVersion, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var operations []domain.NoteOperation
	for rows.Next() {
		operation, err := scanNoteOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func (s *Store) GetOldestNoteOperationVersion(ctx context.Context, noteID string) (int64, error) {
	row := s.queryRow(ctx, `SELECT COALESCE(MIN(applied_version), 0) FROM note_operations WHERE note_id = ?`, noteID)
	var version int64
	if err := row.Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Store) HasNoteOperation(ctx context.Context, noteID string, opID string) (bool, error) {
	row := s.queryRow(ctx, `SELECT 1 FROM note_operations WHERE note_id = ? AND op_id = ?`, noteID, opID)
	var value int
	err := row.Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func scanUserNote(scanner interface{ Scan(dest ...any) error }) (domain.UserNote, error) {
	var note domain.UserNote
	var homeID sql.NullString
	var deletedAt sql.NullTime
	err := scanner.Scan(
		&note.ID,
		&note.NoteID,
		&note.OwnerUserID,
		&homeID,
		&note.Title,
		&note.Content,
		&note.PageType,
		&note.BoardJSON,
		&note.Revision,
		&note.Checksum,
		&note.CRDTStateJSON,
		&note.CollabVersion,
		&deletedAt,
		&note.CreatedAt,
		&note.UpdatedAt,
		&note.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.UserNote{}, ErrNotFound
	}
	if err != nil {
		return domain.UserNote{}, err
	}
	if homeID.Valid {
		note.HomeID = homeID.String
	}
	if deletedAt.Valid {
		note.DeletedAt = &deletedAt.Time
	}
	return note, nil
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func scanNoteShareMember(scanner interface{ Scan(dest ...any) error }) (domain.NoteShareMember, error) {
	var share domain.NoteShareMember
	err := scanner.Scan(
		&share.NoteID,
		&share.HomeID,
		&share.TargetUserID,
		&share.SharedBy,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.Email,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NoteShareMember{}, ErrNotFound
	}
	return share, err
}

func scanNoteOperation(scanner interface{ Scan(dest ...any) error }) (domain.NoteOperation, error) {
	var operation domain.NoteOperation
	err := scanner.Scan(
		&operation.NoteID,
		&operation.OpID,
		&operation.ActorUserID,
		&operation.SessionID,
		&operation.BaseVersion,
		&operation.AppliedVersion,
		&operation.OpJSON,
		&operation.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NoteOperation{}, ErrNotFound
	}
	return operation, err
}
