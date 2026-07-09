package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kimoin/vigelo-backend/internal/config"
	"github.com/kimoin/vigelo-backend/internal/logging"
)

func TestHealthzWithoutDatabase(t *testing.T) {
	srv := New(logging.New("error"), config.Config{}, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
