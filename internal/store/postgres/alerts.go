package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/ids"
)

var ErrAlertNotFound = errors.New("alert not found")

func (s *Store) EnsureDefaultAlertRules(ctx context.Context, bindingID string) error {
	rules := []struct {
		typ      string
		severity string
		channels []string
		enabled  bool
	}{
		{"movement_detected", "info", []string{"push"}, false},
		{"no_movement_detected", "warning", []string{"push", "sms"}, false},
		{"device_offline", "critical", []string{"push", "sms"}, true},
	}
	for _, r := range rules {
		ch, err := json.Marshal(r.channels)
		if err != nil {
			return err
		}
		_, err = s.pool.Exec(ctx, `
			INSERT INTO alert_rules (id, device_binding_id, type, enabled, severity, channels_json)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (device_binding_id, type) DO NOTHING
		`, ids.New("rule"), bindingID, r.typ, r.enabled, r.severity, ch)
		if err != nil {
			return err
		}
	}
	return nil
}

type CreateAlertParams struct {
	BindingID     string
	Type          string
	Severity      string
	Title         string
	Body          string
	SourceEventID string
	SeenAt        time.Time
}

func (s *Store) CreateAlert(ctx context.Context, p CreateAlertParams) (domain.Alert, error) {
	if p.SeenAt.IsZero() {
		p.SeenAt = time.Now().UTC()
	}
	var a domain.Alert
	if p.SourceEventID != "" {
		err := s.pool.QueryRow(ctx, `
			INSERT INTO alerts (
				id, device_binding_id, type, severity, status,
				title, body, source_event_id, first_seen_at, last_seen_at, created_at
			) VALUES ($1, $2, $3, $4, 'active', $5, $6, $7, $8, $8, $8)
			ON CONFLICT (source_event_id) WHERE source_event_id IS NOT NULL
			DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at
			RETURNING id, device_binding_id, type, severity, status, title, body,
			          first_seen_at, last_seen_at, acknowledged_at
		`, ids.New("alert"), p.BindingID, p.Type, p.Severity, p.Title, p.Body, p.SourceEventID, p.SeenAt).Scan(
			&a.ID, &a.DeviceBindingID, &a.Type, &a.Severity, &a.Status,
			&a.Title, &a.Body, &a.FirstSeenAt, &a.LastSeenAt, &a.AcknowledgedAt,
		)
		return a, err
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO alerts (
			id, device_binding_id, type, severity, status,
			title, body, first_seen_at, last_seen_at, created_at
		) VALUES ($1, $2, $3, $4, 'active', $5, $6, $7, $7, $7)
		RETURNING id, device_binding_id, type, severity, status, title, body,
		          first_seen_at, last_seen_at, acknowledged_at
	`, ids.New("alert"), p.BindingID, p.Type, p.Severity, p.Title, p.Body, p.SeenAt).Scan(
		&a.ID, &a.DeviceBindingID, &a.Type, &a.Severity, &a.Status,
		&a.Title, &a.Body, &a.FirstSeenAt, &a.LastSeenAt, &a.AcknowledgedAt,
	)
	return a, err
}

func (s *Store) CreateOrRefreshActiveAlert(ctx context.Context, p CreateAlertParams) (domain.Alert, bool, error) {
	if p.SeenAt.IsZero() {
		p.SeenAt = time.Now().UTC()
	}
	var existing domain.Alert
	err := s.pool.QueryRow(ctx, `
		SELECT id, device_binding_id, type, severity, status, title, body,
		       first_seen_at, last_seen_at, acknowledged_at
		FROM alerts
		WHERE device_binding_id = $1 AND type = $2 AND status = 'active'
	`, p.BindingID, p.Type).Scan(
		&existing.ID, &existing.DeviceBindingID, &existing.Type, &existing.Severity,
		&existing.Status, &existing.Title, &existing.Body,
		&existing.FirstSeenAt, &existing.LastSeenAt, &existing.AcknowledgedAt,
	)
	if err == nil {
		_, err = s.pool.Exec(ctx, `
			UPDATE alerts SET last_seen_at = $2, body = $3 WHERE id = $1
		`, existing.ID, p.SeenAt, p.Body)
		if err != nil {
			return domain.Alert{}, false, err
		}
		existing.LastSeenAt = p.SeenAt
		existing.Body = p.Body
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Alert{}, false, err
	}
	created, err := s.CreateAlert(ctx, p)
	return created, true, err
}

func (s *Store) ListAlerts(ctx context.Context, bindingID string) ([]domain.Alert, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_binding_id, type, severity, status, title, body,
		       first_seen_at, last_seen_at, acknowledged_at
		FROM alerts
		WHERE device_binding_id = $1
		ORDER BY first_seen_at DESC
	`, bindingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Alert
	for rows.Next() {
		var a domain.Alert
		if err := rows.Scan(
			&a.ID, &a.DeviceBindingID, &a.Type, &a.Severity, &a.Status,
			&a.Title, &a.Body, &a.FirstSeenAt, &a.LastSeenAt, &a.AcknowledgedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CountActiveAlerts(ctx context.Context, bindingID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alerts
		WHERE device_binding_id = $1 AND status = 'active'
	`, bindingID).Scan(&n)
	return n, err
}

func (s *Store) AckAlert(ctx context.Context, bindingID, alertID, userID string) (domain.Alert, error) {
	var a domain.Alert
	err := s.pool.QueryRow(ctx, `
		UPDATE alerts
		SET status = 'acknowledged', acknowledged_at = now(), acknowledged_by = $3
		WHERE id = $1 AND device_binding_id = $2 AND status = 'active'
		RETURNING id, device_binding_id, type, severity, status, title, body,
		          first_seen_at, last_seen_at, acknowledged_at
	`, alertID, bindingID, userID).Scan(
		&a.ID, &a.DeviceBindingID, &a.Type, &a.Severity, &a.Status,
		&a.Title, &a.Body, &a.FirstSeenAt, &a.LastSeenAt, &a.AcknowledgedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Alert{}, ErrAlertNotFound
	}
	return a, err
}

func (s *Store) ResolveActiveAlerts(ctx context.Context, bindingID, alertType string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE alerts
		SET status = 'resolved', resolved_at = now()
		WHERE device_binding_id = $1 AND type = $2 AND status = 'active'
	`, bindingID, alertType)
	return err
}

func (s *Store) AlertRuleChannels(ctx context.Context, bindingID, alertType string) ([]string, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT channels_json FROM alert_rules
		WHERE device_binding_id = $1 AND type = $2 AND enabled = true
		LIMIT 1
	`, bindingID, alertType).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return []string{"push"}, nil
	}
	if err != nil {
		return nil, err
	}
	var channels []string
	if err := json.Unmarshal(raw, &channels); err != nil {
		return []string{"push"}, nil
	}
	return channels, nil
}

func (s *Store) IsAlertRuleEnabled(ctx context.Context, bindingID, alertType string) (bool, error) {
	var enabled bool
	err := s.pool.QueryRow(ctx, `
		SELECT enabled FROM alert_rules
		WHERE device_binding_id = $1 AND type = $2
		LIMIT 1
	`, bindingID, alertType).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

func (s *Store) SetMovementAlertMode(ctx context.Context, bindingID, mode string, monitored bool) error {
	if !monitored {
		_, err := s.pool.Exec(ctx, `
			UPDATE alert_rules SET enabled = false, updated_at = now()
			WHERE device_binding_id = $1 AND type IN ('movement_detected', 'no_movement_detected')
		`, bindingID)
		return err
	}
	other := "movement_detected"
	if mode == "movement_detected" {
		other = "no_movement_detected"
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE alert_rules
		SET enabled = (type = $2), updated_at = now()
		WHERE device_binding_id = $1 AND type IN ($2, $3)
	`, bindingID, mode, other)
	return err
}

func (s *Store) ListVerifiedPhones(ctx context.Context, householdID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.phone
		FROM household_members hm
		JOIN users u ON u.id = hm.user_id
		WHERE hm.household_id = $1
		  AND u.phone IS NOT NULL
		  AND u.phone_verified_at IS NOT NULL
		  AND u.disabled_at IS NULL
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var phone string
		if err := rows.Scan(&phone); err != nil {
			return nil, err
		}
		out = append(out, phone)
	}
	return out, rows.Err()
}

func (s *Store) RecordNotificationDelivery(ctx context.Context, alertID, channel, destination, status, errMsg string) error {
	var dest, msg any
	if destination != "" {
		dest = destination
	}
	if errMsg != "" {
		msg = errMsg
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_deliveries (alert_id, channel, destination, status, error_message)
		VALUES ($1, $2, $3, $4, $5)
	`, alertID, channel, dest, status, msg)
	return err
}
