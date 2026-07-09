package vnmsevents

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kimoin/vigelo-backend/internal/alerts"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

const (
	pollInterval   = 10 * time.Second
	offlineInterval = 5 * time.Minute
	batchLimit     = 100
)

type Consumer struct {
	Log    *slog.Logger
	DB     *postgres.Store
	VNMS   *vnmsclient.Client
	Alerts *alerts.Service
}

func (c *Consumer) Run(ctx context.Context) {
	if c == nil || c.DB == nil || c.VNMS == nil {
		return
	}
	eventTicker := time.NewTicker(pollInterval)
	offlineTicker := time.NewTicker(offlineInterval)
	defer eventTicker.Stop()
	defer offlineTicker.Stop()

	c.poll(ctx)
	c.checkOffline(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-eventTicker.C:
			if err := c.poll(ctx); err != nil && c.Log != nil {
				c.Log.Warn("vnms event poll failed", "error", err)
			}
		case <-offlineTicker.C:
			c.checkOffline(ctx)
		}
	}
}

func (c *Consumer) poll(ctx context.Context) error {
	cursor, err := c.DB.GetEventCursor(ctx)
	if err != nil {
		return err
	}
	view, err := c.VNMS.ListEvents(ctx, cursor, batchLimit)
	if err != nil {
		return err
	}
	for _, ev := range view.Events {
		if err := c.handleEvent(ctx, ev); err != nil {
			return err
		}
	}
	if view.NextCursor > cursor {
		return c.DB.SetEventCursor(ctx, view.NextCursor)
	}
	return nil
}

func (c *Consumer) handleEvent(ctx context.Context, ev vnmsclient.Event) error {
	eventID := postgres.EventID(ev.EventID, ev.IdempotencyKey)
	return c.DB.ProcessEventTx(ctx, eventID, ev.EventType, func(ctx context.Context) error {
		if c.Alerts == nil {
			return nil
		}
		return c.Alerts.HandleEvent(ctx, eventID, ev)
	})
}

func (c *Consumer) checkOffline(ctx context.Context) {
	if c.Alerts == nil {
		return
	}
	if err := c.Alerts.CheckOffline(ctx); err != nil && c.Log != nil {
		c.Log.Warn("offline check failed", "error", err)
	}
}

func (c *Consumer) String() string {
	return fmt.Sprintf("vnmsevents.Consumer")
}
