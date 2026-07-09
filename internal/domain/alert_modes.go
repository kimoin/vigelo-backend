package domain

const (
	AlertModeMovement   = "movement_detected"
	AlertModeNoMovement = "no_movement_detected"
)

func ValidMonitoredWindowAlertMode(mode string) bool {
	return mode == AlertModeMovement || mode == AlertModeNoMovement
}
