package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kimoin/vigelo-backend/internal/audit"
)

func (s *Store) InsertAudit(ctx context.Context, e audit.Entry) error {
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	var actor any
	if e.ActorUserID != "" {
		actor = e.ActorUserID
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_log (actor_user_id, action, resource_type, resource_id, message, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, actor, e.Action, nullIfEmpty(e.ResourceType), nullIfEmpty(e.ResourceID), e.Message, raw)
	return err
}

func (s *Store) PruneAuditBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM audit_log WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type AuditLogRow struct {
	ID           int64
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Message      string
	MetadataJSON []byte
	CreatedAt    time.Time
}

func (s *Store) ListAuditLog(ctx context.Context, q string, limit, offset int) ([]AuditLogRow, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q = strings.TrimSpace(q)
	var total int
	var rows pgx.Rows
	var err error
	if q != "" {
		pattern := "%" + q + "%"
		if err = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM audit_log
			WHERE message ILIKE $1 OR action ILIKE $1 OR COALESCE(resource_id, '') ILIKE $1
		`, pattern).Scan(&total); err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT id, actor_user_id, action, resource_type, resource_id, message, metadata_json, created_at
			FROM audit_log
			WHERE message ILIKE $1 OR action ILIKE $1 OR COALESCE(resource_id, '') ILIKE $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, pattern, limit, offset)
	} else {
		if err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&total); err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT id, actor_user_id, action, resource_type, resource_id, message, metadata_json, created_at
			FROM audit_log
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AuditLogRow
	for rows.Next() {
		var row AuditLogRow
		var actor, rtype, rid *string
		if err := rows.Scan(&row.ID, &actor, &row.Action, &rtype, &rid, &row.Message, &row.MetadataJSON, &row.CreatedAt); err != nil {
			return nil, 0, err
		}
		if actor != nil {
			row.ActorUserID = *actor
		}
		if rtype != nil {
			row.ResourceType = *rtype
		}
		if rid != nil {
			row.ResourceID = *rid
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
