// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/volantvm/volant/internal/server/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps a SQLite connection pool with migration metadata.
type Store struct {
	db *sql.DB
}

// Open establishes a SQLite connection, applies migrations, and enables
// recommended pragmas for the orchestrator workload.
func Open(ctx context.Context, path string) (*Store, error) {
	expanded, err := expandPath(path)
	if err != nil {
		return nil, fmt.Errorf("expand path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(expanded), 0o755); err != nil {
		return nil, fmt.Errorf("ensure database directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=1", expanded)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := configurePool(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := applyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close shuts down the underlying connection pool.
func (s *Store) Close(ctx context.Context) error {
	closeCh := make(chan error, 1)
	go func() { closeCh <- s.db.Close() }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-closeCh:
		return err
	}
}

// Queries returns repository accessors bound to the root connection.
func (s *Store) Queries() db.Queries {
	return &queries{exec: s.db}
}

// WithTx executes fn within a SQL transaction, rolling back on error.
func (s *Store) WithTx(ctx context.Context, fn func(db.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	q := &queries{exec: tx}
	if err := fn(q); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback tx after error %v: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func configurePool(db *sql.DB) error {
	db.SetMaxOpenConns(1) // SQLite is single-writer; keep pool disciplined.
	db.SetConnMaxLifetime(0)
	return nil
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        name TEXT NOT NULL,
        applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
    );`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied, err := loadApplied(ctx, db)
	if err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := executeMigration(ctx, db, m); err != nil {
			return err
		}
	}

	return nil
}

func loadApplied(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("select applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan migration version: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}

	sort.Strings(entries)
	migrations := make([]migration, 0, len(entries))
	for _, path := range entries {
		content, err := fs.ReadFile(migrationsFS, path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", path, err)
		}
		base := filepath.Base(path)
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration filename: %s", base)
		}
		version, err := parseVersion(parts[0])
		if err != nil {
			return nil, fmt.Errorf("parse version for %s: %w", base, err)
		}
		name := strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
		migrations = append(migrations, migration{version: version, name: name, sql: string(content)})
	}
	return migrations, nil
}

func parseVersion(prefix string) (int, error) {
	var v int
	if _, err := fmt.Sscanf(prefix, "%d", &v); err != nil {
		return 0, err
	}
	return v, nil
}

func executeMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.version, err)
	}

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %d: %w", m.version, err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?);`, m.version, m.name, time.Now().UTC()); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %d: %w", m.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", m.version, err)
	}
	return nil
}

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
	}
	return path, nil
}
