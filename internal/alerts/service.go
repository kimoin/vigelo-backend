package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kimoin/vigelo-backend/internal/devices"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/notifications"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

const (
	EventMovementDetected   = "monitored_window.movement_detected"
	EventNoMovementDetected = "monitored_window.no_movement_detected"
	EventMovementUplink     = "movement_uplink.accepted"
	EventDeviceStatus       = "device_status.received"
	EventPolicyDelivered    = "device.policy_delivered"
	EventLifecycleChanged   = "device.lifecycle_changed"
)

type Service struct {
	DB               *postgres.Store
	Devices          *devices.Service
	Notify           *notifications.Dispatcher
	OfflineThreshold time.Duration
}

func (s *Service) HandleEvent(ctx context.Context, eventID string, ev vnmsclient.Event) error {
	switch ev.EventType {
	case EventPolicyDelivered:
		if s.Devices != nil {
			return s.Devices.HandlePolicyDelivered(ctx, ev.Payload)
		}
		return nil
	case EventMovementUplink:
		return s.handleMovementUplink(ctx, ev.Payload)
	case EventDeviceStatus:
		return s.handleDeviceStatus(ctx, ev.Payload)
	case EventMovementDetected:
		return s.createFactAlert(ctx, eventID, ev.Payload, "movement_detected", "info",
			"Movement detected", "Movement detected in %s.")
	case EventNoMovementDetected:
		return s.createFactAlert(ctx, eventID, ev.Payload, "no_movement_detected", "warning",
			"No movement detected", "No movement detected in %s during the monitored window.")
	case EventLifecycleChanged:
		return s.handleLifecycleChanged(ctx, ev.Payload)
	default:
		return nil
	}
}

func (s *Service) CheckOffline(ctx context.Context) error {
	if s.DB == nil {
		return nil
	}
	candidates, err := s.DB.ListOfflineCandidates(ctx, s.OfflineThreshold)
	if err != nil {
		return err
	}
	for _, b := range candidates {
		body := fmt.Sprintf("Vigelo device has not checked in recently (%s).", labelFor(b))
		alert, created, err := s.DB.CreateOrRefreshActiveAlert(ctx, postgres.CreateAlertParams{
			BindingID: b.ID,
			Type:      "device_offline",
			Severity:  "critical",
			Title:     "Device offline",
			Body:      body,
			SeenAt:    time.Now().UTC(),
		})
		if err != nil {
			return err
		}
		if created {
			s.dispatch(ctx, alert, b.HouseholdID)
		}
	}
	return nil
}

func (s *Service) handleMovementUplink(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		DeviceID      string `json:"device_id"`
		ReceivedAtUTC string `json:"received_at_utc"`
		VoltageMv     *int   `json:"voltage_mv"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.DeviceID == "" {
		return nil
	}
	at := parseVNMSUTC(p.ReceivedAtUTC)
	if err := s.DB.UpdateDeviceContact(ctx, p.DeviceID, at, p.VoltageMv); err != nil {
		return err
	}
	proj, err := s.DB.GetBindingProjection(ctx, p.DeviceID)
	if err != nil {
		return nil
	}
	return s.DB.ResolveActiveAlerts(ctx, proj.ID, "device_offline")
}

func (s *Service) handleDeviceStatus(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		DeviceID      string `json:"device_id"`
		ReceivedAtUTC string `json:"received_at_utc"`
		VoltageMv     int    `json:"voltage_mv"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.DeviceID == "" {
		return nil
	}
	v := p.VoltageMv
	return s.DB.UpdateDeviceContact(ctx, p.DeviceID, parseVNMSUTC(p.ReceivedAtUTC), &v)
}

func (s *Service) handleLifecycleChanged(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		DeviceID string `json:"device_id"`
		State    string `json:"state"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.DeviceID == "" {
		return nil
	}
	return s.DB.UpdateLifecycleCache(ctx, p.DeviceID, p.State)
}

func (s *Service) createFactAlert(ctx context.Context, eventID string, payload json.RawMessage, alertType, severity, title, bodyFmt string) error {
	var p struct {
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.DeviceID == "" {
		return nil
	}
	proj, err := s.DB.GetBindingProjection(ctx, p.DeviceID)
	if err != nil {
		return nil
	}
	enabled, err := s.DB.IsAlertRuleEnabled(ctx, proj.ID, alertType)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	body := fmt.Sprintf(bodyFmt, labelFor(proj))
	alert, err := s.DB.CreateAlert(ctx, postgres.CreateAlertParams{
		BindingID:     proj.ID,
		Type:          alertType,
		Severity:      severity,
		Title:         title,
		Body:          body,
		SourceEventID: eventID,
		SeenAt:        time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	s.dispatch(ctx, alert, proj.HouseholdID)
	return nil
}

func (s *Service) dispatch(ctx context.Context, alert domain.Alert, householdID string) {
	if s.Notify != nil {
		s.Notify.NotifyAlert(ctx, alert, householdID)
	}
}

func labelFor(p postgres.BindingProjection) string {
	if p.RoomLabel != "" {
		return p.RoomLabel
	}
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return "the monitored location"
}

func parseVNMSUTC(raw string) time.Time {
	if raw == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}
