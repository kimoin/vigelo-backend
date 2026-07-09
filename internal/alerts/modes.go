package alerts

import "github.com/kimoin/vigelo-backend/internal/domain"

const (
	AlertModeMovement   = domain.AlertModeMovement
	AlertModeNoMovement = domain.AlertModeNoMovement
)

func ValidAlertMode(mode string) bool {
	return domain.ValidMonitoredWindowAlertMode(mode)
}
