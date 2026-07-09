package domain

import "testing"

func TestValidMonitoredWindowAlertMode(t *testing.T) {
	if !ValidMonitoredWindowAlertMode(AlertModeMovement) || !ValidMonitoredWindowAlertMode(AlertModeNoMovement) {
		t.Fatal("expected valid modes")
	}
	if ValidMonitoredWindowAlertMode("both") {
		t.Fatal("expected invalid mode")
	}
}
