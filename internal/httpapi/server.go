package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/kimoin/vigelo-backend/internal/auth"
	"github.com/kimoin/vigelo-backend/internal/alerts"
	"github.com/kimoin/vigelo-backend/internal/config"
	"github.com/kimoin/vigelo-backend/internal/devices"
	"github.com/kimoin/vigelo-backend/internal/notifications"
	"github.com/kimoin/vigelo-backend/internal/notifications/email"
	"github.com/kimoin/vigelo-backend/internal/notifications/sms"
	"github.com/kimoin/vigelo-backend/internal/store/memory"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsevents"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

type healthChecker interface {
	Ping(ctx context.Context) error
}

type Server struct {
	log     *slog.Logger
	cfg     config.Config
	db      healthChecker
	pg      *postgres.Store
	devices *memory.DeviceStore
	devSvc   *devices.Service
	alertSvc *alerts.Service
	mailer   email.Sender
	cors    map[string]bool
}

func New(log *slog.Logger, cfg config.Config, db *postgres.DB, mailer email.Sender, vnms devices.VNMS, smsSender sms.Sender) *Server {
	allowed := make(map[string]bool, len(cfg.CORSOrigins))
	for _, o := range cfg.CORSOrigins {
		allowed[o] = true
	}
	var hc healthChecker
	var pgStore *postgres.Store
	if db != nil {
		hc = db
		pgStore = postgres.NewStore(db)
	}
	if mailer == nil {
		mailer = &email.LogSender{Log: log}
	}
	var devSvc *devices.Service
	var alertSvc *alerts.Service
	if pgStore != nil {
		devSvc = &devices.Service{
			DB:               pgStore,
			VNMS:             vnms,
			OfflineThreshold: time.Duration(cfg.OfflineHours) * time.Hour,
			TrialDays:        cfg.TrialDays,
		}
		if smsSender == nil {
			smsSender = &sms.LogSender{Log: log}
		}
		notify := &notifications.Dispatcher{
			Log:    log,
			DB:     pgStore,
			SMS:    smsSender,
			Sender: cfg.GatewayAPISender,
		}
		alertSvc = &alerts.Service{
			DB:               pgStore,
			Devices:          devSvc,
			Notify:           notify,
			OfflineThreshold: time.Duration(cfg.OfflineHours) * time.Hour,
		}
	}
	return &Server{
		log:      log,
		cfg:      cfg,
		db:       hc,
		pg:       pgStore,
		devices:  memory.NewDevices(),
		devSvc:   devSvc,
		alertSvc: alertSvc,
		mailer:   mailer,
		cors:     allowed,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	return withCORS(s.cors, mux)
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.HandleFunc("POST /v1/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /v1/auth/refresh", s.auth(s.handleRefresh))
	mux.HandleFunc("POST /v1/auth/logout", s.auth(s.handleLogout))
	mux.HandleFunc("POST /v1/auth/verify-email", s.handleVerifyEmail)
	mux.HandleFunc("POST /v1/auth/password-reset/request", s.handlePasswordResetRequest)
	mux.HandleFunc("POST /v1/auth/password-reset/complete", s.handlePasswordResetComplete)
	mux.HandleFunc("GET /v1/me", s.auth(s.handleMe))
	mux.HandleFunc("PATCH /v1/me", s.auth(s.handlePatchMe))

	mux.HandleFunc("GET /v1/households", s.auth(s.handleListHouseholds))
	mux.HandleFunc("POST /v1/households", s.auth(s.handleCreateHousehold))
	mux.HandleFunc("PATCH /v1/households/{household_id}", s.auth(s.handlePatchHousehold))
	mux.HandleFunc("GET /v1/households/{household_id}/members", s.auth(s.handleListMembers))
	mux.HandleFunc("POST /v1/households/{household_id}/invites", s.auth(s.handleCreateInvite))
	mux.HandleFunc("POST /v1/invites/{token}/accept", s.auth(s.handleAcceptInvite))

	mux.HandleFunc("POST /v1/push-tokens", s.auth(s.handleRegisterPushToken))
	mux.HandleFunc("DELETE /v1/push-tokens/{push_token_id}", s.auth(s.handleDeletePushToken))
	mux.HandleFunc("GET /v1/households/{household_id}/devices", s.auth(s.handleListDevices))
	mux.HandleFunc("POST /v1/households/{household_id}/devices/register", s.auth(s.handleRegisterDevice))
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

func (s *Server) StartBackgroundWorkers(ctx context.Context, vnms *vnmsclient.Client) {
	if s.pg == nil || vnms == nil || s.alertSvc == nil {
		return
	}
	go (&vnmsevents.Consumer{
		Log:    s.log,
		DB:     s.pg,
		VNMS:   vnms,
		Alerts: s.alertSvc,
	}).Run(ctx)
}

func (s *Server) requirePG(w http.ResponseWriter) bool {
	if s.pg == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "database is required", "")
		return false
	}
	return true
}

func writeStoreError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "conflict", "email is already registered", "email")
	case errors.Is(err, auth.ErrInvalidLogin):
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid email or password", "")
	case errors.Is(err, auth.ErrInvalidSession), errors.Is(err, auth.ErrUserDisabled):
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session", "")
	case errors.Is(err, auth.ErrInvalidToken):
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid or expired token", "")
	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "access denied", "")
	case errors.Is(err, auth.ErrInviteNotFound), errors.Is(err, auth.ErrInviteExpired):
		writeError(w, http.StatusNotFound, "not_found", "invite not found or expired", "")
	case errors.Is(err, auth.ErrAlreadyMember):
		writeError(w, http.StatusConflict, "conflict", "already a household member", "")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error", "")
	}
	return true
}
