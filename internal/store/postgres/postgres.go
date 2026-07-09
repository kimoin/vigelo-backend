package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool *pgxpool.Pool
}

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return fmt.Errorf("database not configured")
	}
	return db.pool.Ping(ctx)
}

func (db *DB) Close() {
	if db != nil && db.pool != nil {
		db.pool.Close()
	}
}

func (db *DB) Pool() *pgxpool.Pool {
	if db == nil {
		return nil
	}
	return db.pool
}

func NewStore(db *DB) *Store {
	if db == nil {
		return nil
	}
	return &Store{pool: db.Pool()}
}
