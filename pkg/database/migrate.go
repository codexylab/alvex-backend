package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.up.sql
var postgresMigrations embed.FS

const migrationLockID int64 = 6_269_004_821

// MigratePostgres applies embedded, versioned PostgreSQL migrations in order.
// Every migration is atomic and guarded by a database advisory lock so only
// one deploy can migrate a shared Railway database at a time.
func (db *DB) MigratePostgres(ctx context.Context) error {
	if db.IsSQLite() {
		return fmt.Errorf("postgres migrations cannot run against sqlite")
	}

	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := db.ExecContext(lockCtx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() { _, _ = db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID) }()

	if _, err := db.ExecContext(lockCtx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	entries, err := fs.Glob(postgresMigrations, "migrations/*.up.sql")
	if err != nil {
		return fmt.Errorf("list embedded migrations: %w", err)
	}
	sort.Strings(entries)
	for _, path := range entries {
		version, err := migrationVersion(path)
		if err != nil {
			return err
		}
		var applied bool
		if err := db.QueryRowContext(lockCtx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if applied {
			continue
		}

		sqlBody, err := postgresMigrations.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}
		tx, err := db.BeginTx(lockCtx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err = tx.ExecContext(lockCtx, `SET LOCAL lock_timeout = '10s'; SET LOCAL statement_timeout = '90s'`); err == nil {
			_, err = tx.ExecContext(lockCtx, string(sqlBody))
		}
		if err == nil {
			_, err = tx.ExecContext(lockCtx,
				`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
				version, filepath.Base(path),
			)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

func migrationVersion(path string) (int64, error) {
	name := filepath.Base(path)
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("invalid migration filename %q", name)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid migration version %q: %w", name, err)
	}
	return version, nil
}
