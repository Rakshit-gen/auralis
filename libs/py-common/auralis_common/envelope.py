"""Versioned Kafka event envelope, matching the Go platform definition."""

from __future__ import annotations

import uuid
from datetime import UTC, datetime
from typing import Any

from pydantic import BaseModel, Field


def _utcnow() -> datetime:
    return datetime.now(UTC)


class Envelope(BaseModel):
    """Wraps a domain event with routing and tracing metadata."""

    event_id: str = Field(default_factory=lambda: str(uuid.uuid4()))
    event_type: str
    event_version: int = 1
    occurred_at: datetime = Field(default_factory=_utcnow)
    producer: str
    correlation_id: str = Field(default_factory=lambda: str(uuid.uuid4()))
    causation_id: str = ""
    payload: dict[str, Any]

    def to_bytes(self) -> bytes:
        return self.model_dump_json().encode("utf-8")

    @classmethod
    def from_bytes(cls, raw: bytes) -> Envelope:
        return cls.model_validate_json(raw)


def new_envelope(
    event_type: str,
    payload: dict[str, Any],
    *,
    producer: str,
    version: int = 1,
    correlation_id: str | None = None,
    causation_id: str = "",
) -> Envelope:
    return Envelope(
        event_type=event_type,
        event_version=version,
        producer=producer,
        correlation_id=correlation_id or str(uuid.uuid4()),
        causation_id=causation_id,
        payload=payload,
    )
