package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/kimoin/vigelo-backend/internal/audit"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/notifications/push"
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
	Push   push.Sender
	Audit  *audit.Logger
	Sender string
}

func (d *Dispatcher) NotifyAlert(ctx context.Context, alert domain.Alert, householdID string) {
	if d == nil || d.DB == nil {
		return
	}
	channels, err := d.DB.AlertRuleChannels(ctx, alert.DeviceBindingID, alert.Type)
	if err != nil {
		return
	}
	if slices.Contains(channels, "push") && d.Push != nil {
		d.notifyPush(ctx, alert, householdID)
	}
	if slices.Contains(channels, "sms") && slices.Contains(criticalSMSTypes, alert.Type) && d.SMS != nil {
		d.notifySMS(ctx, alert, householdID)
	}
}

func (d *Dispatcher) notifyPush(ctx context.Context, alert domain.Alert, householdID string) {
	tokens, err := d.DB.ListHouseholdPushTokens(ctx, householdID)
	if err != nil || len(tokens) == 0 {
		return
	}
	for _, pt := range tokens {
		err := d.Push.Send(ctx, push.Message{
			Platform:    pt.Platform,
			Token:       pt.Token,
			Title:       alert.Title,
			Body:        alert.Body,
			Environment: pt.Environment,
			AlertID:     alert.ID,
			DeviceID:    alert.DeviceBindingID,
			AlertType:   alert.Type,
		})
		status := "sent"
		errMsg := ""
		dest := pt.Platform + ":" + tokenHint(pt.Token)
		recipient, _ := d.DB.GetUserEmail(ctx, pt.UserID)
		if err != nil {
			status = "failed"
			errMsg = err.Error()
			_ = d.DB.RecordPushTokenDeliveryError(ctx, pt.ID, errMsg)
			if d.Log != nil {
				d.Log.Warn("push delivery failed",
					"push_token_id", pt.ID,
					"platform", pt.Platform,
					"error", err,
				)
			}
		}
		_ = d.DB.RecordNotificationDelivery(ctx, alert.ID, "push", dest, status, errMsg)
		if d.Audit != nil && status == "sent" {
			d.Audit.Record(ctx, audit.Entry{
				Action:       "notification.push_sent",
				ResourceType: "alert",
				ResourceID:   alert.ID,
				Message: fmt.Sprintf("device %s %s alert sent with push notification to %s (%s)",
					alert.DeviceBindingID, alert.Type, recipient, pt.Platform),
				Metadata: map[string]any{
					"recipient_email": recipient,
					"platform":        pt.Platform,
					"alert_type":      alert.Type,
				},
			})
		}
	}
}

func (d *Dispatcher) notifySMS(ctx context.Context, alert domain.Alert, householdID string) {
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
		if d.Audit != nil && status == "sent" {
			d.Audit.Record(ctx, audit.Entry{
				Action:       "notification.sms_sent",
				ResourceType: "alert",
				ResourceID:   alert.ID,
				Message: fmt.Sprintf("device %s %s alert sent with SMS to %s",
					alert.DeviceBindingID, alert.Type, phone),
				Metadata: map[string]any{"phone": phone, "alert_type": alert.Type},
			})
		}
	}
}

func tokenHint(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
