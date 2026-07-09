package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kimoin/vigelo-backend/internal/domain"
)

type UpdateMonitoredWindowsParams struct {
	BindingID     string
	Timezone      string
	Windows       []domain.MonitoredWindow
	AlertMode     string
	UpdatedByUser string
	DeliveryState string
	SentToVNMS    bool
}

func (s *Store) UpdateMonitoredWindows(ctx context.Context, p UpdateMonitoredWindowsParams) error {
	b, err := json.Marshal(p.Windows)
	if err != nil {
		return err
	}
	if p.DeliveryState == "" {
		p.DeliveryState = "pending_delivery"
	}
	var sentAt any
	if p.SentToVNMS {
		sentAt = time.Now().UTC()
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE monitored_window_intent
		SET timezone = $2,
		    windows_json = $3,
		    alert_mode = $4,
		    delivery_state = $5,
		    desired_version = desired_version + 1,
		    last_sent_to_vnms_at = COALESCE($6, last_sent_to_vnms_at),
		    updated_by_user_id = $7,
		    updated_at = now()
		WHERE device_binding_id = $1
	`, p.BindingID, p.Timezone, b, p.AlertMode, p.DeliveryState, sentAt, nullableUser(p.UpdatedByUser))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func (s *Store) MarkMonitoredWindowsDelivered(ctx context.Context, deviceID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE monitored_window_intent m
		SET delivery_state = 'delivered',
		    last_delivered_by_vnms_at = now(),
		    updated_at = now()
		FROM device_bindings b
		WHERE b.id = m.device_binding_id
		  AND b.device_id = $1
		  AND b.removed_at IS NULL
	`, deviceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func nullableUser(id string) any {
	if id == "" {
		return nil
	}
	return id
}
