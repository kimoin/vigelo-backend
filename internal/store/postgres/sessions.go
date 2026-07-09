package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kimoin/vigelo-backend/internal/auth"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/ids"
)

func insertSession(ctx context.Context, tx pgx.Tx, userID string, accessTTL, refreshTTL time.Duration, rotatedFrom *string) (domain.SessionTokens, error) {
	accessToken, err := auth.NewToken()
	if err != nil {
		return domain.SessionTokens{}, err
	}
	refreshToken, err := auth.NewToken()
	if err != nil {
		return domain.SessionTokens{}, err
	}
	sessionID := ids.New("sess")
	now := time.Now().UTC()
	accessExp := now.Add(accessTTL)
	refreshExp := now.Add(refreshTTL)

	var rotated any
	if rotatedFrom != nil {
		rotated = *rotatedFrom
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO sessions (
			id, user_id, access_token_hash, refresh_token_hash,
			rotated_from_session_id, access_expires_at, refresh_expires_at,
			last_seen_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, sessionID, userID, auth.HashToken(accessToken), auth.HashToken(refreshToken),
		rotated, accessExp, refreshExp, now, now)
	if err != nil {
		return domain.SessionTokens{}, err
	}
	return domain.SessionTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionID:    sessionID,
	}, nil
}

func (s *Store) RequestPasswordReset(ctx context.Context, email string, ttl time.Duration) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var userID string
	err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	raw, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, ids.New("rstok"), userID, auth.HashToken(raw), time.Now().UTC().Add(ttl), time.Now().UTC())
	return raw, err
}

func (s *Store) CompletePasswordReset(ctx context.Context, rawToken, newPassword string) error {
	now := time.Now().UTC()
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID, tokenID string
	err = tx.QueryRow(ctx, `
		SELECT id, user_id FROM password_reset_tokens
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
	`, auth.HashToken(rawToken), now).Scan(&tokenID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrInvalidToken
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, hash)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE password_reset_tokens SET used_at = $2 WHERE id = $1`, tokenID, now)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func pgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
