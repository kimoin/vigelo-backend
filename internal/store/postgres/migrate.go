package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrateFromDir applies SQL files in dir in lexical order (idempotent migrations).
func MigrateFromDir(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no migration files in %s", dir)
	}
	sort.Strings(matches)
	for _, path := range matches {
		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", path, err)
		}
	}
	return nil
}
