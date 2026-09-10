// Package outbox implements the transactional outbox pattern for Go services.
//
// A service writes domain state and an outbox row in the same database
// transaction. A relay loop then reads unpublished rows in order, sends them to
// Kafka, and marks them published. This guarantees an important event is never
// visible to consumers before its transaction commits, at the cost of
// at-least-once delivery (a crash between publish and mark re-sends the row).
//
// The service must create the table via its own migration:
//
//	CREATE TABLE outbox_events (
//	    id           BIGSERIAL PRIMARY KEY,
//	    topic        TEXT NOT NULL,
//	    partition_key TEXT NOT NULL,
//	    envelope     JSONB NOT NULL,
//	    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
//	    published_at TIMESTAMPTZ
//	);
//	CREATE INDEX outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;
package outbox

import (
	"context"
	"time"

	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Enqueue writes one outbox row inside the caller's transaction.
func Enqueue(ctx context.Context, tx pgx.Tx, topic, partitionKey string, env envelope.Envelope) error {
	if err := env.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO outbox_events (topic, partition_key, envelope) VALUES ($1, $2, $3)`,
		topic, partitionKey, env.Bytes())
	return err
}

// Relay drains the outbox to Kafka on an interval.
type Relay struct {
	pool     *pgxpool.Pool
	producer *kafkax.Producer
	interval time.Duration
	batch    int
	service  string
}

func NewRelay(pool *pgxpool.Pool, producer *kafkax.Producer, service string) *Relay {
	return &Relay{pool: pool, producer: producer, interval: time.Second, batch: 500, service: service}
}

// Run publishes pending rows until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	log := logging.L(ctx).With("component", "outbox_relay", "service", r.service)
	log.Info("outbox relay started")
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("outbox relay stopped")
			return
		case <-t.C:
			if n, err := r.drain(ctx); err != nil {
				log.Error("outbox drain failed", "error", err.Error())
			} else if n > 0 {
				log.Debug("outbox rows published", "count", n)
			}
		}
	}
}

func (r *Relay) drain(ctx context.Context) (int, error) {
	published := 0
	for {
		n, err := r.drainBatch(ctx)
		published += n
		if err != nil || n == 0 {
			return published, err
		}
	}
}

// relayLockKey is an arbitrary fixed key for the advisory lock that serializes
// relays across service instances.
const relayLockKey = 0x6175726c7279 // "aurlry"

// drainBatch publishes a batch of unpublished rows in id order and marks them
// published. It first takes a transaction-scoped advisory lock so only one
// instance's relay drains at a time: with several instances draining in
// parallel a later event for an aggregate could reach Kafka before an earlier
// one still sitting in another instance's batch, breaking per-entity order.
func (r *Relay) drainBatch(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, relayLockKey); err != nil {
		return 0, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, topic, partition_key, envelope FROM outbox_events
		 WHERE published_at IS NULL ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, r.batch)
	if err != nil {
		return 0, err
	}
	type item struct {
		id    int64
		topic string
		key   string
		env   envelope.Envelope
	}
	var items []item
	for rows.Next() {
		var it item
		var raw []byte
		if err := rows.Scan(&it.id, &it.topic, &it.key, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		env, perr := envelope.Parse(raw)
		if perr != nil {
			rows.Close()
			return 0, perr
		}
		it.env = env
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}

	rowsToPublish := make([]kafkax.OutboxRow, len(items))
	ids := make([]int64, len(items))
	for i, it := range items {
		rowsToPublish[i] = kafkax.OutboxRow{Topic: it.topic, Key: it.key, Env: it.env}
		ids[i] = it.id
	}
	// One batched produce request for the whole batch. On error nothing is
	// marked published; the rows are retried next tick and consumers dedupe on
	// event_id. Per-entity order holds because the partition key is the
	// aggregate id and kafka-go writes each partition's messages in order.
	if err := r.producer.PublishBatch(ctx, rowsToPublish); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE outbox_events SET published_at = now() WHERE id = ANY($1)`, ids); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(ids), nil
}
