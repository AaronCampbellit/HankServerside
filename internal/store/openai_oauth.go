package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

func (s *Store) UpsertOpenAIAccount(ctx context.Context, account domain.OpenAIAccount) error {
	_, err := s.exec(ctx, `INSERT INTO openai_accounts (user_id, provider_user_id, access_token, refresh_token, token_type, scope, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			provider_user_id = excluded.provider_user_id,
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_type = excluded.token_type,
			scope = excluded.scope,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at`,
		account.UserID, account.ProviderUserID, account.AccessToken, account.RefreshToken, account.TokenType, account.Scope, account.ExpiresAt, account.CreatedAt, account.UpdatedAt)
	return err
}

func (s *Store) GetOpenAIAccount(ctx context.Context, userID string) (domain.OpenAIAccount, error) {
	var account domain.OpenAIAccount
	err := s.queryRow(ctx, `SELECT user_id, provider_user_id, access_token, refresh_token, token_type, scope, expires_at, created_at, updated_at
		FROM openai_accounts WHERE user_id = ?`, userID).Scan(&account.UserID, &account.ProviderUserID, &account.AccessToken, &account.RefreshToken, &account.TokenType, &account.Scope, &account.ExpiresAt, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.OpenAIAccount{}, ErrNotFound
		}
		return domain.OpenAIAccount{}, err
	}
	return account, nil
}

func (s *Store) UpsertOpenAIOAuthState(ctx context.Context, state domain.OpenAIOAuthState) error {
	_, err := s.exec(ctx, `INSERT INTO openai_oauth_states (state_hash, user_id, code_verifier, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(state_hash) DO UPDATE SET user_id = excluded.user_id, code_verifier = excluded.code_verifier, created_at = excluded.created_at, expires_at = excluded.expires_at`,
		state.StateHash, state.UserID, state.CodeVerifier, state.CreatedAt, state.ExpiresAt)
	return err
}

func (s *Store) ConsumeOpenAIOAuthState(ctx context.Context, stateHash string) (domain.OpenAIOAuthState, error) {
	tx, err := s.beginTx(ctx, nil)
	if err != nil {
		return domain.OpenAIOAuthState{}, err
	}
	defer tx.Rollback()
	var state domain.OpenAIOAuthState
	err = tx.QueryRowContext(ctx, `SELECT state_hash, user_id, code_verifier, created_at, expires_at FROM openai_oauth_states WHERE state_hash = ?`, stateHash).Scan(&state.StateHash, &state.UserID, &state.CodeVerifier, &state.CreatedAt, &state.ExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.OpenAIOAuthState{}, ErrNotFound
		}
		return domain.OpenAIOAuthState{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM openai_oauth_states WHERE state_hash = ?`, stateHash); err != nil {
		return domain.OpenAIOAuthState{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.OpenAIOAuthState{}, err
	}
	if time.Now().UTC().After(state.ExpiresAt) {
		return domain.OpenAIOAuthState{}, ErrNotFound
	}
	return state, nil
}
