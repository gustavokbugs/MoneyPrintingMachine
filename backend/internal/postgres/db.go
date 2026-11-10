// Package postgres implements persistence adapters on PostgreSQL using pgx.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Connect opens a connection pool and verifies connectivity.
func Connect(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// migrationLockID is an arbitrary constant for pg_advisory_xact_lock so
// concurrent processes do not apply migrations at the same time.
const migrationLockID = 727_001

// Migrate applies embedded migrations in filename order. Each migration
// runs in its own transaction and is recorded in schema_migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	var applied []string
	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		sqlBytes, err := migrationFiles.ReadFile(name)
		if err != nil {
			return applied, fmt.Errorf("read migration %s: %w", version, err)
		}
		did, err := applyMigration(ctx, pool, version, string(sqlBytes))
		if err != nil {
			return applied, err
		}
		if did {
			applied = append(applied, version)
		}
	}
	return applied, nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, version, sql string) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return false, fmt.Errorf("lock migrations: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	if exists {
		return false, nil
	}
	if _, err := tx.Exec(ctx, sql); err != nil {
		return false, fmt.Errorf("apply migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return false, fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit migration %s: %w", version, err)
	}
	return true, nil
}

// isInvalidUUID reports whether err is PostgreSQL's invalid text
// representation error, raised when a malformed UUID is used as a key.
func isInvalidUUID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}

// isUniqueViolation reports whether err is a unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
