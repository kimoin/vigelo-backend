package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kimoin/vigelo-backend/internal/audit"
	"github.com/kimoin/vigelo-backend/internal/auth"
	"github.com/kimoin/vigelo-backend/internal/devices"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
)

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/admin/me", s.auth(s.handleAdminMe))
	mux.HandleFunc("GET /v1/admin/status", s.admin(s.handleAdminStatus))
	mux.HandleFunc("GET /v1/admin/dashboard", s.admin(s.handleAdminDashboard))
	mux.HandleFunc("GET /v1/admin/audit-log", s.admin(s.handleAdminAuditLog))

	mux.HandleFunc("GET /v1/admin/users", s.admin(s.handleAdminListUsers))
	mux.HandleFunc("POST /v1/admin/users", s.admin(s.handleAdminCreateUser))
	mux.HandleFunc("GET /v1/admin/users/{user_id}", s.admin(s.handleAdminGetUser))
	mux.HandleFunc("DELETE /v1/admin/users/{user_id}", s.admin(s.handleAdminDeleteUser))
	mux.HandleFunc("POST /v1/admin/users/{user_id}/disable", s.admin(s.handleAdminDisableUser))
	mux.HandleFunc("POST /v1/admin/users/{user_id}/enable", s.admin(s.handleAdminEnableUser))

	mux.HandleFunc("GET /v1/admin/households", s.admin(s.handleAdminListHouseholds))
	mux.HandleFunc("POST /v1/admin/households", s.admin(s.handleAdminCreateHousehold))
	mux.HandleFunc("DELETE /v1/admin/households/{household_id}", s.admin(s.handleAdminDeleteHousehold))
	mux.HandleFunc("POST /v1/admin/households/{household_id}/members", s.admin(s.handleAdminAddHouseholdMember))
	mux.HandleFunc("DELETE /v1/admin/households/{household_id}/members/{user_id}", s.admin(s.handleAdminRemoveHouseholdMember))
	mux.HandleFunc("POST /v1/admin/households/{household_id}/devices", s.admin(s.handleAdminProvisionDevice))

	mux.HandleFunc("GET /v1/admin/devices", s.admin(s.handleAdminListDevices))
	mux.HandleFunc("POST /v1/admin/devices/{device_binding_id}/enable", s.admin(s.handleAdminEnableDevice))
	mux.HandleFunc("POST /v1/admin/devices/{device_binding_id}/disable", s.admin(s.handleAdminDisableDevice))
	mux.HandleFunc("POST /v1/admin/devices/{device_binding_id}/unprovision", s.admin(s.handleAdminUnprovisionDevice))
	mux.HandleFunc("DELETE /v1/admin/devices/{device_binding_id}", s.admin(s.handleAdminDeleteDevice))
	mux.HandleFunc("POST /v1/admin/devices/{device_binding_id}/move", s.admin(s.handleAdminMoveDevice))
	mux.HandleFunc("POST /v1/admin/devices/{device_binding_id}/extend-trial", s.admin(s.handleAdminExtendTrial))
	mux.HandleFunc("POST /v1/admin/devices/{device_binding_id}/activate-subscription", s.admin(s.handleAdminActivateSubscription))
	mux.HandleFunc("GET /v1/admin/devices/{device_binding_id}/monitored-windows", s.admin(s.handleAdminGetMonitoredWindows))
	mux.HandleFunc("PUT /v1/admin/devices/{device_binding_id}/monitored-windows", s.admin(s.handleAdminPutMonitoredWindows))
}

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireAdmin(w, r) {
			return
		}
		next(w, r)
	})
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	user, err := s.pg.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if writeStoreError(w, err) {
		return false
	}
	if !s.cfg.IsAdminEmail(user.Email) {
		writeError(w, http.StatusForbidden, "forbidden", "admin access required", "")
		return false
	}
	return true
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.pg.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"admin": s.cfg.IsAdminEmail(user.Email),
		"user":  publicUser(user),
	})
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if s.statusChecker == nil {
		writeJSON(w, http.StatusOK, map[string]any{"services": []any{}, "host": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"services": s.statusChecker.All(r.Context()),
		"host":     s.statusChecker.HostMetrics(),
	})
}

func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	counts, err := s.pg.AdminDashboardCounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "dashboard failed", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts})
}

