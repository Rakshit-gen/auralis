# Kafka

Kafka carries asynchronous domain events between services. Synchronous needs
(the gateway proxying a call, playback asking content for media keys) use REST;
Kafka is for everything that can be eventually consistent.

Local: `bitnami/kafka:3.9` in KRaft mode, single broker, `NUM_PARTITIONS=3`,
auto topic creation on. Production: a single-node Redpanda instance (Kafka-API
compatible, no code changes) or a managed Kafka such as Redpanda Cloud or
Confluent Cloud. See [DEPLOYMENT.md](DEPLOYMENT.md) for why a plain HTTP queue
is not a substitute.

## Transport security

Local and CI brokers speak PLAINTEXT and need no configuration. Managed brokers
require SASL over TLS. Both the Go (`kafkax`) and Python (`auralis_common.kafka`)
clients read the same environment variables and apply them to every producer,
consumer, and dead-letter writer:

| Variable | Values | Notes |
| --- | --- | --- |
| `KAFKA_SASL_MECHANISM` | `PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512` | empty means PLAINTEXT |
| `KAFKA_SASL_USERNAME` | broker principal | required when a mechanism is set |
| `KAFKA_SASL_PASSWORD` | broker secret | required when a mechanism is set |
| `KAFKA_TLS_ENABLED` | `1`/`true` | implied whenever a mechanism is set |
| `KAFKA_TLS_SKIP_VERIFY` | `1`/`true` | skips certificate verification, test only |

A mechanism set without credentials is a startup error. Managed brokers also
disable auto topic creation, so create the topics in
[Topics](#topics) and their `.dlq` counterparts out of band (`rpk topic create`
or the provider console) before the services start.

## Topics

| Topic | Producer | Consumers |
| --- | --- | --- |
| `auralis.user.events` | auth, user | user, analytics, recommendation |
| `auralis.content.events` | content | playback, analytics, recommendation |
| `auralis.playback.events` | playback | user, analytics, recommendation |
| `auralis.ai.events` | reserved | reserved |
| `auralis.media.events` | reserved | reserved |
| `<topic>.dlq` | consumers | manual inspection |

`auralis.ai.events` and `auralis.media.events` have topic constants and DLQ
counterparts provisioned by the local bootstrap but are not produced to in the
current build; the ai-media pipeline reports progress through its own
`generation_jobs` / `job_events` tables and patches results onto content over
REST. They are kept as named seams for a future split of the pipeline into
event-driven stages.

`libs/go-platform/kafkax` and `libs/py-common/auralis_common/kafka` hold the
topic constants. Referencing a topic by constant makes a rename a compile
error rather than a silent misroute.

## Partitioning and ordering

The producer uses a hash balancer keyed by the **aggregate id** (user id, show
id, episode id, depending on the event). All events for one entity land on the
same partition and are therefore consumed in order. There is no global
ordering and consumers must not assume it.

## Delivery semantics

At-least-once. The outbox relay can crash between publishing to Kafka and
marking the row published, which re-sends on restart. A consumer can crash
between handling a message and committing its offset, which redelivers.

Consumers cope with this by being idempotent:

1. Parse the envelope. An undecodable message goes straight to the DLQ.
2. Check `processed_events (consumer, event_id)`. If present, skip.
3. Handle the event. Handlers use upserts and monotonic updates so a
   double-apply is harmless even if step 2 races.
4. Insert into `processed_events` and commit the offset.

## Retries and dead-lettering

Both the Go and Python consumer wrappers retry a failing handler with capped
exponential backoff (base 500 ms, default 5 attempts). A message that still
fails is written to `<origin-topic>.dlq` with headers:

- `dlq_reason`: the last error, truncated
- `dlq_origin_topic`: the original topic
- (Python also carries the original key and value)

The origin offset is then committed so the consumer group keeps moving. DLQ
topics are not auto-consumed; inspect them with any Kafka console consumer and
replay by re-producing to the origin topic once the bug is fixed.

## Consumer groups

One group per service, so scaling a service to N instances splits its
partitions across them:

| Group | Service |
| --- | --- |
| `user-service` | user |
| `playback-service` | playback |
| `analytics-service` | analytics |
| `recommendation-service` | recommendation |
| `ai-media-service` | ai-media (upload events) |

## Operational notes

- Go consumers using `segmentio/kafka-go` take 30 to 40 seconds to join a group
  on Kafka 4.x because of the rebalance protocol. This shows up as a delay
  before the first event is processed after startup; it is not a hang.
- Python consumers using `aiokafka` join in 1 to 2 seconds.
- `auralis_kafka_consumer_lag_messages` is exported per topic and partition;
  alert on sustained growth. See [OBSERVABILITY.md](OBSERVABILITY.md).
- Offsets start at `FirstOffset` for a new group, so a fresh `recommendation`
  or `analytics` database is populated by replaying history.
