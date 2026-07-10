package push

import (
	"context"
	"log/slog"
)

// LogSender records push payloads locally. Used when PUSH_PROVIDER=log or as a
// fallback before APNs credentials are configured.
type LogSender struct {
	Log *slog.Logger
}

func (s *LogSender) Send(ctx context.Context, msg Message) error {
	if s.Log == nil {
		s.Log = slog.Default()
	}
	s.Log.Info("push (dev log sender)",
		"platform", msg.Platform,
		"token_hint", tokenHint(msg.Token),
		"title", msg.Title,
		"body", msg.Body,
		"alert_id", msg.AlertID,
		"device_binding_id", msg.DeviceID,
	)
	return nil
}

func tokenHint(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
