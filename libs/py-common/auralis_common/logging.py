"""Structured JSON logging with the platform's standard field set."""

from __future__ import annotations

import logging
import sys

import structlog
from structlog.contextvars import bind_contextvars, clear_contextvars

_STANDARD_FIELDS = (
    "timestamp",
    "service",
    "level",
    "request_id",
    "trace_id",
    "correlation_id",
)


def setup_logging(service: str, level: str = "info") -> None:
    lvl = getattr(logging, level.upper(), logging.INFO)
    logging.basicConfig(format="%(message)s", stream=sys.stdout, level=lvl)

    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso", key="timestamp"),
            _add_service(service),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.make_filtering_bound_logger(lvl),
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )


def _add_service(service: str):
    def _proc(_logger, _method, event_dict):
        event_dict.setdefault("service", service)
        return event_dict

    return _proc


def bind_request_context(
    *,
    request_id: str = "",
    correlation_id: str = "",
    trace_id: str = "",
    endpoint: str = "",
) -> None:
    clear_contextvars()
    ctx = {}
    if request_id:
        ctx["request_id"] = request_id
    if correlation_id:
        ctx["correlation_id"] = correlation_id
    if trace_id:
        ctx["trace_id"] = trace_id
    if endpoint:
        ctx["endpoint"] = endpoint
    if ctx:
        bind_contextvars(**ctx)
