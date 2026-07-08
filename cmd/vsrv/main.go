package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type server struct {
	log   *slog.Logger
	store *store
}

type store struct {
	mu          sync.Mutex
	users       map[string]*user
	usersByMail map[string]string
	sessions    map[string]string
	households  map[string]*household
	devices     map[string]*deviceBinding
	alerts      map[string]*alert
	pushTokens  map[string]*pushToken
	nextID      int
}

type user struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type household struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Timezone  string    `json:"timezone"`
	OwnerID   string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type monitoredWindow struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type subscription struct {
	Status           string     `json:"status"`
	ServiceStatus    string     `json:"service_status"`
	PlanCode         string     `json:"plan_code"`
	CurrentPeriodEnd *time.Time `json:"current_period_end,omitempty"`
	NextAction       *string    `json:"next_action"`
}

type deviceBinding struct {
	ID                            string            `json:"id"`
	HouseholdID                   string            `json:"household_id"`
	DeviceID                      string            `json:"device_id,omitempty"`
	DisplayName                   string            `json:"display_name"`
	RoomOrLocationLabel           string            `json:"room_or_location_label"`
	Status                        string            `json:"status"`
	LastSeenAt                    *time.Time        `json:"last_seen_at"`
	BatteryVoltageV               *float64          `json:"battery_voltage_v"`
	BatteryStatus                 string            `json:"battery_status"`
	SubscriptionStatus            string            `json:"subscription_status"`
	MonitoredWindows              []monitoredWindow `json:"monitored_windows"`
	MonitoredWindowsDeliveryState string            `json:"monitored_windows_delivery_state"`
	ActiveAlertCount              int               `json:"active_alert_count"`
	Subscription                  subscription      `json:"subscription"`
	CreatedAt                     time.Time         `json:"created_at"`
	UpdatedAt                     time.Time         `json:"updated_at"`
}

type alert struct {
	ID              string     `json:"id"`
	DeviceBindingID string     `json:"device_binding_id"`
	Type            string     `json:"type"`
	Severity        string     `json:"severity"`
	Status          string     `json:"status"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
}

type pushToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"-"`
	Platform  string    `json:"platform"`
	TokenHint string    `json:"token_hint"`
	CreatedAt time.Time `json:"created_at"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field,omitempty"`
	} `json:"error"`
}

