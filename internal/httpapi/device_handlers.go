package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kimoin/vigelo-backend/internal/audit"
	"github.com/kimoin/vigelo-backend/internal/authz"
	"github.com/kimoin/vigelo-backend/internal/devices"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
)

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	s.registerDevice(w, r, false)
}

func (s *Server) handleClaimDevice(w http.ResponseWriter, r *http.Request) {
	s.registerDevice(w, r, true)
}

func (s *Server) registerDevice(w http.ResponseWriter, r *http.Request, fromQR bool) {
	if !s.requirePG(w) || s.devSvc == nil {
		if s.devSvc == nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "device enrollment is not configured", "")
		}
		return
	}
	userID := userIDFromContext(r.Context())
	householdID := r.PathValue("household_id")
	role, ok := s.householdRole(w, r, householdID)
	if !ok {
		return
	}
	if !authz.CanClaimDevices(role) {
		writeError(w, http.StatusForbidden, "forbidden", "device claim not allowed for your role", "")
		return
	}

	var req struct {
		QRPayload           string `json:"qr_payload"`
		DeviceID            string `json:"device_id"`
		EnrollmentSecret    string `json:"enrollment_secret"`
		DisplayName         string `json:"display_name"`
		RoomOrLocationLabel string `json:"room_or_location_label"`
		Timezone            string `json:"timezone"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	secret := devices.NormalizeEnrollmentSecret(req.EnrollmentSecret)
	if fromQR {
		if deviceID == "" {
			deviceID = devices.ParseDeviceID(req.QRPayload)
		}
		if secret == "" {
			secret = devices.ParseEnrollmentSecret(req.QRPayload)
		}
	}
	if deviceID == "" || secret == "" {
		field := "enrollment_secret"
		if fromQR {
			field = "qr_payload"
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "device id and enrollment secret are required", field)
		return
	}

	hh, err := s.pg.GetHouseholdTimezone(r.Context(), householdID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	tz := req.Timezone
	if tz == "" {
		tz = hh
	}

	dev, err := s.devSvc.Register(r.Context(), devices.RegisterInput{
		HouseholdID:         householdID,
		UserID:              userID,
		DeviceID:            deviceID,
		EnrollmentSecret:    secret,
		DisplayName:         req.DisplayName,
		RoomOrLocationLabel: req.RoomOrLocationLabel,
		Timezone:            tz,
	})
	if writeDeviceError(w, err) {
		return
	}
	userEmail, _ := s.pg.GetUserEmail(r.Context(), userID)
	hhName, _ := s.pg.GetHouseholdName(r.Context(), householdID)
	s.recordAudit(r, audit.Entry{
		ActorUserID: userID, Action: "device.provisioned",
		ResourceType: "device_binding", ResourceID: dev.ID,
		Message: fmt.Sprintf("%s provisioned device %s to household %q", userEmail, dev.DeviceID, hhName),
		Metadata: map[string]any{"device_id": dev.DeviceID, "household_id": householdID},
	})
	writeJSON(w, http.StatusCreated, dev)
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) || s.devSvc == nil {
		return
	}
	householdID := r.PathValue("household_id")
	role, ok := s.householdRole(w, r, householdID)
	if !ok || !authz.CanViewDevices(role) {
		if ok {
			writeError(w, http.StatusForbidden, "forbidden", "device access denied", "")
		}
		return
	}
	out, err := s.devSvc.ListHouseholdDevices(r.Context(), householdID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.attachAlertCounts(r.Context(), out)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dev)
}

func (s *Server) handlePatchDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName         *string `json:"display_name"`
		RoomOrLocationLabel *string `json:"room_or_location_label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	dev, role, ok := s.authorizedDeviceWithRole(w, r)
	if !ok {
		return
	}
	if !authz.CanConfigureDevices(role) {
		writeError(w, http.StatusForbidden, "forbidden", "device configuration not allowed", "")
		return
	}
	displayName := dev.DisplayName
	room := dev.RoomOrLocationLabel
	if req.DisplayName != nil && strings.TrimSpace(*req.DisplayName) != "" {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.RoomOrLocationLabel != nil && strings.TrimSpace(*req.RoomOrLocationLabel) != "" {
		room = strings.TrimSpace(*req.RoomOrLocationLabel)
	}
	if err := s.pg.UpdateBindingMeta(r.Context(), dev.ID, displayName, room); err != nil {
		writeDeviceError(w, err)
		return
	}
	updated, err := s.devSvc.GetBinding(r.Context(), dev.ID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	s.attachAlertCounts(r.Context(), []*domain.DeviceBinding{updated})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleRemoveDevice(w http.ResponseWriter, r *http.Request) {
	dev, role, ok := s.authorizedDeviceWithRole(w, r)
	if !ok {
		return
	}
	if !authz.CanClaimDevices(role) {
		writeError(w, http.StatusForbidden, "forbidden", "device removal not allowed", "")
		return
	}
	if err := s.pg.RemoveBinding(r.Context(), dev.ID); err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) handleGetMonitoredWindows(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	view, err := s.devSvc.GetMonitoredWindows(r.Context(), dev.ID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handlePutMonitoredWindows(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Timezone  string                   `json:"timezone"`
		Windows   []domain.MonitoredWindow `json:"windows"`
		AlertMode string                   `json:"alert_mode"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	dev, role, ok := s.authorizedDeviceWithRole(w, r)
	if !ok {
		return
	}
	if !authz.CanConfigureDevices(role) {
		writeError(w, http.StatusForbidden, "forbidden", "monitored windows not allowed", "")
		return
	}
	view, err := s.devSvc.SetMonitoredWindows(r.Context(), devices.MonitoredWindowsInput{
		BindingID: dev.ID,
		UserID:    userIDFromContext(r.Context()),
		Timezone:  req.Timezone,
		Windows:   req.Windows,
		AlertMode: req.AlertMode,
	})
	if writeDeviceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	tz := "Europe/Helsinki"
	if view, err := s.devSvc.GetMonitoredWindows(r.Context(), dev.ID); err == nil && view.Timezone != "" {
		tz = view.Timezone
	}
	now := time.Now().UTC()
	days := make([]map[string]any, 0, 7)
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		hours := make([]map[string]any, 0, 24)
		for h := 0; h < 24; h++ {
			monitored := hourInWindows(h, dev.MonitoredWindows)
			movement := monitored && (h == 8 || h == 12 || h == 18)
			event := ""
			if monitored && h == 18 {
				event = "movement_detected"
			}
			hours = append(hours, map[string]any{
				"start":     fmt.Sprintf("%02d:00", h),
				"end":       fmt.Sprintf("%02d:00", h+1),
				"movement":  movement,
				"monitored": monitored,
				"event":     event,
			})
		}
		days = append(days, map[string]any{
			"date":  day.Format("2006-01-02"),
			"hours": hours,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"timezone": tz,
		"days":     days,
	})
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	out, err := s.pg.ListAlerts(r.Context(), dev.ID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FirstSeenAt.After(out[j].FirstSeenAt) })
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out})
}

func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	alertID := r.PathValue("alert_id")
	a, err := s.pg.AckAlert(r.Context(), dev.ID, alertID, userIDFromContext(r.Context()))
	if errors.Is(err, postgres.ErrAlertNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "alert not found", "")
		return
	}
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dev.Subscription)
}

func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	if err := s.pg.ActivateSubscription(r.Context(), dev.ID); err != nil {
		writeDeviceError(w, err)
		return
	}
	updated, err := s.devSvc.GetBinding(r.Context(), dev.ID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	s.attachAlertCounts(r.Context(), []*domain.DeviceBinding{updated})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "active",
		"checkout_url": "dev://checkout/succeeded",
		"subscription": updated.Subscription,
	})
}

func (s *Server) handleRegisterPushToken(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
	userID := userIDFromContext(r.Context())
	var req struct {
		Platform    string `json:"platform"`
		Token       string `json:"token"`
		Environment string `json:"environment"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validPushPlatform(req.Platform) || req.Token == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "platform and token are required", "token")
		return
	}
	sessionID := ""
	if token := accessTokenFromContext(r.Context()); token != "" {
		if _, sid, err := s.pg.ResolveAccessToken(r.Context(), token); err == nil {
			sessionID = sid
		}
	}
	pt, err := s.pg.UpsertPushToken(r.Context(), userID, sessionID, req.Platform, req.Token, req.Environment)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not register push token", "")
		return
	}
	writeJSON(w, http.StatusCreated, pt)
}

func (s *Server) handleDeletePushToken(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
	userID := userIDFromContext(r.Context())
	id := r.PathValue("push_token_id")
	_ = s.pg.DeletePushToken(r.Context(), userID, id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func validPushPlatform(platform string) bool {
	switch platform {
	case "ntfy", "web", "ios", "android", "unifiedpush":
		return true
	default:
		return false
	}
}

func (s *Server) authorizedDevice(w http.ResponseWriter, r *http.Request) (*domain.DeviceBinding, bool) {
	dev, _, ok := s.authorizedDeviceWithRole(w, r)
	return dev, ok
}

func (s *Server) authorizedDeviceWithRole(w http.ResponseWriter, r *http.Request) (*domain.DeviceBinding, authz.Role, bool) {
	if !s.requirePG(w) || s.devSvc == nil {
		return nil, "", false
	}
	deviceID := r.PathValue("device_binding_id")
	dev, err := s.devSvc.GetBinding(r.Context(), deviceID)
	if err != nil {
		writeDeviceError(w, err)
		return nil, "", false
	}
	role, ok := s.householdRole(w, r, dev.HouseholdID)
	if !ok {
		return nil, "", false
	}
	if !authz.CanViewDevices(role) {
		writeError(w, http.StatusForbidden, "forbidden", "device access denied", "")
		return nil, "", false
	}
	s.attachAlertCounts(r.Context(), []*domain.DeviceBinding{dev})
	return dev, role, true
}

func (s *Server) attachAlertCounts(ctx context.Context, devs []*domain.DeviceBinding) {
	for _, d := range devs {
		n, err := s.pg.CountActiveAlerts(ctx, d.ID)
		if err == nil {
			d.ActiveAlertCount = n
		}
	}
}

func writeDeviceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, postgres.ErrDeviceAlreadyBound):
		writeError(w, http.StatusConflict, "conflict", "device is already claimed", "device_id")
	case errors.Is(err, postgres.ErrBindingNotFound):
		writeError(w, http.StatusNotFound, "not_found", "device not found", "")
	case errors.Is(err, devices.ErrVNMSNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "device enrollment is not configured", "")
	case errors.Is(err, devices.ErrInvalidEnrollment):
		writeError(w, http.StatusBadRequest, "invalid_request", "device id and enrollment secret are required", "enrollment_secret")
	case errors.Is(err, devices.ErrEnrollmentRejected):
		writeError(w, http.StatusForbidden, "forbidden", "enrollment could not be verified", "enrollment_secret")
	case errors.Is(err, devices.ErrDeviceAlreadyActive):
		writeError(w, http.StatusConflict, "conflict", "device is already active on the network", "device_id")
	case errors.Is(err, devices.ErrInvalidMonitoredWindows):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), "windows")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error", "")
	}
	return true
}

func tokenHint(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func hourInWindows(hour int, windows []domain.MonitoredWindow) bool {
	for _, w := range windows {
		start, err1 := parseHour(w.StartTime)
		end, err2 := parseHour(w.EndTime)
		if err1 != nil || err2 != nil {
			continue
		}
		duration := (end - start + 24) % 24
		if duration == 0 {
			duration = 24
		}
		for i := 0; i < duration; i++ {
			if (start+i)%24 == hour {
				return true
			}
		}
	}
	return false
}

func parseHour(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 || parts[1] != "00" {
		return 0, errors.New("use whole-hour 24-hour times like 08:00")
	}
	var h int
	if _, err := fmt.Sscanf(parts[0], "%02d", &h); err != nil || h < 0 || h > 24 {
		return 0, errors.New("use whole-hour 24-hour times like 08:00")
	}
	if h == 24 {
		h = 0
	}
	return h, nil
}
