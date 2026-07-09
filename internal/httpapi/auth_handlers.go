package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/notifications/email"
)

func publicUser(u domain.User) map[string]any {
	out := map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"created_at":   u.CreatedAt,
	}
	if u.EmailVerifiedAt != nil {
		out["email_verified_at"] = u.EmailVerifiedAt
	}
	if u.Timezone != nil {
		out["timezone"] = *u.Timezone
	}
	return out
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
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

	res, err := s.pg.Signup(r.Context(), req.Email, req.Password, fallback(req.DisplayName, req.Email),
		s.cfg.VerifyEmailTTL, s.cfg.AccessTokenTTL, s.cfg.RefreshTokenTTL)
	if writeStoreError(w, err) {
		return
	}

	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimRight(s.cfg.FrontendBaseURL, "/"), res.VerifyRaw)
	_ = s.mailer.Send(r.Context(), email.Message{
		To:      req.Email,
		Subject: "Verify your Vigelo email",
		Text:    fmt.Sprintf("Welcome to Vigelo. Verify your email: %s", verifyURL),
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"access_token":  res.Tokens.AccessToken,
		"refresh_token": res.Tokens.RefreshToken,
		"user":          publicUser(res.User),
		"household":     res.Household,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, tokens, err := s.pg.Login(r.Context(), req.Email, req.Password, s.cfg.AccessTokenTTL, s.cfg.RefreshTokenTTL)
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"user":          publicUser(user),
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.pg.RefreshSession(r.Context(), accessTokenFromContext(r.Context()), s.cfg.AccessTokenTTL, s.cfg.RefreshTokenTTL)
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	_ = s.pg.Logout(r.Context(), accessTokenFromContext(r.Context()))
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.pg.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (s *Server) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName *string `json:"display_name"`
		Timezone    *string `json:"timezone"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.pg.UpdateUser(r.Context(), userIDFromContext(r.Context()), req.DisplayName, req.Timezone)
	if writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.pg.VerifyEmail(r.Context(), req.Token); writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (s *Server) handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	raw, err := s.pg.RequestPasswordReset(r.Context(), req.Email, s.cfg.ResetPasswordTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error", "")
		return
	}
	if raw != "" {
		resetURL := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimRight(s.cfg.FrontendBaseURL, "/"), raw)
		_ = s.mailer.Send(r.Context(), email.Message{
			To:      strings.ToLower(strings.TrimSpace(req.Email)),
			Subject: "Reset your Vigelo password",
			Text:    fmt.Sprintf("Reset your password: %s", resetURL),
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePasswordResetComplete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePG(w) {
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters", "new_password")
		return
	}
	if err := s.pg.CompletePasswordReset(r.Context(), req.Token, req.NewPassword); writeStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_updated"})
}

func fallback(v, fb string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fb
	}
	return v
}
