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

func (s *Store) ListHouseholds(ctx context.Context, userID string) ([]domain.Household, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT h.id, h.name, h.timezone, h.country, h.created_by_user_id, hm.role, h.created_at
		FROM households h
		JOIN household_members hm ON hm.household_id = h.id
		WHERE hm.user_id = $1 AND h.archived_at IS NULL
		ORDER BY h.created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Household
	for rows.Next() {
		var h domain.Household
		if err := rows.Scan(&h.ID, &h.Name, &h.Timezone, &h.Country, &h.OwnerID, &h.Role, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) CreateHousehold(ctx context.Context, userID, name, timezone string) (domain.Household, error) {
	householdID := ids.New("hh")
	now := time.Now().UTC()
	if timezone == "" {
		timezone = "Europe/Helsinki"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Household{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO households (id, name, timezone, created_by_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, householdID, name, timezone, userID, now)
	if err != nil {
		return domain.Household{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO household_members (household_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`, householdID, userID, now)
	if err != nil {
		return domain.Household{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Household{}, err
	}
	return domain.Household{
		ID:        householdID,
		Name:      name,
		Timezone:  timezone,
		OwnerID:   userID,
		Role:      "owner",
		CreatedAt: now,
	}, nil
}

func (s *Store) GetMembership(ctx context.Context, householdID, userID string) (role string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT role FROM household_members
		WHERE household_id = $1 AND user_id = $2
	`, householdID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrForbidden
	}
	return role, err
}

func (s *Store) GetHouseholdTimezone(ctx context.Context, householdID string) (string, error) {
	var tz string
	err := s.pool.QueryRow(ctx, `
		SELECT timezone FROM households WHERE id = $1 AND archived_at IS NULL
	`, householdID).Scan(&tz)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrForbidden
	}
	return tz, err
}

func (s *Store) UpdateHousehold(ctx context.Context, householdID, requesterID string, name, timezone *string) (domain.Household, error) {
	if _, err := s.GetMembership(ctx, householdID, requesterID); err != nil {
		return domain.Household{}, err
	}
	row := s.pool.QueryRow(ctx, `
		SELECT h.id, h.name, h.timezone, h.country, h.created_by_user_id, hm.role, h.created_at
		FROM households h
		JOIN household_members hm ON hm.household_id = h.id AND hm.user_id = $2
		WHERE h.id = $1 AND h.archived_at IS NULL
	`, householdID, requesterID)
	var h domain.Household
	if err := row.Scan(&h.ID, &h.Name, &h.Timezone, &h.Country, &h.OwnerID, &h.Role, &h.CreatedAt); err != nil {
		return domain.Household{}, err
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		h.Name = strings.TrimSpace(*name)
	}
	if timezone != nil && strings.TrimSpace(*timezone) != "" {
		h.Timezone = strings.TrimSpace(*timezone)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE households SET name = $2, timezone = $3 WHERE id = $1
	`, householdID, h.Name, h.Timezone)
	if err != nil {
		return domain.Household{}, err
	}
	return h, nil
}

func (s *Store) ListMembers(ctx context.Context, householdID, requesterID string) ([]domain.HouseholdMember, error) {
	if _, err := s.GetMembership(ctx, householdID, requesterID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.display_name, hm.role, hm.created_at
		FROM household_members hm
		JOIN users u ON u.id = hm.user_id
		WHERE hm.household_id = $1
		ORDER BY hm.created_at
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.HouseholdMember
	for rows.Next() {
		var m domain.HouseholdMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type InviteResult struct {
	Invite   domain.HouseholdInvite
	TokenRaw string
}

func (s *Store) CreateInvite(ctx context.Context, householdID, invitedBy, email, role string, ttl time.Duration) (InviteResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	memberRole, err := s.GetMembership(ctx, householdID, invitedBy)
	if err != nil {
		return InviteResult{}, err
	}
	if memberRole != "owner" && memberRole != "admin" {
		return InviteResult{}, auth.ErrForbidden
	}
	if role == "" {
		role = "caregiver"
	}
	if role == "owner" {
		return InviteResult{}, auth.ErrForbidden
	}

	raw, err := auth.NewToken()
	if err != nil {
		return InviteResult{}, err
	}
	inviteID := ids.New("inv")
	now := time.Now().UTC()
	expires := now.Add(ttl)

	_, err = s.pool.Exec(ctx, `
		INSERT INTO household_invites (id, household_id, email, role, token_hash, invited_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, inviteID, householdID, email, role, auth.HashToken(raw), invitedBy, expires, now)
	if err != nil {
		return InviteResult{}, err
	}

	return InviteResult{
		Invite: domain.HouseholdInvite{
			ID:          inviteID,
			HouseholdID: householdID,
			Email:       email,
			Role:        role,
			ExpiresAt:   expires,
			CreatedAt:   now,
		},
		TokenRaw: raw,
	}, nil
}

func (s *Store) AcceptInvite(ctx context.Context, rawToken, userID string) (domain.Household, error) {
	now := time.Now().UTC()
	var inviteID, householdID, role, inviteEmail, userEmail string
	err := s.pool.QueryRow(ctx, `
		SELECT hi.id, hi.household_id, hi.role, hi.email, u.email
		FROM household_invites hi
		JOIN users u ON u.id = $2
		WHERE hi.token_hash = $1 AND hi.accepted_at IS NULL AND hi.expires_at > $3
	`, auth.HashToken(rawToken), userID, now).Scan(&inviteID, &householdID, &role, &inviteEmail, &userEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Household{}, auth.ErrInviteNotFound
	}
	if err != nil {
		return domain.Household{}, err
	}
	if strings.ToLower(inviteEmail) != strings.ToLower(userEmail) {
		return domain.Household{}, auth.ErrInviteNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Household{}, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM household_members WHERE household_id = $1 AND user_id = $2
		)
	`, householdID, userID).Scan(&exists)
	if err != nil {
		return domain.Household{}, err
	}
	if exists {
		return domain.Household{}, auth.ErrAlreadyMember
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO household_members (household_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
	`, householdID, userID, role, now)
	if err != nil {
		return domain.Household{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE household_invites SET accepted_at = $2 WHERE id = $1`, inviteID, now)
	if err != nil {
		return domain.Household{}, err
	}

	var h domain.Household
	err = tx.QueryRow(ctx, `
		SELECT id, name, timezone, country, created_by_user_id, created_at
		FROM households WHERE id = $1
	`, householdID).Scan(&h.ID, &h.Name, &h.Timezone, &h.Country, &h.OwnerID, &h.CreatedAt)
	if err != nil {
		return domain.Household{}, err
	}
	h.Role = role
	if err := tx.Commit(ctx); err != nil {
		return domain.Household{}, err
	}
	return h, nil
}
