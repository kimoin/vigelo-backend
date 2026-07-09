package schedule

import (
	"testing"
	"time"

	"github.com/kimoin/vigelo-backend/internal/domain"
)

func TestLocalToUTCSingleWindow(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-01-15 is standard time (UTC+2).
	ref := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	windows := []domain.MonitoredWindow{{StartTime: "08:00", EndTime: "20:00"}}
	got, err := LocalToUTC(loc, ref, windows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].StartHour != 6 || got[0].DurationHours != 12 {
		t.Fatalf("got %+v, want start=6 duration=12", got)
	}
}

func TestLocalToUTCCrossingMidnight(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Fatal(err)
	}
	ref := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	windows := []domain.MonitoredWindow{{StartTime: "22:00", EndTime: "06:00"}}
	got, err := LocalToUTC(loc, ref, windows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].StartHour != 20 || got[0].DurationHours != 8 {
		t.Fatalf("got %+v, want one UTC window 20/8", got)
	}
}

func TestValidateRejectsOverlap(t *testing.T) {
	err := ValidateLocalWindows([]domain.MonitoredWindow{
		{StartTime: "08:00", EndTime: "14:00"},
		{StartTime: "12:00", EndTime: "18:00"},
	})
	if err == nil {
		t.Fatal("expected overlap error")
	}
}
