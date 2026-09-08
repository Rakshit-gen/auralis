// Package migrate applies ordered SQL migration files embedded in a service.
//
// Files are named NNNN_description.sql (e.g. 0001_init.sql) and applied in
// lexical order. Each file runs in its own transaction; applied versions are
// recorded in schema_migrations so re-running is a no-op. There is no down
// migration path by design: roll forward with a new file.
package migrate

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run applies every *.sql file in dir of fsys that has not been applied yet.
func Run(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, dir string) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return err
		}
		if err := applyOne(ctx, pool, version, string(body)); err != nil {
			return errors.New("migration " + name + ": " + err.Error())
		}
	}
	return nil
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, version, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
