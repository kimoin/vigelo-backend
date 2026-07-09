package memory

import (
	"fmt"
	"sync"
	"time"

	"github.com/kimoin/vigelo-backend/internal/domain"
)

// DeviceStore holds device, alert, and push-token state until Phase 3+ migrates
// device bindings to PostgreSQL.
type DeviceStore struct {
	mu         sync.Mutex
	devices    map[string]*domain.DeviceBinding
	alerts     map[string]*domain.Alert
	pushTokens map[string]*domain.PushToken
	nextID     int
}

func NewDevices() *DeviceStore {
	return &DeviceStore{
		devices:    map[string]*domain.DeviceBinding{},
		alerts:     map[string]*domain.Alert{},
		pushTokens: map[string]*domain.PushToken{},
	}
}

func (s *DeviceStore) Lock()   { s.mu.Lock() }
func (s *DeviceStore) Unlock() { s.mu.Unlock() }

func (s *DeviceStore) Devices() map[string]*domain.DeviceBinding { return s.devices }
func (s *DeviceStore) Alerts() map[string]*domain.Alert          { return s.alerts }
func (s *DeviceStore) PushTokens() map[string]*domain.PushToken  { return s.pushTokens }

func (s *DeviceStore) NewID(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s_%06d", prefix, s.nextID)
}

func (s *DeviceStore) RefreshDeviceDerivedState(dev *domain.DeviceBinding) {
	dev.ActiveAlertCount = s.ActiveAlertCount(dev.ID)
	if dev.Subscription.Status == "active" && dev.LastSeenAt != nil {
		dev.Status = "online"
	}
}

func (s *DeviceStore) EnsureDemoAlert(dev *domain.DeviceBinding) {
	if s.ActiveAlertCount(dev.ID) > 0 {
		return
	}
	now := time.Now().UTC()
	a := &domain.Alert{
		ID:              s.NewID("alert"),
		DeviceBindingID: dev.ID,
		Type:            "movement_detected",
		Severity:        "info",
		Status:          "active",
		Title:           "Movement detected",
		Body:            fmt.Sprintf("Movement detected in %s.", dev.RoomOrLocationLabel),
		FirstSeenAt:     now.Add(-30 * time.Minute),
		LastSeenAt:      now.Add(-30 * time.Minute),
	}
	s.alerts[a.ID] = a
	dev.ActiveAlertCount = s.ActiveAlertCount(dev.ID)
}

func (s *DeviceStore) ActiveAlertCount(deviceBindingID string) int {
	n := 0
	for _, a := range s.alerts {
		if a.DeviceBindingID == deviceBindingID && a.Status == "active" {
			n++
		}
	}
	return n
}
