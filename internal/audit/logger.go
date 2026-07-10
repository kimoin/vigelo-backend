package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

const DefaultRetention = 60 * 24 * time.Hour

type Entry struct {
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Message      string
	Metadata     map[string]any
}

type Store interface {
	InsertAudit(ctx context.Context, e Entry) error
	PruneAuditBefore(ctx context.Context, before time.Time) (int64, error)
}

type Logger struct {
	Log       *slog.Logger
	Store     Store
	Retention time.Duration
}

func (l *Logger) Record(ctx context.Context, e Entry) {
	if l == nil || l.Store == nil || e.Message == "" {
		return
	}
	if err := l.Store.InsertAudit(ctx, e); err != nil && l.Log != nil {
		l.Log.Warn("audit log write failed", "action", e.Action, "error", err)
	}
}

func (l *Logger) RunRetention(ctx context.Context) {
	if l == nil || l.Store == nil {
		return
	}
	retention := l.Retention
	if retention <= 0 {
		retention = DefaultRetention
	}
	cutoff := time.Now().UTC().Add(-retention)
	n, err := l.Store.PruneAuditBefore(ctx, cutoff)
	if err != nil {
		if l.Log != nil {
			l.Log.Warn("audit retention failed", "error", err)
		}
		return
	}
	if n > 0 && l.Log != nil {
		l.Log.Info("audit log pruned", "removed", n, "older_than_days", int(retention.Hours()/24))
	}
}

func MetadataJSON(m map[string]any) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage(`{}`)
	}
	b, _ := json.Marshal(m)
	return b
}
