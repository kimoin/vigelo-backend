package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/ids"
)

var (
	ErrDeviceAlreadyBound = errors.New("device is already bound")
	ErrBindingNotFound    = errors.New("device binding not found")
)

type DeviceBindingRow struct {
	ID                     string
	HouseholdID            string
	DeviceID               string
	DisplayName            string
	RoomOrLocationLabel    string
	ClaimStatus            string
	VNMSLifecycleCache     *string
	LastContactAt          *time.Time
	LastVNMSSyncAt         *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Timezone               string
	WindowsJSON            []byte
	WindowsDeliveryState   string
	AlertMode              string
	Subscription           domain.Subscription
	TrialEndsAt            *time.Time
}

func (s *Store) GetBindingByDeviceID(ctx context.Context, deviceID string) (DeviceBindingRow, error) {
	return s.scanBinding(ctx, `
		SELECT b.id, b.household_id, b.device_id, b.display_name, b.room_or_location_label,
		       b.claim_status, b.vnms_lifecycle_cache, b.last_contact_at, b.last_vnms_sync_at,
		       b.created_at, b.updated_at,
		       COALESCE(m.timezone, h.timezone), COALESCE(m.windows_json, '[]'::jsonb),
		       COALESCE(m.delivery_state, 'not_configured'),
		       COALESCE(m.alert_mode, 'no_movement_detected'),
		       sub.status, sub.service_status, sub.plan_code, sub.current_period_end, sub.trial_ends_at
		FROM device_bindings b
		JOIN households h ON h.id = b.household_id
		LEFT JOIN monitored_window_intent m ON m.device_binding_id = b.id
		LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
		WHERE b.device_id = $1 AND b.removed_at IS NULL
	`, deviceID)
}

func (s *Store) GetBinding(ctx context.Context, bindingID string) (DeviceBindingRow, error) {
	return s.scanBinding(ctx, `
		SELECT b.id, b.household_id, b.device_id, b.display_name, b.room_or_location_label,
		       b.claim_status, b.vnms_lifecycle_cache, b.last_contact_at, b.last_vnms_sync_at,
		       b.created_at, b.updated_at,
		       COALESCE(m.timezone, h.timezone), COALESCE(m.windows_json, '[]'::jsonb),
		       COALESCE(m.delivery_state, 'not_configured'),
		       COALESCE(m.alert_mode, 'no_movement_detected'),
		       sub.status, sub.service_status, sub.plan_code, sub.current_period_end, sub.trial_ends_at
		FROM device_bindings b
		JOIN households h ON h.id = b.household_id
		LEFT JOIN monitored_window_intent m ON m.device_binding_id = b.id
		LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
		WHERE b.id = $1 AND b.removed_at IS NULL
	`, bindingID)
}

