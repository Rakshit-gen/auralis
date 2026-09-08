// Package kafkax wraps segmentio/kafka-go with the platform conventions:
// envelope-encoded values, at-least-once consumption, idempotent handling via a
// processed-events table, bounded retries, and a dead-letter topic.
package kafkax

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/telemetry"
	"github.com/segmentio/kafka-go"
)

// Topics used across the platform. Producers and consumers reference these
// constants so a rename is a compile error, not a silent misroute.
const (
	TopicUserEvents     = "auralis.user.events"
	TopicContentEvents  = "auralis.content.events"
	TopicPlaybackEvents = "auralis.playback.events"
	TopicAIEvents       = "auralis.ai.events"
	TopicMediaEvents    = "auralis.media.events"
	DeadLetterSuffix    = ".dlq"
)

// AllTopics is the set the local bootstrap creates.
var AllTopics = []string{
	TopicUserEvents, TopicContentEvents, TopicPlaybackEvents, TopicAIEvents, TopicMediaEvents,
	TopicUserEvents + DeadLetterSuffix, TopicContentEvents + DeadLetterSuffix,
	TopicPlaybackEvents + DeadLetterSuffix, TopicAIEvents + DeadLetterSuffix,
	TopicMediaEvents + DeadLetterSuffix,
}

func brokers(csv string) []string { return strings.Split(csv, ",") }

// Producer sends envelopes to Kafka. It is safe for concurrent use.
type Producer struct {
	w       *kafka.Writer
	service string
}

// NewProducer connects a writer to the given comma-separated broker list.
func NewProducer(brokerCSV, service string) *Producer {
	return &Producer{
		service: service,
		w: &kafka.Writer{
			Addr:                   kafka.TCP(brokers(brokerCSV)...),
			Balancer:               &kafka.Hash{}, // key-based partitioning keeps per-entity order
			RequiredAcks:           kafka.RequireAll,
			AllowAutoTopicCreation: true,
			BatchTimeout:           50 * time.Millisecond,
			WriteTimeout:           10 * time.Second,
			MaxAttempts:            5,
		},
	}
}

// Publish sends env to topic, partitioned by key (usually the aggregate id).
func (p *Producer) Publish(ctx context.Context, topic, key string, env envelope.Envelope) error {
	if err := env.Validate(); err != nil {
		return fmt.Errorf("kafkax: refusing to publish invalid envelope: %w", err)
	}
	done := telemetry.Timer(func(d time.Duration) { telemetry.ObserveKafkaProduce(p.service, topic, d) })
	defer done()
	err := p.w.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: env.Bytes(),
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(env.EventType)},
			{Key: "event_version", Value: []byte(fmt.Sprint(env.EventVersion))},
			{Key: "correlation_id", Value: []byte(env.CorrelationID)},
		},
		Time: env.OccurredAt,
	})
	if err != nil {
		telemetry.Count(p.service, "kafka_publish", "failure")
		return err
	}
	telemetry.Count(p.service, "kafka_publish", "success")
	return nil
}

func (p *Producer) Close() error { return p.w.Close() }

// Handler processes a single event. Returning a non-nil error triggers retry and
// eventually dead-lettering. Handlers must be idempotent regardless.
type Handler func(ctx context.Context, env envelope.Envelope) error

// Seen records processed event ids so a redelivered message is skipped. The
// consuming service implements this against its own database in one transaction
// with the handler's side effects where possible.
type Seen interface {
	// AlreadyProcessed reports whether eventID was handled before.
	AlreadyProcessed(ctx context.Context, consumer, eventID string) (bool, error)
	// MarkProcessed records eventID as handled by consumer.
	MarkProcessed(ctx context.Context, consumer, eventID string) error
}

// ConsumerConfig configures a group consumer.
type ConsumerConfig struct {
	BrokerCSV  string
	GroupID    string
	Topics     []string
	Service    string
	MaxRetries int           // per-message handler attempts before DLQ (default 5)
	RetryBase  time.Duration // base backoff (default 500ms, capped exponential)
}

// Consumer runs a consumer-group loop with idempotency and dead-lettering.
type Consumer struct {
	cfg  ConsumerConfig
	r    *kafka.Reader
	dlq  *kafka.Writer
	seen Seen
}

// NewConsumer builds a consumer. seen may be nil to disable the dedupe check
// (handlers must still be idempotent).
func NewConsumer(cfg ConsumerConfig, seen Seen) *Consumer {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5
	}
	if cfg.RetryBase == 0 {
		cfg.RetryBase = 500 * time.Millisecond
	}
	return &Consumer{
		cfg:  cfg,
		seen: seen,
		r: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers(cfg.BrokerCSV),
			GroupID:        cfg.GroupID,
			GroupTopics:    cfg.Topics,
			MinBytes:       1,
			MaxBytes:       10 << 20,
			CommitInterval: 0, // commit explicitly after each message
			StartOffset:    kafka.FirstOffset,
		}),
		dlq: &kafka.Writer{
			Addr:                   kafka.TCP(brokers(cfg.BrokerCSV)...),
			Balancer:               &kafka.Hash{},
			RequiredAcks:           kafka.RequireAll,
			AllowAutoTopicCreation: true,
		},
	}
}

