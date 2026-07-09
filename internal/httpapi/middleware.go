package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field,omitempty"`
	} `json:"error"`
}

type contextUserID struct{}
type contextAccessToken struct{}

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

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePG(w) {
			return
		}
		token := bearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token", "")
			return
		}
		userID, _, err := s.pg.ResolveAccessToken(r.Context(), token)
		if writeStoreError(w, err) {
			return
		}
		ctx := context.WithValue(r.Context(), contextUserID{}, userID)
		ctx = context.WithValue(ctx, contextAccessToken{}, token)
		next(w, r.WithContext(ctx))
	}
}

func userIDFromContext(ctx context.Context) string {
	return ctx.Value(contextUserID{}).(string)
}

func accessTokenFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextAccessToken{}).(string)
	return v
}

func withCORS(allowed map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"status": "ok"}
	if s.db != nil {
		if err := s.db.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status":   "degraded",
				"database": "unavailable",
			})
			return
		}
		resp["database"] = "ok"
	}
	writeJSON(w, http.StatusOK, resp)
}
