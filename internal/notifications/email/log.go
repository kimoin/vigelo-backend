package email

import (
	"context"
	"log/slog"
)

type LogSender struct {
	Log *slog.Logger
}

func (s *LogSender) Send(ctx context.Context, msg Message) error {
	if s.Log == nil {
		s.Log = slog.Default()
	}
	s.Log.Info("email (dev log sender)",
		"to", msg.To,
		"subject", msg.Subject,
		"text", msg.Text,
	)
	return nil
}
