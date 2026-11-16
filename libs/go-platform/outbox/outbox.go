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

// PruneSQL deletes rows published more than a week ago. They are only kept
// that long for looking into what was sent. Pass it to db.Prune.
const PruneSQL = `DELETE FROM outbox_events WHERE published_at < now() - interval '7 days'`

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

// publisher is the part of kafkax.Producer the relay uses.
type publisher interface {
	PublishBatch(ctx context.Context, rows []kafkax.OutboxRow) error
	PublishRaw(ctx context.Context, topic, key string, value []byte) error
}

// Relay drains the outbox to Kafka on an interval.
type Relay struct {
	pool     *pgxpool.Pool
	producer publisher
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

// publishTimeout bounds one batch's Kafka write while the relay holds its
// transaction open.
const publishTimeout = 15 * time.Second

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
	type bad struct {
		item
		raw []byte
		err error
	}
	var items []item
	var bads []bad
	for rows.Next() {
		var it item
		var raw []byte
		if err := rows.Scan(&it.id, &it.topic, &it.key, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		env, perr := envelope.Parse(raw)
		if perr != nil {
			bads = append(bads, bad{it, raw, perr})
			continue
		}
		it.env = env
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 && len(bads) == 0 {
		return 0, nil
	}

	// A row that isn't a valid envelope would fail every batch it is in and
	// stop the relay for good. Send it to the dead-letter topic as it is and
	// count it as handled, so the events behind it still go out.
	ids := make([]int64, 0, len(items)+len(bads))
	for _, b := range bads {
		logging.L(ctx).Error("outbox row is not a valid envelope, dead-lettering it",
			"service", r.service, "id", b.id, "topic", b.topic, "error", b.err.Error())
		pctx, cancel := context.WithTimeout(ctx, publishTimeout)
		err := r.producer.PublishRaw(pctx, b.topic+kafkax.DeadLetterSuffix, b.key, b.raw)
		cancel()
		if err != nil {
			return 0, err
		}
		ids = append(ids, b.id)
	}

	rowsToPublish := make([]kafkax.OutboxRow, len(items))
	for i, it := range items {
		rowsToPublish[i] = kafkax.OutboxRow{Topic: it.topic, Key: it.key, Env: it.env}
		ids = append(ids, it.id)
	}
	// One batched produce request for the whole batch. On error nothing is
	// marked published; the rows are retried next tick and consumers dedupe on
	// event_id. Per-entity order holds because the partition key is the
	// aggregate id and kafka-go writes each partition's messages in order.
	//
	// The transaction, and the relay lock with it, stays open across this
	// call, which is what keeps another instance from publishing a later
	// event first. publishTimeout caps how long that can last when Kafka is
	// slow; the batch is then retried on the next tick.
	pctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := r.producer.PublishBatch(pctx, rowsToPublish); err != nil {
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
