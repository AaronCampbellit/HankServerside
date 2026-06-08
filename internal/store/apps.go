package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/dropfile/hankremote/internal/domain"
)

const homeAgentAppColumns = `home_id, app_id, name, version, enabled, public_config_json, secret_fields_set_json, status, last_error, updated_at, updated_by`

func (s *Store) UpsertHomeApp(ctx context.Context, app domain.HomeAgentApp) error {
	_, err := s.exec(ctx, `INSERT INTO home_agent_apps (
			home_id, app_id, name, version, enabled, public_config_json, secret_fields_set_json, status, last_error, updated_at, updated_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(home_id, app_id) DO UPDATE SET
			name = excluded.name,
			version = excluded.version,
			enabled = excluded.enabled,
			public_config_json = excluded.public_config_json,
			secret_fields_set_json = excluded.secret_fields_set_json,
			status = excluded.status,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at,
			updated_by = excluded.updated_by`,
		app.HomeID,
		app.AppID,
		app.Name,
		app.Version,
		app.Enabled,
		app.PublicConfigJSON,
		app.SecretFieldsSetJSON,
		app.Status,
		app.LastError,
		app.UpdatedAt,
		app.UpdatedBy,
	)
	return err
}

func (s *Store) GetHomeApp(ctx context.Context, homeID string, appID string) (domain.HomeAgentApp, error) {
	row := s.queryRow(ctx, `SELECT `+homeAgentAppColumns+`
		FROM home_agent_apps
		WHERE home_id = ? AND app_id = ?`, homeID, appID)
	return scanHomeAgentApp(row)
}

func (s *Store) ListHomeApps(ctx context.Context, homeID string) ([]domain.HomeAgentApp, error) {
	rows, err := s.query(ctx, `SELECT `+homeAgentAppColumns+`
		FROM home_agent_apps
		WHERE home_id = ?
		ORDER BY name ASC, app_id ASC`, homeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []domain.HomeAgentApp
	for rows.Next() {
		app, err := scanHomeAgentApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func scanHomeAgentApp(scanner interface{ Scan(dest ...any) error }) (domain.HomeAgentApp, error) {
	var app domain.HomeAgentApp
	err := scanner.Scan(
		&app.HomeID,
		&app.AppID,
		&app.Name,
		&app.Version,
		&app.Enabled,
		&app.PublicConfigJSON,
		&app.SecretFieldsSetJSON,
		&app.Status,
		&app.LastError,
		&app.UpdatedAt,
		&app.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.HomeAgentApp{}, ErrNotFound
	}
	return app, err
}