// Run consumes until ctx is cancelled. Each message is retried with exponential
// backoff up to MaxRetries; a message that still fails is written to
// "<topic>.dlq" with failure headers and the offset is committed so the group
// makes progress.
func (c *Consumer) Run(ctx context.Context, handle Handler) error {
	log := logging.L(ctx).With("consumer", c.cfg.GroupID)
	log.Info("consumer started", "topics", c.cfg.Topics)
	defer c.r.Close()
	defer c.dlq.Close()

	for {
		m, err := c.r.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			log.Error("fetch failed", "error", err.Error())
			time.Sleep(time.Second)
			continue
		}
		telemetry.SetConsumerLag(c.cfg.Service, m.Topic, m.Partition, c.r.Lag())
		c.process(ctx, log, m, handle)
		if err := c.r.CommitMessages(ctx, m); err != nil {
			log.Error("commit failed", "error", err.Error(), "topic", m.Topic, "offset", m.Offset)
		}
	}
}

func (c *Consumer) process(ctx context.Context, log logging.Logger, m kafka.Message, handle Handler) {
	env, err := envelope.Parse(m.Value)
	if err != nil {
		log.Error("undecodable message dead-lettered", "error", err.Error(), "topic", m.Topic)
		c.deadLetter(ctx, m, "envelope_parse_error: "+err.Error())
		return
	}
	l := log.With("event_id", env.EventID, "event_type", env.EventType, "correlation_id", env.CorrelationID)

	if c.seen != nil {
		done, serr := c.seen.AlreadyProcessed(ctx, c.cfg.GroupID, env.EventID)
		if serr != nil {
			l.Error("dedupe check failed, processing anyway", "error", serr.Error())
		} else if done {
			l.Debug("duplicate event skipped")
			telemetry.Count(c.cfg.Service, "kafka_consume", "duplicate")
			return
		}
	}

	start := time.Now()
	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxRetries; attempt++ {
		lastErr = handle(ctx, env)
		if lastErr == nil {
			break
		}
		l.Warn("handler failed", "attempt", attempt, "error", lastErr.Error())
		if attempt < c.cfg.MaxRetries {
			backoff := c.cfg.RetryBase * (1 << (attempt - 1))
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}
	telemetry.ObserveWorker(c.cfg.Service, "consume:"+env.EventType, time.Since(start))

	if lastErr != nil {
		l.Error("event dead-lettered after retries", "error", lastErr.Error())
		telemetry.Count(c.cfg.Service, "kafka_consume", "dead_lettered")
		c.deadLetter(ctx, m, lastErr.Error())
		return
	}
	if c.seen != nil {
		if err := c.seen.MarkProcessed(ctx, c.cfg.GroupID, env.EventID); err != nil {
			l.Error("failed to record processed event", "error", err.Error())
		}
	}
	telemetry.Count(c.cfg.Service, "kafka_consume", "success")
}

func (c *Consumer) deadLetter(ctx context.Context, m kafka.Message, reason string) {
	err := c.dlq.WriteMessages(ctx, kafka.Message{
		Topic: m.Topic + DeadLetterSuffix,
		Key:   m.Key,
		Value: m.Value,
		Headers: append(m.Headers,
			kafka.Header{Key: "dlq_reason", Value: []byte(reason)},
			kafka.Header{Key: "dlq_origin_topic", Value: []byte(m.Topic)},
			kafka.Header{Key: "dlq_at", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
		),
	})
	if err != nil {
		logging.L(ctx).Error("failed to write to dead-letter topic", "error", err.Error(), "topic", m.Topic)
	}
}

// EnsureTopics creates the platform topics if they do not exist. Used by the
// local bootstrap and tests; production brokers are provisioned out of band.
func EnsureTopics(ctx context.Context, brokerCSV string, partitions, replication int) error {
	conn, err := kafka.DialContext(ctx, "tcp", brokers(brokerCSV)[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	cc, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return err
	}
	defer cc.Close()
	cfgs := make([]kafka.TopicConfig, 0, len(AllTopics))
	for _, t := range AllTopics {
		cfgs = append(cfgs, kafka.TopicConfig{Topic: t, NumPartitions: partitions, ReplicationFactor: replication})
	}
	return cc.CreateTopics(cfgs...)
}