func (s *Server) handleAdminAuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, total, err := s.pg.ListAuditLog(r.Context(), q, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "audit log failed", "")
		return
	}
	entries := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var meta map[string]any
		_ = json.Unmarshal(row.MetadataJSON, &meta)
		entries = append(entries, map[string]any{
			"id":            row.ID,
			"actor_user_id": row.ActorUserID,
			"action":        row.Action,
			"resource_type": row.ResourceType,
			"resource_id":   row.ResourceID,
			"message":       row.Message,
			"metadata":      meta,
			"created_at":    row.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": total})
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, total, err := s.pg.AdminSearchUsers(r.Context(), q, r.URL.Query().Get("filter"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "list users failed", "")
		return
	}
	actorID := userIDFromContext(r.Context())
	for i := range rows {
		s.enrichAdminUserMeta(&rows[i], actorID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": rows, "total": total})
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Timezone    string `json:"timezone"`
		AccountType string `json:"account_type"`
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
	accountType := strings.ToLower(strings.TrimSpace(req.AccountType))
	if accountType == "" {
		accountType = "user"
	}
	if accountType != "user" && accountType != "admin" {
		writeError(w, http.StatusBadRequest, "invalid_request", "account_type must be user or admin", "account_type")
		return
	}

	res, err := s.pg.AdminCreateUser(r.Context(), req.Email, req.Password, req.DisplayName, req.Timezone)
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "conflict", "email is already registered", "email")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "create user failed", "")
		return
	}

	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "user.created",
		ResourceType: "user", ResourceID: res.User.ID,
		Message: fmt.Sprintf("admin %s created %s account for %s", adminEmail, accountType, req.Email),
		Metadata: map[string]any{"account_type": accountType, "email_verified": true},
	})

	resp := map[string]any{
		"user":      publicUser(res.User),
		"household": res.Household,
		"account_type": accountType,
		"email_verified": true,
	}
	if accountType == "admin" {
		resp["console_admin_note"] = "Add this email to VSRV_ADMIN_EMAILS and redeploy for VSRV admin console access."
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleAdminGetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("user_id")
	detail, err := s.pg.AdminGetUser(r.Context(), id)
	if err != nil {
		if err == postgres.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "not_found", "user not found", "")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "get user failed", "")
		return
	}
	s.enrichAdminUserDetail(&detail, userIDFromContext(r.Context()))
	s.enrichAdminUserDetailVNMS(r.Context(), &detail)
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("user_id")
	if id == userIDFromContext(r.Context()) {
		writeError(w, http.StatusConflict, "conflict", "cannot delete your own account", "")
		return
	}
	targetEmail, err := s.pg.GetUserEmail(r.Context(), id)
	if err != nil {
		if err == postgres.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "not_found", "user not found", "")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "lookup user failed", "")
		return
	}
	if s.cfg.IsAdminEmail(targetEmail) {
		writeError(w, http.StatusConflict, "conflict", "users listed in VSRV_ADMIN_EMAILS cannot be deleted", "")
		return
	}
	if err := s.pg.AdminDeleteUser(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, postgres.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "not_found", "user not found", "")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "delete user failed", "")
		}
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "user.deleted",
		ResourceType: "user", ResourceID: id,
		Message: fmt.Sprintf("admin %s deleted user %s", adminEmail, targetEmail),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) enrichAdminUserMeta(row *postgres.AdminUserRow, actorID string) {
	row.ConsoleAdmin = s.cfg.IsAdminEmail(row.Email)
	row.Deletable = row.ID != actorID && !row.ConsoleAdmin
}

func (s *Server) enrichAdminUserDetail(d *postgres.AdminUserDetail, actorID string) {
	d.ConsoleAdmin = s.cfg.IsAdminEmail(d.Email)
	d.Deletable = d.ID != actorID && !d.ConsoleAdmin
}

func (s *Server) handleAdminDisableUser(w http.ResponseWriter, r *http.Request) {
	s.adminSetUserDisabled(w, r, true)
}

func (s *Server) handleAdminEnableUser(w http.ResponseWriter, r *http.Request) {
	s.adminSetUserDisabled(w, r, false)
}

