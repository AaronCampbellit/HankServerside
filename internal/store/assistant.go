package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

func (s *Store) CreateAssistantSession(ctx context.Context, session domain.AssistantSession) error {
	_, err := s.exec(ctx, `INSERT INTO assistant_sessions (id, home_id, user_id, title, last_message_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.HomeID,
		session.UserID,
		session.Title,
		session.LastMessageAt,
		session.CreatedAt,
		session.UpdatedAt,
	)
	return err
}

func (s *Store) ListAssistantSessions(ctx context.Context, homeID string, userID string) ([]domain.AssistantSession, error) {
	rows, err := s.query(ctx, `SELECT id, home_id, user_id, title, last_message_at, created_at, updated_at
		FROM assistant_sessions
		WHERE home_id = ? AND user_id = ?
		ORDER BY updated_at DESC, created_at DESC`, homeID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []domain.AssistantSession
	for rows.Next() {
		session, err := scanAssistantSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Store) GetAssistantSession(ctx context.Context, sessionID string) (domain.AssistantSession, error) {
	row := s.queryRow(ctx, `SELECT id, home_id, user_id, title, last_message_at, created_at, updated_at
		FROM assistant_sessions WHERE id = ?`, sessionID)
	return scanAssistantSession(row)
}

func (s *Store) TouchAssistantSession(ctx context.Context, sessionID string, title string, updatedAt time.Time) error {
	_, err := s.exec(ctx, `UPDATE assistant_sessions
		SET title = ?, last_message_at = ?, updated_at = ?
		WHERE id = ?`, title, updatedAt, updatedAt, sessionID)
	return err
}

func (s *Store) CreateAssistantMessage(ctx context.Context, message domain.AssistantMessage) error {
	_, err := s.exec(ctx, `INSERT INTO assistant_messages (id, session_id, role, status, content_json, model_name, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		message.ID,
		message.SessionID,
		message.Role,
		message.Status,
		message.ContentJSON,
		message.ModelName,
		message.CreatedAt,
	)
	return err
}

func (s *Store) ListAssistantMessages(ctx context.Context, sessionID string) ([]domain.AssistantMessage, error) {
	rows, err := s.query(ctx, `SELECT id, session_id, role, status, content_json, model_name, created_at
		FROM assistant_messages
		WHERE session_id = ?
		ORDER BY created_at ASC, id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.AssistantMessage
	for rows.Next() {
		message, err := scanAssistantMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) CreateAssistantRun(ctx context.Context, run domain.AssistantRun) error {
	_, err := s.exec(ctx, `INSERT INTO assistant_runs (
			id, session_id, message_id, state, requires_client_tools, requires_confirmation,
			pending_action_json, created_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.SessionID,
		run.MessageID,
		run.State,
		run.RequiresClientTools,
		run.RequiresConfirmation,
		run.PendingActionJSON,
		run.CreatedAt,
		run.CompletedAt,
	)
	return err
}

func (s *Store) GetAssistantRun(ctx context.Context, runID string) (domain.AssistantRun, error) {
	row := s.queryRow(ctx, `SELECT id, session_id, message_id, state, requires_client_tools, requires_confirmation,
		pending_action_json, created_at, completed_at
		FROM assistant_runs WHERE id = ?`, runID)
	return scanAssistantRun(row)
}

func (s *Store) UpdateAssistantRun(ctx context.Context, run domain.AssistantRun) error {
	_, err := s.exec(ctx, `UPDATE assistant_runs
		SET state = ?, requires_client_tools = ?, requires_confirmation = ?, pending_action_json = ?, completed_at = ?
		WHERE id = ?`,
		run.State,
		run.RequiresClientTools,
		run.RequiresConfirmation,
		run.PendingActionJSON,
		run.CompletedAt,
		run.ID,
	)
	return err
}

func (s *Store) UpsertAssistantCalendarEntries(ctx context.Context, entries []domain.AssistantCalendarEntry) error {
	tx, err := s.beginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, entry := range entries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assistant_calendar_entries (
				id, home_id, user_id, device_id, external_event_id, calendar_id, title, location,
				notes, starts_at, ends_at, is_all_day, search_text, metadata_json, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id, device_id, external_event_id) DO UPDATE SET
				id = excluded.id,
				home_id = excluded.home_id,
				calendar_id = excluded.calendar_id,
				title = excluded.title,
				location = excluded.location,
				notes = excluded.notes,
				starts_at = excluded.starts_at,
				ends_at = excluded.ends_at,
				is_all_day = excluded.is_all_day,
				search_text = excluded.search_text,
				metadata_json = excluded.metadata_json,
				updated_at = excluded.updated_at`,
			entry.ID,
			entry.HomeID,
			entry.UserID,
			entry.DeviceID,
			entry.ExternalEventID,
			entry.CalendarID,
			entry.Title,
			entry.Location,
			entry.Notes,
			entry.StartsAt,
			entry.EndsAt,
			entry.IsAllDay,
			entry.SearchText,
			entry.MetadataJSON,
			entry.UpdatedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListAssistantCalendarEntries(ctx context.Context, homeID string, userID string) ([]domain.AssistantCalendarEntry, error) {
	rows, err := s.query(ctx, `SELECT id, home_id, user_id, device_id, external_event_id, calendar_id, title, location,
		notes, starts_at, ends_at, is_all_day, search_text, metadata_json, updated_at
		FROM assistant_calendar_entries
		WHERE home_id = ? AND user_id = ?
		ORDER BY starts_at ASC, title ASC`, homeID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []domain.AssistantCalendarEntry
	for rows.Next() {
		entry, err := scanAssistantCalendarEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanAssistantSession(scanner interface{ Scan(dest ...any) error }) (domain.AssistantSession, error) {
	var session domain.AssistantSession
	if err := scanner.Scan(
		&session.ID,
		&session.HomeID,
		&session.UserID,
		&session.Title,
		&session.LastMessageAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.AssistantSession{}, ErrNotFound
		}
		return domain.AssistantSession{}, err
	}
	return session, nil
}

func scanAssistantMessage(scanner interface{ Scan(dest ...any) error }) (domain.AssistantMessage, error) {
	var message domain.AssistantMessage
	if err := scanner.Scan(
		&message.ID,
		&message.SessionID,
		&message.Role,
		&message.Status,
		&message.ContentJSON,
		&message.ModelName,
		&message.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.AssistantMessage{}, ErrNotFound
		}
		return domain.AssistantMessage{}, err
	}
	return message, nil
}

func scanAssistantRun(scanner interface{ Scan(dest ...any) error }) (domain.AssistantRun, error) {
	var run domain.AssistantRun
	if err := scanner.Scan(
		&run.ID,
		&run.SessionID,
		&run.MessageID,
		&run.State,
		&run.RequiresClientTools,
		&run.RequiresConfirmation,
		&run.PendingActionJSON,
		&run.CreatedAt,
		&run.CompletedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.AssistantRun{}, ErrNotFound
		}
		return domain.AssistantRun{}, err
	}
	return run, nil
}

func scanAssistantCalendarEntry(scanner interface{ Scan(dest ...any) error }) (domain.AssistantCalendarEntry, error) {
	var entry domain.AssistantCalendarEntry
	if err := scanner.Scan(
		&entry.ID,
		&entry.HomeID,
		&entry.UserID,
		&entry.DeviceID,
		&entry.ExternalEventID,
		&entry.CalendarID,
		&entry.Title,
		&entry.Location,
		&entry.Notes,
		&entry.StartsAt,
		&entry.EndsAt,
		&entry.IsAllDay,
		&entry.SearchText,
		&entry.MetadataJSON,
		&entry.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.AssistantCalendarEntry{}, ErrNotFound
		}
		return domain.AssistantCalendarEntry{}, err
	}
	return entry, nil
}
