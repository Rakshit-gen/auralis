"""Prometheus metrics and OpenTelemetry tracing setup for the Python services."""

from __future__ import annotations

import time
from contextlib import contextmanager

from prometheus_client import Counter, Histogram

HTTP_REQUESTS = Counter(
    "auralis_http_requests_total",
    "HTTP requests handled.",
    ["service", "method", "route", "status"],
)
HTTP_DURATION = Histogram(
    "auralis_http_request_duration_seconds",
    "HTTP request latency.",
    ["service", "method", "route"],
)
WORKER_DURATION = Histogram(
    "auralis_worker_job_duration_seconds",
    "Background worker job duration.",
    ["service", "job"],
    buckets=(0.05, 0.1, 0.5, 1, 5, 15, 30, 60, 120, 300, 600),
)
AI_DURATION = Histogram(
    "auralis_ai_generation_duration_seconds",
    "AI generation duration.",
    ["service", "stage", "provider"],
    buckets=(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120),
)
TTS_DURATION = Histogram(
    "auralis_tts_duration_seconds",
    "TTS synthesis duration.",
    ["service", "provider"],
    buckets=(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300),
)
FFMPEG_DURATION = Histogram(
    "auralis_ffmpeg_duration_seconds",
    "FFmpeg encode/package duration.",
    ["service", "operation"],
    buckets=(0.1, 0.5, 1, 5, 15, 30, 60, 120, 300),
)
EVENTS = Counter("auralis_events_total", "Domain counters.", ["service", "name", "outcome"])


def observe_http(service: str, method: str, route: str, status: int, seconds: float) -> None:
    cls = f"{status // 100}xx"
    HTTP_REQUESTS.labels(service, method, route, cls).inc()
    HTTP_DURATION.labels(service, method, route).observe(seconds)


def count(service: str, name: str, outcome: str) -> None:
    EVENTS.labels(service, name, outcome).inc()


@contextmanager
def timed(histogram: Histogram, *labels: str):
    start = time.perf_counter()
    try:
        yield
    finally:
        histogram.labels(*labels).observe(time.perf_counter() - start)


def setup_tracing(service: str, version: str, endpoint: str) -> None:
    """Configure the global tracer provider. A no-op when endpoint is empty so a
    service never depends on a collector being present."""
    if not endpoint:
        return
    from opentelemetry import trace
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor

    provider = TracerProvider(resource=Resource.create({"service.name": service, "service.version": version}))
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=f"{endpoint}/v1/traces")))
    trace.set_tracer_provider(provider)