func main() {
	addr := env("VSRV_ADDR", "127.0.0.1:8090")
	s := &server{
		log:   slog.New(slog.NewTextHandler(os.Stdout, nil)),
		store: newStore(),
	}
	mux := http.NewServeMux()
	s.routes(mux)

	s.log.Info("starting VSRV", slog.String("addr", addr))
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		s.log.Error("server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

func newStore() *store {
	return &store{
		users:       map[string]*user{},
		usersByMail: map[string]string{},
		sessions:    map[string]string{},
		households:  map[string]*household{},
		devices:     map[string]*deviceBinding{},
		alerts:      map[string]*alert{},
		pushTokens:  map[string]*pushToken{},
	}
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /v1/auth/refresh", s.auth(s.handleRefresh))
	mux.HandleFunc("POST /v1/auth/logout", s.auth(s.handleLogout))
	mux.HandleFunc("GET /v1/me", s.auth(s.handleMe))
	mux.HandleFunc("GET /v1/households", s.auth(s.handleListHouseholds))
	mux.HandleFunc("POST /v1/households", s.auth(s.handleCreateHousehold))
	mux.HandleFunc("POST /v1/push-tokens", s.auth(s.handleRegisterPushToken))
	mux.HandleFunc("DELETE /v1/push-tokens/{push_token_id}", s.auth(s.handleDeletePushToken))
	mux.HandleFunc("GET /v1/households/{household_id}/devices", s.auth(s.handleListDevices))
	mux.HandleFunc("POST /v1/households/{household_id}/device-claims", s.auth(s.handleClaimDevice))
	mux.HandleFunc("GET /v1/devices/{device_binding_id}", s.auth(s.handleGetDevice))
	mux.HandleFunc("PATCH /v1/devices/{device_binding_id}", s.auth(s.handlePatchDevice))
	mux.HandleFunc("POST /v1/devices/{device_binding_id}/remove", s.auth(s.handleRemoveDevice))
	mux.HandleFunc("GET /v1/devices/{device_binding_id}/monitored-windows", s.auth(s.handleGetMonitoredWindows))
	mux.HandleFunc("PUT /v1/devices/{device_binding_id}/monitored-windows", s.auth(s.handlePutMonitoredWindows))
	mux.HandleFunc("GET /v1/devices/{device_binding_id}/activity", s.auth(s.handleActivity))
	mux.HandleFunc("GET /v1/devices/{device_binding_id}/alerts", s.auth(s.handleAlerts))
	mux.HandleFunc("POST /v1/devices/{device_binding_id}/alerts/{alert_id}/ack", s.auth(s.handleAckAlert))
	mux.HandleFunc("GET /v1/devices/{device_binding_id}/subscription", s.auth(s.handleSubscription))
	mux.HandleFunc("POST /v1/devices/{device_binding_id}/subscription/checkout", s.auth(s.handleCheckout))
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email is required", "email")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters", "password")
		return
	}

	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if _, ok := s.store.usersByMail[req.Email]; ok {
		writeError(w, http.StatusConflict, "conflict", "email is already registered", "email")
		return
	}
	now := time.Now().UTC()
	u := &user{
		ID:           s.store.id("user"),
		Email:        req.Email,
		DisplayName:  fallback(req.DisplayName, req.Email),
		PasswordHash: hashPassword(req.Password),
		CreatedAt:    now,
	}
	h := &household{
		ID:        s.store.id("hh"),
		Name:      "Home",
		Timezone:  "Europe/Helsinki",
		OwnerID:   u.ID,
		CreatedAt: now,
	}
	token := randomToken()
	s.store.users[u.ID] = u
	s.store.usersByMail[u.Email] = u.ID
	s.store.households[h.ID] = h
	s.store.sessions[token] = u.ID

	writeJSON(w, http.StatusCreated, map[string]any{
		"access_token": token,
		"user":         publicUser(u),
		"household":    h,
	})
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	userID, ok := s.store.usersByMail[email]
	if !ok || s.store.users[userID].PasswordHash != hashPassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid email or password", "")
		return
	}
	token := randomToken()
	s.store.sessions[token] = userID
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"user":         publicUser(s.store.users[userID]),
	})
}

func (s *server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	token := randomToken()
	s.store.sessions[token] = userID
	writeJSON(w, http.StatusOK, map[string]string{"access_token": token})
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	s.store.mu.Lock()
	delete(s.store.sessions, token)
	s.store.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(s.store.users[userID])})
}

func (s *server) handleListHouseholds(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	var out []*household
	for _, h := range s.store.households {
		if h.OwnerID == userID {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"households": out})
}

func (s *server) handleCreateHousehold(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	var req struct {
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	h := &household{
		ID:        s.store.id("hh"),
		Name:      fallback(req.Name, "Home"),
		Timezone:  fallback(req.Timezone, "Europe/Helsinki"),
		OwnerID:   userID,
		CreatedAt: time.Now().UTC(),
	}
	s.store.households[h.ID] = h
	writeJSON(w, http.StatusCreated, h)
}

func (s *server) handleClaimDevice(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	householdID := r.PathValue("household_id")
	var req struct {
		QRPayload           string `json:"qr_payload"`
		DisplayName         string `json:"display_name"`
		RoomOrLocationLabel string `json:"room_or_location_label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	deviceID := parseDeviceID(req.QRPayload)
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "qr payload does not contain a device id", "qr_payload")
		return
	}

	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if !s.store.ownsHousehold(userID, householdID) {
		writeError(w, http.StatusForbidden, "forbidden", "household access denied", "")
		return
	}
	for _, d := range s.store.devices {
		if d.DeviceID == deviceID && d.Status != "removed" {
			writeError(w, http.StatusConflict, "conflict", "device is already claimed", "qr_payload")
			return
		}
	}
	now := time.Now().UTC()
	voltage := 3.000
	dev := &deviceBinding{
		ID:                            s.store.id("devbind"),
		HouseholdID:                   householdID,
		DeviceID:                      deviceID,
		DisplayName:                   fallback(req.DisplayName, "Vigelo device"),
		RoomOrLocationLabel:           fallback(req.RoomOrLocationLabel, "Home"),
		Status:                        "waiting_for_first_contact",
		LastSeenAt:                    nil,
		BatteryVoltageV:               &voltage,
		BatteryStatus:                 "unknown",
		SubscriptionStatus:            "none",
		MonitoredWindowsDeliveryState: "not_configured",
		Subscription: subscription{
			Status:        "none",
			ServiceStatus: "service_limited",
			PlanCode:      "device_monitoring_monthly",
			NextAction:    strPtr("activate_service"),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.store.devices[dev.ID] = dev
	writeJSON(w, http.StatusCreated, dev)
}

func (s *server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	householdID := r.PathValue("household_id")
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if !s.store.ownsHousehold(userID, householdID) {
		writeError(w, http.StatusForbidden, "forbidden", "household access denied", "")
		return
	}
	var out []*deviceBinding
	for _, d := range s.store.devices {
		if d.HouseholdID == householdID && d.Status != "removed" {
			s.store.refreshDeviceDerivedState(d)
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func (s *server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dev)
}

func (s *server) handlePatchDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName         *string `json:"display_name"`
		RoomOrLocationLabel *string `json:"room_or_location_label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	if req.DisplayName != nil && strings.TrimSpace(*req.DisplayName) != "" {
		dev.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.RoomOrLocationLabel != nil && strings.TrimSpace(*req.RoomOrLocationLabel) != "" {
		dev.RoomOrLocationLabel = strings.TrimSpace(*req.RoomOrLocationLabel)
	}
	dev.UpdatedAt = time.Now().UTC()
	writeJSON(w, http.StatusOK, dev)
}

func (s *server) handleRemoveDevice(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	dev.Status = "removed"
	dev.UpdatedAt = time.Now().UTC()
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *server) handleGetMonitoredWindows(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"windows":        dev.MonitoredWindows,
		"delivery_state": dev.MonitoredWindowsDeliveryState,
	})
}

