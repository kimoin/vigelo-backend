package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg := Load()
	if cfg.Addr != "127.0.0.1:8090" {
		t.Fatalf("addr = %q", cfg.Addr)
	}
	if cfg.OfflineHours != 3 {
		t.Fatalf("offline hours = %d", cfg.OfflineHours)
	}
	if cfg.TrialDays != 30 {
		t.Fatalf("trial days = %d", cfg.TrialDays)
	}
	if len(cfg.CORSOrigins) == 0 {
		t.Fatal("expected default CORS origins")
	}
}
