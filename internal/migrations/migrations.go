package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type Migration struct {
	Version    int64
	Name       string
	Statements []string
}

type Status struct {
	Version    int64     `json:"version"`
	Name       string    `json:"name"`
	Checksum   string    `json:"checksum"`
	AppliedAt  time.Time `json:"applied_at"`
	DurationMS int       `json:"duration_ms"`
}

var ErrChecksumMismatch = errors.New("schema migration checksum mismatch")

func Baseline() Migration {
	return Migration{
		Version: 1,
		Name:    "baseline",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS schema_migrations (
				version BIGINT PRIMARY KEY,
				name TEXT NOT NULL,
				checksum TEXT NOT NULL,
				applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				duration_ms INTEGER NOT NULL
			);`,
		},
	}
}

func Checksum(migration Migration) string {
	sum := sha256.New()
	_, _ = fmt.Fprintf(sum, "%d:%s\n", migration.Version, migration.Name)
	for _, statement := range migration.Statements {
		_, _ = sum.Write([]byte(statement))
		_, _ = sum.Write([]byte{'\n'})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func EnsureTable(ctx context.Context, db *sql.DB) error {
	for _, statement := range Baseline().Statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func BaselineExisting(ctx context.Context, db *sql.DB, duration time.Duration) error {
	if err := EnsureTable(ctx, db); err != nil {
		return err
	}
	migration := Baseline()
	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, checksum, applied_at, duration_ms)
		VALUES ($1, $2, $3, now(), $4)
		ON CONFLICT(version) DO NOTHING`,
		migration.Version,
		migration.Name,
		Checksum(migration),
		int(duration.Milliseconds()),
	); err != nil {
		return err
	}
	return Check(ctx, db)
}

func Applied(ctx context.Context, db *sql.DB) ([]Status, error) {
	if err := EnsureTable(ctx, db); err != nil {
		return nil, err
	}
	return AppliedReadOnly(ctx, db)
}

func AppliedReadOnly(ctx context.Context, db *sql.DB) ([]Status, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, name, checksum, applied_at, duration_ms FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var statuses []Status
	for rows.Next() {
		var status Status
		if err := rows.Scan(&status.Version, &status.Name, &status.Checksum, &status.AppliedAt, &status.DurationMS); err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}

func Check(ctx context.Context, db *sql.DB) error {
	statuses, err := Applied(ctx, db)
	if err != nil {
		return err
	}
	return CheckStatuses(statuses)
}

func CheckReadOnly(ctx context.Context, db *sql.DB) error {
	statuses, err := AppliedReadOnly(ctx, db)
	if err != nil {
		return err
	}
	return CheckStatuses(statuses)
}

func CheckStatuses(statuses []Status) error {
	if len(statuses) == 0 {
		return nil
	}
	want := Checksum(Baseline())
	for _, status := range statuses {
		if status.Version == 1 && status.Checksum != want {
			return fmt.Errorf("%w: version %d", ErrChecksumMismatch, status.Version)
		}
	}
	return nil
}