func (s *server) handlePutMonitoredWindows(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Windows []monitoredWindow `json:"windows"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateWindows(req.Windows); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), "windows")
		return
	}
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	dev.MonitoredWindows = req.Windows
	dev.MonitoredWindowsDeliveryState = "pending_delivery"
	dev.UpdatedAt = time.Now().UTC()
	writeJSON(w, http.StatusOK, map[string]any{
		"windows":        dev.MonitoredWindows,
		"delivery_state": dev.MonitoredWindowsDeliveryState,
	})
}

func (s *server) handleActivity(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
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
		"timezone": "Europe/Helsinki",
		"days":     days,
	})
}

func (s *server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	var out []*alert
	for _, a := range s.store.alerts {
		if a.DeviceBindingID == dev.ID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FirstSeenAt.After(out[j].FirstSeenAt) })
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out})
}

func (s *server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	alertID := r.PathValue("alert_id")
	a, ok := s.store.alerts[alertID]
	if !ok || a.DeviceBindingID != dev.ID {
		writeError(w, http.StatusNotFound, "not_found", "alert not found", "")
		return
	}
	now := time.Now().UTC()
	a.Status = "acknowledged"
	a.AcknowledgedAt = &now
	dev.ActiveAlertCount = s.store.activeAlertCount(dev.ID)
	writeJSON(w, http.StatusOK, a)
}

func (s *server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dev.Subscription)
}

func (s *server) handleCheckout(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authorizedDevice(w, r)
	if !ok {
		return
	}
	end := time.Now().UTC().AddDate(0, 1, 0)
	dev.Subscription = subscription{
		Status:           "active",
		ServiceStatus:    "service_active",
		PlanCode:         "device_monitoring_monthly",
		CurrentPeriodEnd: &end,
		NextAction:       nil,
	}
	dev.SubscriptionStatus = "active"
	dev.Status = "online"
	now := time.Now().UTC()
	dev.LastSeenAt = &now
	dev.BatteryStatus = "ok"
	dev.UpdatedAt = now
	s.store.ensureDemoAlert(dev)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "active",
		"checkout_url": "dev://checkout/succeeded",
		"subscription": dev.Subscription,
	})
}

func (s *server) handleRegisterPushToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	var req struct {
		Platform string `json:"platform"`
		Token    string `json:"token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Platform == "" || req.Token == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "platform and token are required", "")
		return
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	pt := &pushToken{
		ID:        s.store.id("push"),
		UserID:    userID,
		Platform:  req.Platform,
		TokenHint: tokenHint(req.Token),
		CreatedAt: time.Now().UTC(),
	}
	s.store.pushTokens[pt.ID] = pt
	writeJSON(w, http.StatusCreated, pt)
}