func (s *Server) adminSetUserDisabled(w http.ResponseWriter, r *http.Request, disabled bool) {
	id := r.PathValue("user_id")
	targetEmail, err := s.pg.GetUserEmail(r.Context(), id)
	if err != nil {
		if err == postgres.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "not_found", "user not found", "")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "lookup user failed", "")
		return
	}
	if err := s.pg.AdminSetUserDisabled(r.Context(), id, disabled); err != nil {
		if err == postgres.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "not_found", "user not found", "")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "update user failed", "")
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	action := "user.enabled"
	msg := fmt.Sprintf("admin %s enabled user %s", adminEmail, targetEmail)
	if disabled {
		action = "user.disabled"
		msg = fmt.Sprintf("admin %s disabled user %s", adminEmail, targetEmail)
	}
	s.recordAudit(r, audit.Entry{ActorUserID: userIDFromContext(r.Context()), Action: action, ResourceType: "user", ResourceID: id, Message: msg})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminListHouseholds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, total, err := s.pg.AdminListHouseholds(r.Context(), q, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "list households failed", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"households": rows, "total": total})
}

func (s *Server) handleAdminCreateHousehold(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		OwnerUserID string `json:"owner_user_id"`
		Timezone    string `json:"timezone"`
	}
	if !decodeJSON(w, r, &req) || req.Name == "" || req.OwnerUserID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name and owner_user_id are required", "")
		return
	}
	hh, err := s.pg.AdminCreateHousehold(r.Context(), req.Name, req.OwnerUserID, req.Timezone)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "create household failed", "")
		return
	}
	owner, _ := s.pg.GetUserEmail(r.Context(), req.OwnerUserID)
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "household.created",
		ResourceType: "household", ResourceID: hh.ID,
		Message: fmt.Sprintf("admin %s created household %q for %s", adminEmail, hh.Name, owner),
	})
	writeJSON(w, http.StatusCreated, hh)
}

func (s *Server) handleAdminDeleteHousehold(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("household_id")
	if err := s.pg.AdminDeleteHousehold(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "delete household failed", "")
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "household.deleted",
		ResourceType: "household", ResourceID: id,
		Message: fmt.Sprintf("admin %s deleted household %s", adminEmail, id),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAdminAddHouseholdMember(w http.ResponseWriter, r *http.Request) {
	householdID := r.PathValue("household_id")
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
		Invite bool  `json:"invite_if_missing"`
	}
	if !decodeJSON(w, r, &req) || strings.TrimSpace(req.Email) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email is required", "email")
		return
	}
	adminID := userIDFromContext(r.Context())
	if req.Invite {
		res, err := s.pg.AdminInviteHouseholdMember(r.Context(), householdID, adminID, req.Email, req.Role, 7*24*time.Hour)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "invite failed", "")
			return
		}
		adminEmail, _ := s.pg.GetUserEmail(r.Context(), adminID)
		s.recordAudit(r, audit.Entry{
			ActorUserID: adminID, Action: "household.member_invited",
			ResourceType: "household", ResourceID: householdID,
			Message: fmt.Sprintf("admin %s invited %s as %s", adminEmail, req.Email, res.Invite.Role),
		})
		writeJSON(w, http.StatusCreated, map[string]any{"status": "invited", "invite_token": res.TokenRaw, "invite": res.Invite})
		return
	}
	if err := s.pg.AdminAddHouseholdMember(r.Context(), householdID, req.Email, req.Role); err != nil {
		switch err {
		case postgres.ErrUserNotFound:
			writeError(w, http.StatusNotFound, "not_found", "user not found — enable invite_if_missing to send invite", "email")
		case postgres.ErrAlreadyMember:
			writeError(w, http.StatusConflict, "conflict", "user is already a member", "email")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "add member failed", "")
		}
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), adminID)
	s.recordAudit(r, audit.Entry{
		ActorUserID: adminID, Action: "household.member_added",
		ResourceType: "household", ResourceID: householdID,
		Message: fmt.Sprintf("admin %s added %s as %s", adminEmail, req.Email, req.Role),
	})
	writeJSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

