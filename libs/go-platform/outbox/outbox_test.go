package outbox

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/auralis/platform/db"
	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/kafkax"
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

type fakePublisher struct {
	published []kafkax.OutboxRow
	dead      []string
}

func (f *fakePublisher) PublishBatch(_ context.Context, rows []kafkax.OutboxRow) error {
	f.published = append(f.published, rows...)
	return nil
}

func (f *fakePublisher) PublishRaw(_ context.Context, topic, _ string, _ []byte) error {
	f.dead = append(f.dead, topic)
	return nil
}

// relayPool connects with its own schema holding an outbox_events table, so
// these tests don't touch the auth service's rows in the same database.
func relayPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dbURL())
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "outbox_test"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		t.Skipf("test database not reachable (%v)", err)
	}
	t.Cleanup(pool.Close)
	for _, q := range []string{
		`CREATE SCHEMA IF NOT EXISTS outbox_test`,
		`DROP TABLE IF EXISTS outbox_test.outbox_events`,
		`CREATE TABLE outbox_test.outbox_events (
		    id BIGSERIAL PRIMARY KEY, topic TEXT NOT NULL, partition_key TEXT NOT NULL,
		    envelope JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		    published_at TIMESTAMPTZ)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func enqueue(t *testing.T, pool *pgxpool.Pool, eventType string) {
	t.Helper()
	ctx := context.Background()
	env, err := envelope.New(eventType, 1, "test", "corr", "", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(ctx, tx, "auralis.test", "key", env); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBadRowDoesNotBlockTheRelay(t *testing.T) {
	ctx := context.Background()
	pool := relayPool(t)
	enqueue(t, pool, "test.first")
	if _, err := pool.Exec(ctx,
		`INSERT INTO outbox_events (topic, partition_key, envelope) VALUES ('auralis.test', 'key', '{"not": "an envelope"}')`); err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, "test.second")

	pub := &fakePublisher{}
	r := &Relay{pool: pool, producer: pub, batch: 500, service: "test"}
	n, err := r.drain(ctx)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 3 || len(pub.published) != 2 || len(pub.dead) != 1 || pub.dead[0] != "auralis.test.dlq" {
		t.Fatalf("drained %d, published %d, dead-lettered %v", n, len(pub.published), pub.dead)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d rows still pending", left)
	}
}

func TestPruneKeepsPendingAndRecentRows(t *testing.T) {
	ctx := context.Background()
	pool := relayPool(t)
	for i := 0; i < 3; i++ {
		enqueue(t, pool, "test.event")
	}
	// Row 1: published long ago. Row 2: published just now. Row 3: pending.
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET published_at = now() - interval '8 days' WHERE id = 1;
		UPDATE outbox_events SET published_at = now() WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET created_at = now() - interval '30 days' WHERE id = 3`); err != nil {
		t.Fatal(err)
	}
	db.PruneOnce(ctx, pool, map[string]string{"outbox_events": PruneSQL}, t.Logf, t.Logf)
	var ids []int64
	rows, err := pool.Query(ctx, `SELECT id FROM outbox_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Fatalf("rows left after prune: %v, want [2 3]", ids)
	}
}