func (s *server) handleDeletePushToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextUserID{}).(string)
	id := r.PathValue("push_token_id")
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if pt, ok := s.store.pushTokens[id]; ok && pt.UserID == userID {
		delete(s.store.pushTokens, id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *server) authorizedDevice(w http.ResponseWriter, r *http.Request) (*deviceBinding, bool) {
	userID := r.Context().Value(contextUserID{}).(string)
	deviceID := r.PathValue("device_binding_id")
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	dev, ok := s.store.devices[deviceID]
	if !ok || dev.Status == "removed" {
		writeError(w, http.StatusNotFound, "not_found", "device not found", "")
		return nil, false
	}
	if !s.store.ownsHousehold(userID, dev.HouseholdID) {
		writeError(w, http.StatusForbidden, "forbidden", "device access denied", "")
		return nil, false
	}
	s.store.refreshDeviceDerivedState(dev)
	return dev, true
}

func (s *store) refreshDeviceDerivedState(dev *deviceBinding) {
	dev.ActiveAlertCount = s.activeAlertCount(dev.ID)
	if dev.Subscription.Status == "active" && dev.LastSeenAt != nil {
		dev.Status = "online"
	}
}

func (s *store) ensureDemoAlert(dev *deviceBinding) {
	if s.activeAlertCount(dev.ID) > 0 {
		return
	}
	now := time.Now().UTC()
	a := &alert{
		ID:              s.id("alert"),
		DeviceBindingID: dev.ID,
		Type:            "movement_detected",
		Severity:        "info",
		Status:          "active",
		Title:           "Movement detected",
		Body:            fmt.Sprintf("Movement detected in %s.", dev.RoomOrLocationLabel),
		FirstSeenAt:     now.Add(-30 * time.Minute),
		LastSeenAt:      now.Add(-30 * time.Minute),
	}
	s.alerts[a.ID] = a
	dev.ActiveAlertCount = s.activeAlertCount(dev.ID)
}

func (s *store) activeAlertCount(deviceBindingID string) int {
	n := 0
	for _, a := range s.alerts {
		if a.DeviceBindingID == deviceBindingID && a.Status == "active" {
			n++
		}
	}
	return n
}

func (s *store) ownsHousehold(userID, householdID string) bool {
	h, ok := s.households[householdID]
	return ok && h.OwnerID == userID
}

func (s *store) id(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s_%06d", prefix, s.nextID)
}

type contextUserID struct{}

func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token", "")
			return
		}
		s.store.mu.Lock()
		userID, ok := s.store.sessions[token]
		s.store.mu.Unlock()
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid bearer token", "")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), contextUserID{}, userID)))
	}
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body", "")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	var resp errorResponse
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.Field = field
	writeJSON(w, status, resp)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", env("VSRV_CORS_ORIGIN", "http://127.0.0.1:5173"))
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validateWindows(windows []monitoredWindow) error {
	if len(windows) > 2 {
		return errors.New("you can add up to two monitored windows")
	}
	seen := make([]bool, 24)
	for _, w := range windows {
		start, err := parseHour(w.StartTime)
		if err != nil {
			return err
		}
		end, err := parseHour(w.EndTime)
		if err != nil {
			return err
		}
		duration := (end - start + 24) % 24
		if duration == 0 {
			duration = 24
		}
		if duration == 24 && len(windows) > 1 {
			return errors.New("a full-day window cannot be combined with another window")
		}
		for i := 0; i < duration; i++ {
			h := (start + i) % 24
			if seen[h] {
				return errors.New("monitored windows cannot overlap")
			}
			seen[h] = true
		}
	}
	return nil
}

func hourInWindows(hour int, windows []monitoredWindow) bool {
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

func parseDeviceID(payload string) string {
	payload = strings.TrimSpace(payload)
	for _, part := range strings.FieldsFunc(payload, func(r rune) bool {
		return r == '&' || r == '?' || r == ';' || r == ',' || r == '\n'
	}) {
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

func publicUser(u *user) map[string]any {
	return map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"created_at":   u.CreatedAt,
	}
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte("vsrv-dev-password:" + password))
	return hex.EncodeToString(sum[:])
}

func randomToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func tokenHint(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func fallback(v, fb string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fb
	}
	return v
}

func strPtr(v string) *string { return &v }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