func (s *Server) handleAdminRemoveHouseholdMember(w http.ResponseWriter, r *http.Request) {
	householdID := r.PathValue("household_id")
	memberID := r.PathValue("user_id")
	memberEmail, _ := s.pg.GetUserEmail(r.Context(), memberID)
	if err := s.pg.AdminRemoveHouseholdMember(r.Context(), householdID, memberID); err != nil {
		switch err {
		case postgres.ErrMemberNotFound:
			writeError(w, http.StatusNotFound, "not_found", "member not found", "")
		case postgres.ErrCannotRemoveOwner:
			writeError(w, http.StatusConflict, "conflict", "cannot remove sole household owner", "")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "remove member failed", "")
		}
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "household.member_removed",
		ResourceType: "household", ResourceID: householdID,
		Message: fmt.Sprintf("admin %s removed %s from household", adminEmail, memberEmail),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) handleAdminProvisionDevice(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) || s.devSvc == nil {
		if s.devSvc == nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "device enrollment is not configured", "")
		}
		return
	}
	householdID := r.PathValue("household_id")
	var req struct {
		UserID              string `json:"user_id"`
		DeviceID            string `json:"device_id"`
		EnrollmentSecret    string `json:"enrollment_secret"`
		DisplayName         string `json:"display_name"`
		RoomOrLocationLabel string `json:"room_or_location_label"`
	}
	if !decodeJSON(w, r, &req) || strings.TrimSpace(req.DeviceID) == "" || strings.TrimSpace(req.EnrollmentSecret) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "device_id and enrollment_secret are required", "")
		return
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		var err error
		userID, err = s.pg.GetHouseholdOwnerID(r.Context(), householdID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "household has no owner — provide user_id", "user_id")
			return
		}
	}
	hh, err := s.pg.AdminGetHouseholdDetail(r.Context(), householdID, "")
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "household not found", "")
		return
	}
	dev, err := s.devSvc.Register(r.Context(), devices.RegisterInput{
		HouseholdID:         householdID,
		UserID:              userID,
		DeviceID:            req.DeviceID,
		EnrollmentSecret:    req.EnrollmentSecret,
		DisplayName:         req.DisplayName,
		RoomOrLocationLabel: req.RoomOrLocationLabel,
		Timezone:            hh.Timezone,
	})
	if writeDeviceError(w, err) {
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "device.provisioned",
		ResourceType: "device_binding", ResourceID: dev.ID,
		Message: fmt.Sprintf("admin %s provisioned device %s (%s) to household %s", adminEmail, dev.DeviceID, dev.DisplayName, householdID),
		Metadata: map[string]any{"device_id": dev.DeviceID, "household_id": householdID},
	})
	writeJSON(w, http.StatusCreated, dev)
}

func (s *Server) handleAdminListDevices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, total, err := s.pg.AdminListDevices(r.Context(), q, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "list devices failed", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": rows, "total": total})
}

func (s *Server) handleAdminEnableDevice(w http.ResponseWriter, r *http.Request) {
	s.adminVNMSDevice(w, r, "enable")
}

func (s *Server) handleAdminDisableDevice(w http.ResponseWriter, r *http.Request) {
	s.adminVNMSDevice(w, r, "disable")
}

func (s *Server) handleAdminUnprovisionDevice(w http.ResponseWriter, r *http.Request) {
	s.adminVNMSDevice(w, r, "unprovision")
}

