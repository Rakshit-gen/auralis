# Observability

Three signals: metrics (Prometheus), traces (OpenTelemetry over OTLP), and
structured logs. Every service exposes the same endpoints and uses the same
field names so a request can be followed across the stack.

## Endpoints

Every service mounts:

| Path | Purpose |
| --- | --- |
| `/health` | liveness. Process is up. Never checks dependencies, so a flaky database does not cause an orchestrator to kill a healthy pod. |
| `/ready` | readiness. Runs registered checks (database ping, and where relevant Kafka and object storage). Returns 503 until all pass. |
| `/metrics` | Prometheus exposition. |

## Metrics

Defined once in `libs/go-platform/telemetry` (Go) and
`libs/py-common/auralis_common/telemetry` (Python). Names are stable.

| Metric | Type | Labels |
| --- | --- | --- |
| `auralis_http_requests_total` | counter | service, method, route, status (class) |
| `auralis_http_request_duration_seconds` | histogram | service, method, route |
| `auralis_http_errors_total` | counter | service, route (status >= 500) |
| `auralis_db_query_duration_seconds` | histogram | service, operation |
| `auralis_redis_command_duration_seconds` | histogram | service, command |
| `auralis_kafka_produce_duration_seconds` | histogram | service, topic |
| `auralis_kafka_consumer_lag_messages` | gauge | service, topic, partition |
| `auralis_worker_job_duration_seconds` | histogram | service, job |
| `auralis_events_total` | counter | service, name, outcome |
| `auralis_ai_generation_duration_seconds` | histogram | stage, provider (Python) |

`auralis_events_total` carries the domain counters: playback authorize
outcomes (`granted`, `denied_premium`, `not_found`, ...), search queries,
rate-limit blocks, login outcomes, generation job results.

Route labels are the registered route pattern, not the concrete path, so
`/shows/{id}` is one series regardless of id (bounded cardinality).

## Prometheus

`infra/compose/prometheus.yml` scrapes all eight services every 15 seconds on
`/metrics` with a `platform: auralis` label. In production point Prometheus (or
Grafana Agent, or the hosting platform's metrics scraper) at the same paths.

## Grafana

`infra/compose/grafana` provisions a Prometheus datasource and one dashboard,
"Auralis Overview", with:

- HTTP requests per second by service
- HTTP p95 latency by service
- 5xx responses per second
- Kafka consumer lag (messages)
- AI generation and TTS duration (p90)
- Worker job duration (p90)

Local Grafana is on `http://localhost:3001` (`admin` / `admin` by default).

## Tracing

Services export OTLP traces to the endpoint in `OTEL_EXPORTER_OTLP_ENDPOINT`
(`http://otel-collector:4318` locally). The collector
(`otel/opentelemetry-collector-contrib`) receives OTLP on 4317/4318, batches,
and currently exports traces to its debug logger and metrics to a Prometheus
endpoint on 8889. To keep traces, add an exporter (Tempo, Jaeger, or a hosted
backend) to the `traces` pipeline in `infra/compose/otel-collector.yaml`.

Trace context propagates on the standard `traceparent` header, including
through the gateway proxy.

## Logging

Structured JSON to stdout (`log/slog` in Go, `structlog` in Python). Standard
fields on every line:

- `service`, `timestamp`, `level`, `message`
- `request_id`: unique per inbound request, set at the gateway, forwarded as
  `X-Auralis-Request-Id`
- `correlation_id`: stable across every service and every Kafka event caused by
  one action
- `endpoint`: the route

`LOG_LEVEL` (`debug` | `info` | `warn` | `error`) is read at startup. In
production, ship stdout to the platform's log aggregator and index on
`correlation_id`.

## Following one request

1. Find the `request_id` from the client (returned in the error body, or the
   gateway access log).
2. Grep the gateway log for it to get the `correlation_id`.
3. Grep every service log for that `correlation_id`: you get the synchronous
   calls and the outbox events, in order.
4. The same id is on the Kafka envelopes, so consumer-side processing lines up
   too.
