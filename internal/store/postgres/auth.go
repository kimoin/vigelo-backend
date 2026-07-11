package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kimoin/vigelo-backend/internal/auth"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/ids"
)

type SignupResult struct {
	User       domain.User
	Household  domain.Household
	Tokens     domain.SessionTokens
	VerifyRaw  string
}

func (s *Store) Signup(ctx context.Context, email, password, displayName string, verifyTTL, accessTTL, refreshTTL time.Duration) (SignupResult, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return SignupResult{}, err
	}
	userID := ids.New("user")
	householdID := ids.New("hh")
	now := time.Now().UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SignupResult{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, email, displayName, hash, now)
	if pgUniqueViolation(err) {
		return SignupResult{}, auth.ErrEmailTaken
	}
	if err != nil {
		return SignupResult{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO households (id, name, timezone, created_by_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, householdID, "Home", "Europe/Helsinki", userID, now)
	if err != nil {
		return SignupResult{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO household_members (household_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`, householdID, userID, now)
	if err != nil {
		return SignupResult{}, err
	}

	tokens, err := insertSession(ctx, tx, userID, accessTTL, refreshTTL, nil)
	if err != nil {
		return SignupResult{}, err
	}

	verifyToken, err := auth.NewToken()
	if err != nil {
		return SignupResult{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, ids.New("evtok"), userID, auth.HashToken(verifyToken), now.Add(verifyTTL), now)
	if err != nil {
		return SignupResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return SignupResult{}, err
	}

	return SignupResult{
		User: domain.User{
			ID:          userID,
			Email:       email,
			DisplayName: displayName,
			CreatedAt:   now,
		},
		Household: domain.Household{
			ID:        householdID,
			Name:      "Home",
			Timezone:  "Europe/Helsinki",
			OwnerID:   userID,
			Role:      "owner",
			CreatedAt: now,
		},
		Tokens:    tokens,
		VerifyRaw: verifyToken,
	}, nil
}

func (s *Store) Login(ctx context.Context, email, password string, accessTTL, refreshTTL time.Duration) (domain.User, domain.SessionTokens, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u domain.User
	var disabledAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, display_name, password_hash, email_verified_at, timezone, disabled_at, created_at
		FROM users WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.EmailVerifiedAt, &u.Timezone, &disabledAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.SessionTokens{}, auth.ErrInvalidLogin
	}
	if err != nil {
		return domain.User{}, domain.SessionTokens{}, err
	}
	if disabledAt != nil {
		return domain.User{}, domain.SessionTokens{}, auth.ErrUserDisabled
	}
	ok, err := auth.VerifyPassword(password, u.PasswordHash)
	if err != nil || !ok {
		return domain.User{}, domain.SessionTokens{}, auth.ErrInvalidLogin
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, domain.SessionTokens{}, err
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, `UPDATE users SET last_login_at = $2 WHERE id = $1`, u.ID, time.Now().UTC())
	tokens, err := insertSession(ctx, tx, u.ID, accessTTL, refreshTTL, nil)
	if err != nil {
		return domain.User{}, domain.SessionTokens{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, domain.SessionTokens{}, err
	}
	u.PasswordHash = ""
	return u, tokens, nil
}

func (s *Store) ResolveAccessToken(ctx context.Context, accessToken string) (userID, sessionID string, err error) {
	now := time.Now().UTC()
	err = s.pool.QueryRow(ctx, `
		SELECT id, user_id FROM sessions
		WHERE access_token_hash = $1
		  AND revoked_at IS NULL
		  AND access_expires_at > $2
	`, auth.HashToken(accessToken), now).Scan(&sessionID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", auth.ErrInvalidSession
	}
	return userID, sessionID, err
}

func (s *Store) RefreshSession(ctx context.Context, accessToken string, accessTTL, refreshTTL time.Duration) (domain.SessionTokens, error) {
	userID, sessionID, err := s.ResolveAccessToken(ctx, accessToken)
	if err != nil {
		return domain.SessionTokens{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.SessionTokens{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE id = $1`, sessionID, now)
	if err != nil {
		return domain.SessionTokens{}, err
	}
	tokens, err := insertSession(ctx, tx, userID, accessTTL, refreshTTL, &sessionID)
	if err != nil {
		return domain.SessionTokens{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.SessionTokens{}, err
	}
	return tokens, nil
}

func (s *Store) Logout(ctx context.Context, accessToken string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = $2
		WHERE access_token_hash = $1 AND revoked_at IS NULL
	`, auth.HashToken(accessToken), time.Now().UTC())
	return err
}

func (s *Store) GetUserByID(ctx context.Context, userID string) (domain.User, error) {
	var u domain.User
	var disabledAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, display_name, email_verified_at, timezone, disabled_at, created_at
		FROM users WHERE id = $1
	`, userID).Scan(&u.ID, &u.Email, &u.DisplayName, &u.EmailVerifiedAt, &u.Timezone, &disabledAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, auth.ErrInvalidSession
	}
	if disabledAt != nil {
		return domain.User{}, auth.ErrUserDisabled
	}
	return u, err
}

func (s *Store) UpdateUser(ctx context.Context, userID string, displayName, timezone *string) (domain.User, error) {
	if displayName != nil {
		_, err := s.pool.Exec(ctx, `UPDATE users SET display_name = $2 WHERE id = $1`, userID, strings.TrimSpace(*displayName))
		if err != nil {
			return domain.User{}, err
		}
	}
	if timezone != nil {
		_, err := s.pool.Exec(ctx, `UPDATE users SET timezone = $2 WHERE id = $1`, userID, strings.TrimSpace(*timezone))
		if err != nil {
			return domain.User{}, err
		}
	}
	return s.GetUserByID(ctx, userID)
}

func (s *Store) VerifyEmail(ctx context.Context, rawToken string) error {
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID string
	var tokenID string
	err = tx.QueryRow(ctx, `
		SELECT id, user_id FROM email_verification_tokens
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
	`, auth.HashToken(rawToken), now).Scan(&tokenID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrInvalidToken
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET email_verified_at = $2 WHERE id = $1`, userID, now)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE email_verification_tokens SET used_at = $2 WHERE id = $1`, tokenID, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrInvalidSession
	}
	if err != nil {
		return err
	}
	ok, err := auth.VerifyPassword(currentPassword, hash)
	if err != nil || !ok {
		return auth.ErrWrongPassword
	}
	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, newHash)
	return err
}
