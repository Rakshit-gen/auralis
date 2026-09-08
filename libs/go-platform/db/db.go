// Package db provides a pgx connection pool with sane pool limits, a query
// timing helper that feeds telemetry, and a readiness check.
package db

import (
	"context"
	"time"

	"github.com/auralis/platform/health"
	"github.com/auralis/platform/telemetry"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a pool from a Postgres URL (postgres://user:pass@host:port/db).
func Open(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// ReadyCheck returns a readiness check that pings the pool.
func ReadyCheck(pool *pgxpool.Pool) health.Check {
	return func(ctx context.Context) error {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return pool.Ping(c)
	}
}

// Timed records the duration of fn against the db query metric for service.
func Timed(service, operation string, fn func() error) error {
	done := telemetry.Timer(func(d time.Duration) { telemetry.ObserveDB(service, operation, d) })
	defer done()
	return fn()
}
