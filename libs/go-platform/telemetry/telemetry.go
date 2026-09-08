// Package telemetry exposes Prometheus metrics and OpenTelemetry tracing setup
// shared by every Go service. Metric names are stable and documented in
// docs/OBSERVABILITY.md.
package telemetry

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auralis_http_requests_total",
		Help: "HTTP requests handled, by service, method, route, and status class.",
	}, []string{"service", "method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "auralis_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"service", "method", "route"})

	httpErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auralis_http_errors_total",
		Help: "HTTP responses with status >= 500.",
	}, []string{"service", "route"})

	dbDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "auralis_db_query_duration_seconds",
		Help:    "Database query latency in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"service", "operation"})

	redisDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "auralis_redis_command_duration_seconds",
		Help:    "Redis command latency in seconds.",
		Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.05, 0.1},
	}, []string{"service", "command"})

	kafkaProduce = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "auralis_kafka_produce_duration_seconds",
		Help:    "Kafka producer send latency in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2},
	}, []string{"service", "topic"})

	kafkaConsumerLag = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "auralis_kafka_consumer_lag_messages",
		Help: "Estimated consumer lag in messages, by service, topic, partition.",
	}, []string{"service", "topic", "partition"})

	workerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "auralis_worker_job_duration_seconds",
		Help:    "Background worker job duration in seconds.",
		Buckets: []float64{0.05, 0.1, 0.5, 1, 5, 15, 30, 60, 120, 300},
	}, []string{"service", "job"})

	businessCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auralis_events_total",
		Help: "Domain counters such as playback authorization failures and search queries.",
	}, []string{"service", "name", "outcome"})
)

func statusClass(code int) string {
	switch {
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

// ObserveHTTP records one handled HTTP request.
func ObserveHTTP(service, method, route string, status int, d time.Duration) {
	httpRequests.WithLabelValues(service, method, route, statusClass(status)).Inc()
	httpDuration.WithLabelValues(service, method, route).Observe(d.Seconds())
	if status >= 500 {
		httpErrors.WithLabelValues(service, route).Inc()
	}
}

// ObserveDB records a database query.
func ObserveDB(service, operation string, d time.Duration) {
	dbDuration.WithLabelValues(service, operation).Observe(d.Seconds())
}

// ObserveRedis records a Redis command.
func ObserveRedis(service, command string, d time.Duration) {
	redisDuration.WithLabelValues(service, command).Observe(d.Seconds())
}

// ObserveKafkaProduce records a Kafka send.
func ObserveKafkaProduce(service, topic string, d time.Duration) {
	kafkaProduce.WithLabelValues(service, topic).Observe(d.Seconds())
}

// SetConsumerLag publishes the current consumer lag for a partition.
func SetConsumerLag(service, topic string, partition int, lag int64) {
	kafkaConsumerLag.WithLabelValues(service, topic, strconv.Itoa(partition)).Set(float64(lag))
}

// ObserveWorker records a background job's duration.
func ObserveWorker(service, job string, d time.Duration) {
	workerDuration.WithLabelValues(service, job).Observe(d.Seconds())
}

// Count increments a named domain counter with an outcome label
// ("success", "failure", "denied", ...).
func Count(service, name, outcome string) {
	businessCounter.WithLabelValues(service, name, outcome).Inc()
}

// Timer returns a function that, when called, records the elapsed time via record.
func Timer(record func(time.Duration)) func() {
	start := time.Now()
	return func() { record(time.Since(start)) }
}

// SetupTracing configures the global OTel tracer provider. When otlpEndpoint is
// empty, tracing is disabled and a no-op provider is used so services never
// depend on a collector being present. Returns a shutdown function.
func SetupTracing(ctx context.Context, service, version, otlpEndpoint string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	if otlpEndpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(service),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Tracer returns a named tracer from the global provider.
func Tracer(name string) trace.Tracer { return otel.Tracer(name) }

// Attr is a convenience re-export for building span attributes.
func Attr(key, value string) attribute.KeyValue { return attribute.String(key, value) }
