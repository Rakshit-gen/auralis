package outbox

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func dbURL() string {
	if v := os.Getenv("OUTBOX_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/auth_test?sslmode=disable"
}

// TestRelayLockSerializesDrainers checks that the transaction advisory lock
// drainBatch takes actually blocks a second holder until the first commits.
// Without it two service instances drain in parallel and can publish an
// aggregate's events to Kafka out of order.
func TestRelayLockSerializesDrainers(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL())
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("test database not reachable (%v)", err)
	}
	defer pool.Close()

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx)
	if _, err := tx1.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, relayLockKey); err != nil {
		t.Fatal(err)
	}

	// A second holder must not acquire the lock while tx1 is open.
	got := make(chan struct{})
	go func() {
		tx2, err := pool.Begin(ctx)
		if err != nil {
			return
		}
		defer tx2.Rollback(ctx)
		_, _ = tx2.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, relayLockKey)
		close(got)
	}()

	select {
	case <-got:
		t.Fatal("second drainer acquired the relay lock while the first held it")
	case <-time.After(300 * time.Millisecond):
	}

	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second drainer never acquired the lock after the first released it")
	}
}
