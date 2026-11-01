# notification

Java 21, Spring Boot 4.1. Tells listeners when a show they follow gets a new
episode.

## What it does

1. Consumes `user.followed` / `user.unfollowed` from `auralis.user.events` and
   keeps a `show_followers` projection. `user` stays the source of truth.
2. On `content.episode_published` from `auralis.content.events`, inserts one
   `new_episode` notification per follower with a single
   `INSERT ... SELECT ... ON CONFLICT DO NOTHING`. The show's creator is
   skipped, and a re-publish of the same episode notifies nobody twice.
3. Serves the caller's inbox behind the gateway at `/api/notifications`.

Every event is recorded in `processed_events` in the same transaction as its
writes, so redelivery is a no-op. Failed events retry five times with
backoff from 500ms, then go to `<topic>.dlq`; an envelope that fails validation
goes there on the first attempt.

## API

All routes need the gateway's signed identity headers.

| Method | Path | Returns |
| --- | --- | --- |
| GET | `/notifications?limit=20&before=<cursor>` | `notifications`, `unread`, `next_before` |
| GET | `/notifications/unread-count` | `unread` |
| POST | `/notifications/{id}/read` | `read`, or 404 if it is not the caller's |
| POST | `/notifications/read-all` | `updated` |

`limit` is clamped to 1..100. Paging is keyset on `(created_at, id)`; pass the
previous page's `next_before` back as `before`. `/health`, `/ready` and
`/metrics` are open, as on every service.

## Configuration

| Variable | Default | |
| --- | --- | --- |
| `NOTIFICATION_HTTP_ADDR` | `:8088` | `host:port` or `:port` |
| `NOTIFICATION_DATABASE_URL` | required | `postgres://` URL, converted to JDBC at startup |
| `IDENTITY_SECRET` | required | shared with the gateway |
| `KAFKA_BROKERS` | `localhost:9092` | |
| `KAFKA_SASL_*`, `KAFKA_TLS_*` | unset | same meaning as in `libs/go-platform/kafkax` |
| `NOTIFICATION_RUN_CONSUMER` | `true` | `false` serves the API only |
| `NOTIFICATION_MAX_PUBLISH_AGE` | `24h` | older publish events update nothing, so a replay does not flood inboxes |

## Run and test

```
mvn -q package -DskipTests
java -jar target/notification.jar migrate   # apply Flyway migrations and exit
java -jar target/notification.jar

mvn verify                                  # unit tests
NOTIFICATION_TEST_DATABASE_URL=postgres://localhost:5432/notification_test?sslmode=disable mvn verify
```

## Not built yet

- No OpenTelemetry tracing. The Go services export OTLP spans; this one only
  has logs and Prometheus metrics.
- Logs are Spring's default text format, not the JSON the Go services write.
- In-app only. No email or push delivery.
- No frontend yet. The web client does not show the inbox or an unread badge.
