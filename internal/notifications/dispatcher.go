package notifications

import (
	"context"
	"log/slog"
	"slices"

	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/notifications/sms"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
)

var criticalSMSTypes = []string{
	"no_movement_detected",
	"device_offline",
}

type Dispatcher struct {
	Log    *slog.Logger
	DB     *postgres.Store
	SMS    sms.Sender
	Sender string
}

func (d *Dispatcher) NotifyAlert(ctx context.Context, alert domain.Alert, householdID string) {
	if d == nil || d.DB == nil || d.SMS == nil {
		return
	}
	if !slices.Contains(criticalSMSTypes, alert.Type) {
		return
	}
	channels, err := d.DB.AlertRuleChannels(ctx, alert.DeviceBindingID, alert.Type)
	if err != nil {
		return
	}
	if !slices.Contains(channels, "sms") {
		return
	}
	phones, err := d.DB.ListVerifiedPhones(ctx, householdID)
	if err != nil || len(phones) == 0 {
		return
	}
	for _, phone := range phones {
		err := d.SMS.Send(ctx, sms.Message{
			To:     phone,
			Body:   alert.Body,
			Sender: d.Sender,
		})
		status := "sent"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = err.Error()
			if d.Log != nil {
				d.Log.Warn("sms delivery failed", "phone", phone, "error", err)
			}
		}
		_ = d.DB.RecordNotificationDelivery(ctx, alert.ID, "sms", phone, status, errMsg)
	}
}
