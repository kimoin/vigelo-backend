package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/schedule"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

var (
	ErrInvalidMonitoredWindows = errors.New("invalid monitored windows")
)

type MonitoredWindowsInput struct {
	BindingID string
	UserID    string
	Timezone  string
	Windows   []domain.MonitoredWindow
	AlertMode string
}

type MonitoredWindowsView struct {
	Timezone      string                   `json:"timezone"`
	Windows       []domain.MonitoredWindow `json:"windows"`
	AlertMode     string                   `json:"alert_mode"`
	DeliveryState string                   `json:"delivery_state"`
}

func (s *Service) GetMonitoredWindows(ctx context.Context, bindingID string) (MonitoredWindowsView, error) {
	row, err := s.DB.GetBinding(ctx, bindingID)
	if err != nil {
		return MonitoredWindowsView{}, err
	}
	var windows []domain.MonitoredWindow
	_ = decodeWindows(row.WindowsJSON, &windows)
	return MonitoredWindowsView{
		Timezone:      row.Timezone,
		Windows:       windows,
		AlertMode:     row.AlertMode,
		DeliveryState: row.WindowsDeliveryState,
	}, nil
}

func (s *Service) SetMonitoredWindows(ctx context.Context, in MonitoredWindowsInput) (MonitoredWindowsView, error) {
	if s.DB == nil {
		return MonitoredWindowsView{}, fmt.Errorf("database required")
	}
	if err := schedule.ValidateLocalWindows(in.Windows); err != nil {
		return MonitoredWindowsView{}, fmt.Errorf("%w: %v", ErrInvalidMonitoredWindows, err)
	}

	alertMode := in.AlertMode
	if alertMode == "" {
		alertMode = domain.AlertModeNoMovement
	}
	if len(in.Windows) > 0 && !domain.ValidMonitoredWindowAlertMode(alertMode) {
		return MonitoredWindowsView{}, fmt.Errorf("%w: alert_mode must be movement_detected or no_movement_detected", ErrInvalidMonitoredWindows)
	}

	row, err := s.DB.GetBinding(ctx, in.BindingID)
	if err != nil {
		return MonitoredWindowsView{}, err
	}
	tz := in.Timezone
	if tz == "" {
		tz = row.Timezone
	}
	loc, err := schedule.LoadLocation(tz)
	if err != nil {
		return MonitoredWindowsView{}, fmt.Errorf("%w: %v", ErrInvalidMonitoredWindows, err)
	}

	utcWindows, err := schedule.LocalToUTC(loc, time.Now().UTC(), in.Windows)
	if err != nil {
		return MonitoredWindowsView{}, fmt.Errorf("%w: %v", ErrInvalidMonitoredWindows, err)
	}

	deliveryState := "pending_delivery"
	sent := false
	if s.VNMS != nil {
		if _, err := s.VNMS.SetMonitoredWindows(ctx, row.DeviceID, utcWindows); err != nil {
			deliveryState = "sync_failed"
		} else {
			sent = true
		}
	} else {
		deliveryState = "sync_failed"
	}

	if err := s.DB.UpdateMonitoredWindows(ctx, postgres.UpdateMonitoredWindowsParams{
		BindingID:     in.BindingID,
		Timezone:      tz,
		Windows:       in.Windows,
		AlertMode:     alertMode,
		UpdatedByUser: in.UserID,
		DeliveryState: deliveryState,
		SentToVNMS:    sent,
	}); err != nil {
		return MonitoredWindowsView{}, err
	}
	if err := s.DB.SetMovementAlertMode(ctx, in.BindingID, alertMode, len(in.Windows) > 0); err != nil {
		return MonitoredWindowsView{}, err
	}
	return MonitoredWindowsView{
		Timezone:      tz,
		Windows:       in.Windows,
		AlertMode:     alertMode,
		DeliveryState: deliveryState,
	}, nil
}

func (s *Service) maybeConfirmDelivery(ctx context.Context, row postgres.DeviceBindingRow, vnms vnmsclient.DeviceState) {
	if row.WindowsDeliveryState != "pending_delivery" || vnms.DeviceID == "" || s.DB == nil {
		return
	}
	var local []domain.MonitoredWindow
	_ = decodeWindows(row.WindowsJSON, &local)
	loc, err := schedule.LoadLocation(row.Timezone)
	if err != nil {
		return
	}
	if !schedule.UTCMatchesIntent(loc, time.Now().UTC(), local, vnms.MonitoredWindows) {
		return
	}
	_ = s.DB.MarkMonitoredWindowsDelivered(ctx, row.DeviceID)
}

type policyDeliveredPayload struct {
	DeviceID string `json:"device_id"`
}

func (s *Service) HandlePolicyDelivered(ctx context.Context, payload json.RawMessage) error {
	var p policyDeliveredPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	if p.DeviceID == "" {
		return nil
	}
	return s.DB.MarkMonitoredWindowsDelivered(ctx, p.DeviceID)
}
