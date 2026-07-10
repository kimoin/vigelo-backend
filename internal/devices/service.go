package devices

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

const enrollmentKeyHexLen = 32 // AES-128 key as 32 hex characters

var (
	ErrVNMSNotConfigured   = errors.New("vnms is not configured")
	ErrMissingEnrollment   = errors.New("missing enrollment fields")
	ErrInvalidEnrollment   = errors.New("invalid enrollment data")
	ErrEnrollmentRejected  = errors.New("enrollment rejected by vnms")
	ErrDeviceAlreadyActive = errors.New("device is already active on the network")
)

type VNMS interface {
	VerifyEnrollment(ctx context.Context, deviceID, deviceKeyHex string) (vnmsclient.EnrollmentView, error)
	ProvisionInventory(ctx context.Context, deviceID, deviceKeyHex string) error
	Enable(ctx context.Context, deviceID string) error
	BatchGet(ctx context.Context, deviceIDs []string) (map[string]vnmsclient.DeviceState, error)
	SetMonitoredWindows(ctx context.Context, deviceID string, windows []vnmsclient.Window) ([]vnmsclient.Window, error)
}

type Service struct {
	DB               *postgres.Store
	VNMS             VNMS
	OfflineThreshold time.Duration
	TrialDays        int
}

type RegisterInput struct {
	HouseholdID         string
	UserID              string
	DeviceID            string
	EnrollmentSecret    string
	DisplayName         string
	RoomOrLocationLabel string
	Timezone            string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.DeviceBinding, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("database required")
	}
	if s.VNMS == nil {
		return nil, ErrVNMSNotConfigured
	}
	deviceID := strings.TrimSpace(in.DeviceID)
	secret := normalizeEnrollmentSecret(in.EnrollmentSecret)
	if deviceID == "" || secret == "" {
		return nil, ErrMissingEnrollment
	}
	if len(deviceID) > 64 || !validEnrollmentSecret(secret) {
		return nil, ErrInvalidEnrollment
	}

	view, err := s.ensureVNMSDevice(ctx, deviceID, secret)
	if err != nil {
		return nil, err
	}
	if !view.Provisioned || !view.Verified {
		return nil, ErrEnrollmentRejected
	}

	row, err := s.DB.CreateBinding(ctx, postgres.CreateBindingParams{
		HouseholdID:         in.HouseholdID,
		DeviceID:            deviceID,
		DisplayName:         fallback(in.DisplayName, "Vigelo device"),
		RoomOrLocationLabel: fallback(in.RoomOrLocationLabel, "Home"),
		ClaimedByUserID:     in.UserID,
		Timezone:            in.Timezone,
		TrialDays:           s.TrialDays,
		VNMSLifecycle:       view.LifecycleState,
	})
	if err != nil {
		return nil, err
	}

	if view.LifecycleState != "active" {
		if err := s.VNMS.Enable(ctx, deviceID); err != nil {
			_ = s.DB.RemoveBinding(ctx, row.ID)
			return nil, fmt.Errorf("vnms enable: %w", err)
		}
		row.VNMSLifecycleCache = strPtr("active")
	}

	states, err := s.VNMS.BatchGet(ctx, []string{deviceID})
	if err != nil {
		return ProjectBinding(row, vnmsclient.DeviceState{}, s.OfflineThreshold), nil
	}
	return ProjectBinding(row, states[deviceID], s.OfflineThreshold), nil
}

func (s *Service) ensureVNMSDevice(ctx context.Context, deviceID, secret string) (vnmsclient.EnrollmentView, error) {
	view, err := s.VNMS.VerifyEnrollment(ctx, deviceID, secret)
	if err == nil {
		if view.LifecycleState == "active" {
			return view, ErrDeviceAlreadyActive
		}
		return view, nil
	}
	if errors.Is(err, vnmsclient.ErrForbidden) {
		return view, ErrEnrollmentRejected
	}
	if errors.Is(err, vnmsclient.ErrConflict) {
		return view, ErrDeviceAlreadyActive
	}
	if errors.Is(err, vnmsclient.ErrInvalidRequest) {
		return view, ErrInvalidEnrollment
	}
	if errors.Is(err, vnmsclient.ErrNotFound) {
		if err := s.VNMS.ProvisionInventory(ctx, deviceID, secret); err != nil {
			return view, mapProvisionError(err)
		}
		return vnmsclient.EnrollmentView{
			Verified:       true,
			LifecycleState: "disabled",
			Provisioned:    true,
		}, nil
	}
	return view, err
}

func mapProvisionError(err error) error {
	if errors.Is(err, vnmsclient.ErrConflict) {
		return ErrDeviceAlreadyActive
	}
	if errors.Is(err, vnmsclient.ErrInvalidRequest) {
		return ErrInvalidEnrollment
	}
	if errors.Is(err, vnmsclient.ErrForbidden) {
		return ErrEnrollmentRejected
	}
	return err
}

func validEnrollmentSecret(secret string) bool {
	if len(secret) != enrollmentKeyHexLen {
		return false
	}
	_, err := hex.DecodeString(secret)
	return err == nil
}

