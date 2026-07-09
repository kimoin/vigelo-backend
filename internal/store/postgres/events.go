package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) GetEventCursor(ctx context.Context) (int64, error) {
	var cursor int64
	err := s.pool.QueryRow(ctx, `SELECT cursor_id FROM vnms_event_cursor WHERE id = 1`).Scan(&cursor)
	return cursor, err
}

func (s *Store) SetEventCursor(ctx context.Context, cursor int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vnms_event_cursor SET cursor_id = $1, updated_at = now() WHERE id = 1
	`, cursor)
	return err
}

func (s *Store) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM processed_vnms_events WHERE event_id = $1)
	`, eventID).Scan(&exists)
	return exists, err
}

func (s *Store) MarkEventProcessed(ctx context.Context, eventID, eventType string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO processed_vnms_events (event_id, event_type)
		VALUES ($1, $2)
		ON CONFLICT (event_id) DO NOTHING
	`, eventID, eventType)
	return err
}

func (s *Store) ProcessEventTx(ctx context.Context, eventID, eventType string, fn func(ctx context.Context) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM processed_vnms_events WHERE event_id = $1)
	`, eventID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit(ctx)
	}

	if err := fn(ctx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO processed_vnms_events (event_id, event_type) VALUES ($1, $2)
	`, eventID, eventType); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func EventID(eventID int64, idempotencyKey string) string {
	if idempotencyKey != "" {
		return idempotencyKey
	}
	return fmt.Sprintf("vnms:%d", eventID)
}

var ErrCursorConflict = errors.New("event cursor conflict")

func (s *Store) AdvanceEventCursorIf(ctx context.Context, want, next int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vnms_event_cursor
		SET cursor_id = $2, updated_at = now()
		WHERE id = 1 AND cursor_id = $1
	`, want, next)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCursorConflict
	}
	return nil
}

func (s *Store) BindingIDByDeviceID(ctx context.Context, deviceID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM device_bindings WHERE device_id = $1 AND removed_at IS NULL
	`, deviceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrBindingNotFound
	}
	return id, err
}
