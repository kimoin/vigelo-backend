package trials

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kimoin/vigelo-backend/internal/audit"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

type Worker struct {
	Log    *slog.Logger
	DB     *postgres.Store
	VNMS   *vnmsclient.Client
	Audit  *audit.Logger
	Every  time.Duration
}

func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.DB == nil {
		return
	}
	interval := w.Every
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	rows, err := w.DB.ListExpiredTrials(ctx)
	if err != nil {
		if w.Log != nil {
			w.Log.Warn("trial expiry check failed", "error", err)
		}
		return
	}
	for _, row := range rows {
		if w.VNMS != nil {
			if err := w.VNMS.Disable(ctx, row.DeviceID); err != nil && w.Log != nil {
				w.Log.Warn("vnms disable on trial expiry failed", "device_id", row.DeviceID, "error", err)
			}
		}
		if err := w.DB.SuspendSubscription(ctx, row.BindingID); err != nil {
			if w.Log != nil {
				w.Log.Warn("suspend subscription failed", "binding_id", row.BindingID, "error", err)
			}
			continue
		}
		msg := fmt.Sprintf("%s trial expired for device %s in household %q",
			emailOrUnknown(row.OwnerEmail), row.DeviceID, row.HouseholdName)
		if w.Audit != nil {
			w.Audit.Record(ctx, audit.Entry{
				Action:       "subscription.trial_expired",
				ResourceType: "device_binding",
				ResourceID:   row.BindingID,
				Message:      msg,
				Metadata: map[string]any{
					"device_id":      row.DeviceID,
					"household_id":   row.HouseholdID,
					"household_name": row.HouseholdName,
					"owner_email":    row.OwnerEmail,
				},
			})
		}
	}
}

func emailOrUnknown(email string) string {
	if email == "" {
		return "User"
	}
	return email
}