func (s *Service) ListHouseholdDevices(ctx context.Context, householdID string) ([]*domain.DeviceBinding, error) {
	rows, err := s.DB.ListBindingsByHousehold(ctx, householdID)
	if err != nil {
		return nil, err
	}
	deviceIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		deviceIDs = append(deviceIDs, row.DeviceID)
	}
	var states map[string]vnmsclient.DeviceState
	if s.VNMS != nil && len(deviceIDs) > 0 {
		states, _ = s.VNMS.BatchGet(ctx, deviceIDs)
	}
	out := make([]*domain.DeviceBinding, 0, len(rows))
	for _, row := range rows {
		state := states[row.DeviceID]
		s.maybeConfirmDelivery(ctx, row, state)
		if refreshed, err := s.DB.GetBinding(ctx, row.ID); err == nil {
			row = refreshed
		}
		out = append(out, ProjectBinding(row, state, s.OfflineThreshold))
	}
	return out, nil
}

func (s *Service) GetBinding(ctx context.Context, bindingID string) (*domain.DeviceBinding, error) {
	row, err := s.DB.GetBinding(ctx, bindingID)
	if err != nil {
		return nil, err
	}
	var state vnmsclient.DeviceState
	if s.VNMS != nil {
		states, err := s.VNMS.BatchGet(ctx, []string{row.DeviceID})
		if err == nil {
			state = states[row.DeviceID]
		}
	}
	s.maybeConfirmDelivery(ctx, row, state)
	if refreshed, err := s.DB.GetBinding(ctx, bindingID); err == nil {
		row = refreshed
	}
	return ProjectBinding(row, state, s.OfflineThreshold), nil
}

func ProjectBinding(row postgres.DeviceBindingRow, vnms vnmsclient.DeviceState, offlineThreshold time.Duration) *domain.DeviceBinding {
	var windows []domain.MonitoredWindow
	_ = decodeWindows(row.WindowsJSON, &windows)

	sub := row.Subscription
	if sub.Status == "trialing" && row.TrialEndsAt != nil {
		sub.CurrentPeriodEnd = row.TrialEndsAt
	}

	dev := &domain.DeviceBinding{
		ID:                            row.ID,
		HouseholdID:                   row.HouseholdID,
		DeviceID:                      row.DeviceID,
		DisplayName:                   row.DisplayName,
		RoomOrLocationLabel:           row.RoomOrLocationLabel,
		Status:                        "waiting_for_first_contact",
		BatteryStatus:                 "unknown",
		SubscriptionStatus:            sub.Status,
		MonitoredWindows:              windows,
		MonitoredWindowsDeliveryState: row.WindowsDeliveryState,
		MonitoredWindowAlertMode:      row.AlertMode,
		Subscription:                  sub,
		CreatedAt:                     row.CreatedAt,
		UpdatedAt:                     row.UpdatedAt,
	}

	if vnms.DeviceID != "" {
		dev.LastSeenAt = vnms.LastContactAt
		if vnms.LastVoltageMv != nil {
			v := float64(*vnms.LastVoltageMv) / 1000.0
			dev.BatteryVoltageV = &v
			dev.BatteryStatus = batteryStatus(v)
		}
		dev.Status = deriveStatus(vnms.LastContactAt, offlineThreshold)
		if vnms.LifecycleState == "disabled" {
			dev.Status = "service_suspended"
		}
	} else if row.LastContactAt != nil {
		dev.LastSeenAt = row.LastContactAt
		dev.Status = deriveStatus(row.LastContactAt, offlineThreshold)
	}
	return dev
}

func deriveStatus(lastContact *time.Time, offlineThreshold time.Duration) string {
	if lastContact == nil {
		return "waiting_for_first_contact"
	}
	if time.Since(*lastContact) > offlineThreshold {
		return "offline"
	}
	return "online"
}

func batteryStatus(v float64) string {
	switch {
	case v >= 2.9:
		return "ok"
	case v >= 2.7:
		return "low"
	default:
		return "critical"
	}
}

func ParseDeviceID(payload string) string {
	payload = strings.TrimSpace(payload)
	for _, part := range splitPayload(payload) {
		if strings.HasPrefix(part, "device_id=") {
			return strings.TrimPrefix(part, "device_id=")
		}
		if strings.HasPrefix(part, "imei=") {
			return strings.TrimPrefix(part, "imei=")
		}
	}
	if len(payload) >= 8 && len(payload) <= 64 && !strings.Contains(payload, " ") {
		return payload
	}
	return ""
}

func ParseEnrollmentSecret(payload string) string {
	for _, part := range splitPayload(payload) {
		for _, prefix := range []string{"key=", "device_key=", "device_key_hex=", "enrollment_secret="} {
			if strings.HasPrefix(part, prefix) {
				return normalizeEnrollmentSecret(strings.TrimPrefix(part, prefix))
			}
		}
	}
	return ""
}

func NormalizeEnrollmentSecret(secret string) string {
	return normalizeEnrollmentSecret(secret)
}

func normalizeEnrollmentSecret(secret string) string {
	return strings.ReplaceAll(strings.TrimSpace(secret), " ", "")
}

func splitPayload(payload string) []string {
	return strings.FieldsFunc(payload, func(r rune) bool {
		return r == '&' || r == '?' || r == ';' || r == ',' || r == '\n'
	})
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

func strPtr(v string) *string { return &v }

func decodeWindows(raw []byte, out *[]domain.MonitoredWindow) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
