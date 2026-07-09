package alerts

import (
	"testing"
	"time"
)

func TestParseVNMSUTC(t *testing.T) {
	got := parseVNMSUTC("2026-07-09T12:00:00Z")
	if got.IsZero() {
		t.Fatal("expected parsed time")
	}
	if !parseVNMSUTC("").After(time.Now().UTC().Add(-time.Minute)) {
		t.Fatal("expected fallback to now")
	}
}
