package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const driverName = "pgx"

type dbTx struct {
	tx *sql.Tx
}

func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, rebindPlaceholders(query), args...)
}

func (s *Store) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, rebindPlaceholders(query), args...)
}

func (s *Store) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, rebindPlaceholders(query), args...)
}

func (s *Store) beginTx(ctx context.Context, opts *sql.TxOptions) (*dbTx, error) {
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &dbTx{tx: tx}, nil
}

func (tx *dbTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.tx.ExecContext(ctx, rebindPlaceholders(query), args...)
}

func (tx *dbTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.tx.QueryContext(ctx, rebindPlaceholders(query), args...)
}

func (tx *dbTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.tx.QueryRowContext(ctx, rebindPlaceholders(query), args...)
}

func (tx *dbTx) Commit() error {
	return tx.tx.Commit()
}

func (tx *dbTx) Rollback() error {
	return tx.tx.Rollback()
}

func rebindPlaceholders(query string) string {
	if !strings.Contains(query, "?") {
		return query
	}

	var builder strings.Builder
	builder.Grow(len(query) + 8)

	index := 1
	for _, ch := range query {
		if ch == '?' {
			builder.WriteByte('$')
			builder.WriteString(strconv.Itoa(index))
			index++
			continue
		}
		builder.WriteRune(ch)
	}

	return builder.String()
}
