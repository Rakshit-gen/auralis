package db

import (
	"context"
	"time"

	"github.com/auralis/platform/logging"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Prune runs each DELETE in stmts now and then every interval until ctx is
// cancelled. It is for tables that only ever grow, such as dedupe records
// and published outbox rows, so they stay the size of their useful window.
// Several instances running it at once is harmless; the deletes just overlap.
func Prune(ctx context.Context, pool *pgxpool.Pool, every time.Duration, stmts map[string]string) {
	log := logging.L(ctx).With("component", "prune")
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		PruneOnce(ctx, pool, stmts, log.Warn, log.Info)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PruneOnce runs each statement once, reporting failures to warn and rows
// removed to info.
func PruneOnce(ctx context.Context, pool *pgxpool.Pool, stmts map[string]string, warn, info func(string, ...any)) {
	for name, sql := range stmts {
		qctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		ct, err := pool.Exec(qctx, sql)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				warn("prune failed", "table", name, "error", err.Error())
			}
			continue
		}
		if n := ct.RowsAffected(); n > 0 {
			info("pruned old rows", "table", name, "rows", n)
		}
	}
}
