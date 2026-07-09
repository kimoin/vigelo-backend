package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kimoin/vigelo-backend/internal/authz"
	"github.com/kimoin/vigelo-backend/internal/notifications/email"
)

func (s *Server) handleListHouseholds(w http.ResponseWriter, r *http.Request) {
	households, err := s.pg.ListHouseholds(r.Context(), userIDFromContext(r.Context()))
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"households": households})
}

func (s *Server) handleCreateHousehold(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	h, err := s.pg.CreateHousehold(r.Context(), userIDFromContext(r.Context()), fallback(req.Name, "Home"), req.Timezone)
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) handlePatchHousehold(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
	householdID := r.PathValue("household_id")
	role, ok := s.householdRole(w, r, householdID)
	if !ok || !authz.CanManageMembers(role) {
		if ok {
			writeError(w, http.StatusForbidden, "forbidden", "household update not allowed", "")
		}
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Timezone *string `json:"timezone"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	h, err := s.pg.UpdateHousehold(r.Context(), householdID, userIDFromContext(r.Context()), req.Name, req.Timezone)
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	householdID := r.PathValue("household_id")
	members, err := s.pg.ListMembers(r.Context(), householdID, userIDFromContext(r.Context()))
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	householdID := r.PathValue("household_id")
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email is required", "email")
		return
	}
	if req.Role == "" {
		req.Role = string(authz.RoleCaregiver)
	}
	res, err := s.pg.CreateInvite(r.Context(), householdID, userIDFromContext(r.Context()), req.Email, req.Role, s.cfg.InviteTTL)
	if writeStoreError(w, err) {
		return
	}
	inviteURL := fmt.Sprintf("%s/invite/%s", strings.TrimRight(s.cfg.FrontendBaseURL, "/"), res.TokenRaw)
	_ = s.mailer.Send(r.Context(), email.Message{
		To:      req.Email,
		Subject: "You are invited to Vigelo",
		Text:    fmt.Sprintf("Join a Vigelo household: %s", inviteURL),
	})
	writeJSON(w, http.StatusCreated, res.Invite)
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	h, err := s.pg.AcceptInvite(r.Context(), token, userIDFromContext(r.Context()))
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"household": h})
}

func (s *Server) householdRole(w http.ResponseWriter, r *http.Request, householdID string) (authz.Role, bool) {
	role, err := s.pg.GetMembership(r.Context(), householdID, userIDFromContext(r.Context()))
	if writeStoreError(w, err) {
		return "", false
	}
	return authz.ParseRole(role), true
}