func (s *Store) ListBindingsByHousehold(ctx context.Context, householdID string) ([]DeviceBindingRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.household_id, b.device_id, b.display_name, b.room_or_location_label,
		       b.claim_status, b.vnms_lifecycle_cache, b.last_contact_at, b.last_vnms_sync_at,
		       b.created_at, b.updated_at,
		       COALESCE(m.timezone, h.timezone), COALESCE(m.windows_json, '[]'::jsonb),
		       COALESCE(m.delivery_state, 'not_configured'),
		       COALESCE(m.alert_mode, 'no_movement_detected'),
		       sub.status, sub.service_status, sub.plan_code, sub.current_period_end, sub.trial_ends_at
		FROM device_bindings b
		JOIN households h ON h.id = b.household_id
		LEFT JOIN monitored_window_intent m ON m.device_binding_id = b.id
		LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
		WHERE b.household_id = $1 AND b.removed_at IS NULL
		ORDER BY b.created_at
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeviceBindingRow
	for rows.Next() {
		row, err := scanBindingRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type CreateBindingParams struct {
	HouseholdID         string
	DeviceID            string
	DisplayName         string
	RoomOrLocationLabel string
	ClaimedByUserID     string
	Timezone            string
	TrialDays           int
	VNMSLifecycle       string
}

func (s *Store) CreateBinding(ctx context.Context, p CreateBindingParams) (DeviceBindingRow, error) {
	bindingID := ids.New("devbind")
	subID := ids.New("sub")
	intentID := ids.New("mwin")
	now := time.Now().UTC()
	trialEnd := now.AddDate(0, 0, p.TrialDays)
	if p.Timezone == "" {
		p.Timezone = "Europe/Helsinki"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceBindingRow{}, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM device_bindings WHERE device_id = $1 AND removed_at IS NULL
		)
	`, p.DeviceID).Scan(&exists); err != nil {
		return DeviceBindingRow{}, err
	}
	if exists {
		return DeviceBindingRow{}, ErrDeviceAlreadyBound
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO device_bindings (
			id, household_id, device_id, display_name, room_or_location_label,
			claim_status, claimed_by_user_id, vnms_lifecycle_cache, last_vnms_sync_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'enrolled', $6, $7, $8, $9, $9)
	`, bindingID, p.HouseholdID, p.DeviceID, p.DisplayName, p.RoomOrLocationLabel,
		p.ClaimedByUserID, p.VNMSLifecycle, now, now)
	if err != nil {
		return DeviceBindingRow{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO monitored_window_intent (
			id, device_binding_id, timezone, windows_json, alert_mode, delivery_state, created_at, updated_at
		) VALUES ($1, $2, $3, '[]'::jsonb, 'no_movement_detected', 'not_configured', $4, $4)
	`, intentID, bindingID, p.Timezone, now)
	if err != nil {
		return DeviceBindingRow{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO subscriptions (
			id, household_id, device_binding_id, status, service_status, plan_code,
			trial_ends_at, created_at, updated_at
		) VALUES ($1, $2, $3, 'trialing', 'service_active', 'device_monitoring_monthly', $4, $5, $5)
	`, subID, p.HouseholdID, bindingID, trialEnd, now)
	if err != nil {
		return DeviceBindingRow{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return DeviceBindingRow{}, err
	}
	if err := s.EnsureDefaultAlertRules(ctx, bindingID); err != nil {
		return DeviceBindingRow{}, err
	}
	return s.GetBinding(ctx, bindingID)
}

func (s *Store) UpdateBindingMeta(ctx context.Context, bindingID, displayName, roomLabel string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE device_bindings
		SET display_name = $2, room_or_location_label = $3, updated_at = now()
		WHERE id = $1 AND removed_at IS NULL
	`, bindingID, displayName, roomLabel)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func (s *Store) RemoveBinding(ctx context.Context, bindingID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE device_bindings
		SET removed_at = now(), removed_reason = 'user_removed', updated_at = now()
		WHERE id = $1 AND removed_at IS NULL
	`, bindingID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func (s *Store) ActivateSubscription(ctx context.Context, bindingID string) error {
	end := time.Now().UTC().AddDate(0, 1, 0)
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions
		SET status = 'active', service_status = 'service_active',
		    current_period_end = $2, trial_ends_at = NULL, updated_at = now()
		WHERE device_binding_id = $1
	`, bindingID, end)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func (s *Store) scanBinding(ctx context.Context, query string, arg string) (DeviceBindingRow, error) {
	row := s.pool.QueryRow(ctx, query, arg)
	return scanBindingFromRow(row)
}

type bindingScanner interface {
	Scan(dest ...any) error
}

func scanBindingRow(rows pgx.Rows) (DeviceBindingRow, error) {
	return scanBindingFromRow(rows)
}

func scanBindingFromRow(row bindingScanner) (DeviceBindingRow, error) {
	var out DeviceBindingRow
	var periodEnd *time.Time
	err := row.Scan(
		&out.ID, &out.HouseholdID, &out.DeviceID, &out.DisplayName, &out.RoomOrLocationLabel,
		&out.ClaimStatus, &out.VNMSLifecycleCache, &out.LastContactAt, &out.LastVNMSSyncAt,
		&out.CreatedAt, &out.UpdatedAt,
		&out.Timezone, &out.WindowsJSON, &out.WindowsDeliveryState, &out.AlertMode,
		&out.Subscription.Status, &out.Subscription.ServiceStatus, &out.Subscription.PlanCode,
		&periodEnd, &out.TrialEndsAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceBindingRow{}, ErrBindingNotFound
	}
	if err != nil {
		return DeviceBindingRow{}, err
	}
	out.Subscription.CurrentPeriodEnd = periodEnd
	return out, nil
}
