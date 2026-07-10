package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/ids"
)

type PushTokenRecord struct {
	ID          string
	UserID      string
	Platform    string
	Token       string
	Environment string
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) UpsertPushToken(ctx context.Context, userID, sessionID, platform, token, environment string) (*domain.PushToken, error) {
	if token == "" || platform == "" {
		return nil, errors.New("platform and token are required")
	}
	if environment == "" {
		environment = "production"
	}
	id := ids.New("push")
	hash := tokenHash(token)
	var session any
	if sessionID != "" {
		session = sessionID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO push_tokens (
			id, user_id, session_id, platform, token_hash, token_encrypted,
			environment, enabled, last_registered_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, true, now())
		ON CONFLICT (user_id, token_hash) DO UPDATE SET
			platform = EXCLUDED.platform,
			token_encrypted = EXCLUDED.token_encrypted,
			environment = EXCLUDED.environment,
			enabled = true,
			last_registered_at = now(),
			last_delivery_error = NULL
	`, id, userID, session, platform, hash, []byte(token), environment)
	if err != nil {
		return nil, err
	}
	var outID string
	var registered time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT id, last_registered_at FROM push_tokens
		WHERE user_id = $1 AND token_hash = $2
	`, userID, hash).Scan(&outID, &registered)
	if err != nil {
		return nil, err
	}
	return &domain.PushToken{
		ID:          outID,
		Platform:    platform,
		Environment: environment,
		TokenHint:   tokenHint(token),
		CreatedAt:   registered,
	}, nil
}

func (s *Store) DeletePushToken(ctx context.Context, userID, pushTokenID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM push_tokens WHERE id = $1 AND user_id = $2
	`, pushTokenID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ListHouseholdPushTokens(ctx context.Context, householdID string) ([]PushTokenRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pt.id, pt.user_id, pt.platform, pt.token_encrypted, pt.environment
		FROM push_tokens pt
		JOIN household_members hm ON hm.user_id = pt.user_id
		WHERE hm.household_id = $1
		  AND pt.enabled = true
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushTokenRecord
	for rows.Next() {
		var rec PushTokenRecord
		var encrypted []byte
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Platform, &encrypted, &rec.Environment); err != nil {
			return nil, err
		}
		rec.Token = string(encrypted)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) RecordPushTokenDeliveryError(ctx context.Context, pushTokenID, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE push_tokens
		SET last_delivery_error = $2
		WHERE id = $1
	`, pushTokenID, errMsg)
	return err
}

func tokenHint(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
