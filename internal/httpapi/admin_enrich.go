package httpapi

import (
	"context"
	"time"

	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

func (s *Server) enrichAdminUserDetailVNMS(ctx context.Context, d *postgres.AdminUserDetail) {
	if s.vnms == nil {
		return
	}
	ids := adminCollectDeviceIDs(d)
	if len(ids) == 0 {
		return
	}
	states, err := s.vnms.BatchGet(ctx, ids)
	if err != nil {
		return
	}
	threshold := time.Duration(s.cfg.OfflineHours) * time.Hour
	for i := range d.Households {
		for j := range d.Households[i].Devices {
			applyVNMSState(&d.Households[i].Devices[j], states[d.Households[i].Devices[j].DeviceID], threshold)
		}
	}
	for i := range d.Subscriptions {
		applyVNMSState(&d.Subscriptions[i], states[d.Subscriptions[i].DeviceID], threshold)
	}
}

func adminCollectDeviceIDs(d *postgres.AdminUserDetail) []string {
	seen := make(map[string]struct{})
	var ids []string
	add := func(devs []postgres.AdminDeviceDetail) {
		for _, dev := range devs {
			if dev.Removed || dev.DeviceID == "" {
				continue
			}
			if _, ok := seen[dev.DeviceID]; ok {
				continue
			}
			seen[dev.DeviceID] = struct{}{}
			ids = append(ids, dev.DeviceID)
		}
	}
	for _, hh := range d.Households {
		add(hh.Devices)
	}
	add(d.Subscriptions)
	return ids
}

func applyVNMSState(d *postgres.AdminDeviceDetail, st vnmsclient.DeviceState, offlineThreshold time.Duration) {
	if st.DeviceID == "" {
		if d.LastContactAt != nil {
			d.DeviceStatus = adminDeviceStatus(d.LastContactAt, offlineThreshold)
		} else {
			d.DeviceStatus = "waiting_for_first_contact"
		}
		return
	}
	if st.LastContactAt != nil {
		d.LastContactAt = st.LastContactAt
	}
	if st.LastVoltageMv != nil {
		v := float64(*st.LastVoltageMv) / 1000.0
		d.BatteryVoltageV = &v
		d.BatteryStatus = adminBatteryStatus(v)
	} else if d.BatteryStatus == "" {
		d.BatteryStatus = "unknown"
	}
	d.DeviceStatus = adminDeviceStatus(st.LastContactAt, offlineThreshold)
	if st.LifecycleState == "disabled" {
		d.DeviceStatus = "service_suspended"
	}
}

func adminDeviceStatus(lastContact *time.Time, offlineThreshold time.Duration) string {
	if lastContact == nil {
		return "waiting_for_first_contact"
	}
	if time.Since(*lastContact) > offlineThreshold {
		return "offline"
	}
	return "online"
}

func adminBatteryStatus(v float64) string {
	switch {
	case v >= 2.9:
		return "ok"
	case v >= 2.7:
		return "low"
	default:
		return "critical"
	}
}