func (s *Server) adminVNMSDevice(w http.ResponseWriter, r *http.Request, op string) {
	bindingID := r.PathValue("device_binding_id")
	row, err := s.pg.GetBinding(r.Context(), bindingID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if s.vnms == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "vnms not configured", "")
		return
	}
	var vnmsErr error
	switch op {
	case "enable":
		vnmsErr = s.vnms.Enable(r.Context(), row.DeviceID)
	case "disable":
		vnmsErr = s.vnms.Disable(r.Context(), row.DeviceID)
	case "unprovision":
		vnmsErr = s.vnms.Unprovision(r.Context(), row.DeviceID)
	}
	if vnmsErr != nil {
		writeError(w, http.StatusBadGateway, "internal_error", vnmsErr.Error(), "")
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "device." + op,
		ResourceType: "device_binding", ResourceID: bindingID,
		Message: fmt.Sprintf("admin %s %sd device %s (%s)", adminEmail, op, row.DeviceID, row.DisplayName),
		Metadata: map[string]any{"device_id": row.DeviceID},
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminDeleteDevice(w http.ResponseWriter, r *http.Request) {
	bindingID := r.PathValue("device_binding_id")
	row, err := s.pg.GetBinding(r.Context(), bindingID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if err := s.pg.RemoveBinding(r.Context(), bindingID); err != nil {
		writeDeviceError(w, err)
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "device.deleted",
		ResourceType: "device_binding", ResourceID: bindingID,
		Message: fmt.Sprintf("admin %s removed device binding %s (%s)", adminEmail, row.DeviceID, row.DisplayName),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAdminMoveDevice(w http.ResponseWriter, r *http.Request) {
	bindingID := r.PathValue("device_binding_id")
	var req struct {
		HouseholdID string `json:"household_id"`
	}
	if !decodeJSON(w, r, &req) || req.HouseholdID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "household_id is required", "")
		return
	}
	row, err := s.pg.GetBinding(r.Context(), bindingID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if err := s.pg.AdminMoveDevice(r.Context(), bindingID, req.HouseholdID); err != nil {
		writeDeviceError(w, err)
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "device.moved",
		ResourceType: "device_binding", ResourceID: bindingID,
		Message: fmt.Sprintf("admin %s moved device %s to household %s", adminEmail, row.DeviceID, req.HouseholdID),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "moved"})
}

func (s *Server) handleAdminExtendTrial(w http.ResponseWriter, r *http.Request) {
	bindingID := r.PathValue("device_binding_id")
	days := adminQueryInt(r, "days", 0)
	if days <= 0 {
		var body struct {
			Days int `json:"days"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Days > 0 {
			days = body.Days
		}
	}
	if days <= 0 {
		days = 30
	}
	row, err := s.pg.GetBinding(r.Context(), bindingID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if err := s.pg.AdminExtendTrial(r.Context(), bindingID, days); err != nil {
		writeDeviceError(w, err)
		return
	}
	if s.vnms != nil {
		_ = s.vnms.Enable(r.Context(), row.DeviceID)
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "subscription.trial_extended",
		ResourceType: "device_binding", ResourceID: bindingID,
		Message: fmt.Sprintf("admin %s extended trial %d days for device %s", adminEmail, days, row.DeviceID),
		Metadata: map[string]any{"days": days},
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "days": days})
}

func (s *Server) handleAdminActivateSubscription(w http.ResponseWriter, r *http.Request) {
	bindingID := r.PathValue("device_binding_id")
	months := adminQueryInt(r, "months", 0)
	if months <= 0 {
		var body struct {
			Months int `json:"months"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Months > 0 {
			months = body.Months
		}
	}
	if months <= 0 {
		months = 1
	}
	row, err := s.pg.GetBinding(r.Context(), bindingID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if err := s.pg.AdminActivateSubscription(r.Context(), bindingID, months); err != nil {
		writeDeviceError(w, err)
		return
	}
	if s.vnms != nil {
		_ = s.vnms.Enable(r.Context(), row.DeviceID)
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "subscription.activated",
		ResourceType: "device_binding", ResourceID: bindingID,
		Message: fmt.Sprintf("admin %s marked device %s subscription active (%d months)", adminEmail, row.DeviceID, months),
		Metadata: map[string]any{"months": months},
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "months": months})
}

func (s *Server) handleAdminGetMonitoredWindows(w http.ResponseWriter, r *http.Request) {
	if s.devSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "device service not configured", "")
		return
	}
	bindingID := r.PathValue("device_binding_id")
	view, err := s.devSvc.GetMonitoredWindows(r.Context(), bindingID)
	if writeDeviceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleAdminPutMonitoredWindows(w http.ResponseWriter, r *http.Request) {
	if s.devSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "device service not configured", "")
		return
	}
	bindingID := r.PathValue("device_binding_id")
	var req struct {
		Timezone  string                   `json:"timezone"`
		Windows   []domain.MonitoredWindow `json:"windows"`
		AlertMode string                   `json:"alert_mode"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	view, err := s.devSvc.SetMonitoredWindows(r.Context(), devices.MonitoredWindowsInput{
		BindingID: bindingID,
		UserID:    userIDFromContext(r.Context()),
		Timezone:  req.Timezone,
		Windows:   req.Windows,
		AlertMode: req.AlertMode,
	})
	if writeDeviceError(w, err) {
		return
	}
	adminEmail, _ := s.pg.GetUserEmail(r.Context(), userIDFromContext(r.Context()))
	s.recordAudit(r, audit.Entry{
		ActorUserID: userIDFromContext(r.Context()), Action: "device.monitored_windows_updated",
		ResourceType: "device_binding", ResourceID: bindingID,
		Message: fmt.Sprintf("admin %s updated monitored windows for binding %s", adminEmail, bindingID),
	})
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) recordAudit(r *http.Request, e audit.Entry) {
	if s.audit != nil {
		s.audit.Record(r.Context(), e)
	}
}

func adminQueryInt(r *http.Request, key string, def int) int {
	if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

var _ = time.Second
