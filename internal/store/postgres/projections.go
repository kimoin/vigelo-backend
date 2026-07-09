package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type BindingProjection struct {
	ID           string
	HouseholdID  string
	DeviceID     string
	DisplayName  string
	RoomLabel    string
	LastContact  *time.Time
	LastVoltageMv *int
}

func (s *Store) UpdateDeviceContact(ctx context.Context, deviceID string, at time.Time, voltageMv *int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE device_bindings
		SET last_contact_at = GREATEST(COALESCE(last_contact_at, '-infinity'::timestamptz), $2),
		    last_voltage_mv = COALESCE($3, last_voltage_mv),
		    last_vnms_sync_at = now(),
		    updated_at = now()
		WHERE device_id = $1 AND removed_at IS NULL
	`, deviceID, at, voltageMv)
	return err
}

func (s *Store) UpdateLifecycleCache(ctx context.Context, deviceID, lifecycle string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE device_bindings
		SET vnms_lifecycle_cache = $2, last_vnms_sync_at = now(), updated_at = now()
		WHERE device_id = $1 AND removed_at IS NULL
	`, deviceID, lifecycle)
	return err
}

func (s *Store) GetBindingProjection(ctx context.Context, deviceID string) (BindingProjection, error) {
	var p BindingProjection
	err := s.pool.QueryRow(ctx, `
		SELECT id, household_id, device_id, display_name, room_or_location_label,
		       last_contact_at, last_voltage_mv
		FROM device_bindings
		WHERE device_id = $1 AND removed_at IS NULL
	`, deviceID).Scan(
		&p.ID, &p.HouseholdID, &p.DeviceID, &p.DisplayName, &p.RoomLabel,
		&p.LastContact, &p.LastVoltageMv,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BindingProjection{}, ErrBindingNotFound
	}
	return p, err
}

func (s *Store) ListOfflineCandidates(ctx context.Context, threshold time.Duration) ([]BindingProjection, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.household_id, b.device_id, b.display_name, b.room_or_location_label,
		       b.last_contact_at, b.last_voltage_mv
		FROM device_bindings b
		JOIN subscriptions s ON s.device_binding_id = b.id
		WHERE b.removed_at IS NULL
		  AND b.last_contact_at IS NOT NULL
		  AND b.last_contact_at < $1
		  AND s.service_status = 'service_active'
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BindingProjection
	for rows.Next() {
		var p BindingProjection
		if err := rows.Scan(
			&p.ID, &p.HouseholdID, &p.DeviceID, &p.DisplayName, &p.RoomLabel,
			&p.LastContact, &p.LastVoltageMv,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
