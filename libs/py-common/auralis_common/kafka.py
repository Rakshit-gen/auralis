"""Kafka helpers: envelope-encoded producer and an at-least-once consumer with
idempotency, bounded retries, and dead-lettering."""

from __future__ import annotations

import asyncio
from collections.abc import Awaitable, Callable
from typing import Protocol

import structlog
from aiokafka import AIOKafkaConsumer, AIOKafkaProducer

from auralis_common.envelope import Envelope

log = structlog.get_logger()

TOPIC_USER = "auralis.user.events"
TOPIC_CONTENT = "auralis.content.events"
TOPIC_PLAYBACK = "auralis.playback.events"
TOPIC_AI = "auralis.ai.events"
TOPIC_MEDIA = "auralis.media.events"
DLQ_SUFFIX = ".dlq"


class Producer:
    def __init__(self, brokers: str, service: str):
        self._brokers = brokers
        self._service = service
        self._p: AIOKafkaProducer | None = None

    async def start(self) -> None:
        self._p = AIOKafkaProducer(bootstrap_servers=self._brokers, acks="all", enable_idempotence=True)
        await self._p.start()

    async def stop(self) -> None:
        if self._p:
            await self._p.stop()

    async def publish(self, topic: str, key: str, env: Envelope) -> None:
        assert self._p is not None, "producer not started"
        await self._p.send_and_wait(
            topic,
            value=env.to_bytes(),
            key=key.encode(),
            headers=[
                ("event_type", env.event_type.encode()),
                ("correlation_id", env.correlation_id.encode()),
            ],
        )


class Dedupe(Protocol):
    async def already_processed(self, consumer: str, event_id: str) -> bool: ...
    async def mark_processed(self, consumer: str, event_id: str) -> None: ...


Handler = Callable[[Envelope], Awaitable[None]]


class Consumer:
    """Consumer-group loop with idempotency, retry, and dead-lettering."""

    def __init__(
        self,
        brokers: str,
        group_id: str,
        topics: list[str],
        service: str,
        dedupe: Dedupe | None = None,
        max_retries: int = 5,
        retry_base: float = 0.5,
    ):
        self._brokers = brokers
        self._group = group_id
        self._topics = topics
        self._service = service
        self._dedupe = dedupe
        self._max_retries = max_retries
        self._retry_base = retry_base
        self._stop = asyncio.Event()

    def stop(self) -> None:
        self._stop.set()

    async def run(self, handle: Handler) -> None:
        consumer = AIOKafkaConsumer(
            *self._topics,
            bootstrap_servers=self._brokers,
            group_id=self._group,
            enable_auto_commit=False,
            auto_offset_reset="earliest",
        )
        dlq = AIOKafkaProducer(bootstrap_servers=self._brokers, acks="all")
        await consumer.start()
        await dlq.start()
        log.info("consumer started", group=self._group, topics=self._topics)
        try:
            while not self._stop.is_set():
                batch = await consumer.getmany(timeout_ms=1000, max_records=50)
                for tp, messages in batch.items():
                    for msg in messages:
                        await self._process(msg, handle, dlq)
                        await consumer.commit({tp: msg.offset + 1})
        finally:
            await consumer.stop()
            await dlq.stop()

    async def _process(self, msg, handle: Handler, dlq: AIOKafkaProducer) -> None:
        try:
            env = Envelope.from_bytes(msg.value)
        except Exception as exc:
            log.error("undecodable message dead-lettered", error=str(exc), topic=msg.topic)
            await self._dead_letter(dlq, msg, f"envelope_parse_error: {exc}")
            return

        if self._dedupe and await self._dedupe.already_processed(self._group, env.event_id):
            log.debug("duplicate event skipped", event_id=env.event_id)
            return

        last_exc: Exception | None = None
        for attempt in range(1, self._max_retries + 1):
            try:
                await handle(env)
                last_exc = None
                break
            except Exception as exc:
                last_exc = exc
                log.warning(
                    "handler failed",
                    attempt=attempt,
                    event_type=env.event_type,
                    error=str(exc),
                )
                if attempt < self._max_retries:
                    await asyncio.sleep(min(self._retry_base * 2 ** (attempt - 1), 30))

        if last_exc is not None:
            log.error(
                "event dead-lettered after retries",
                event_id=env.event_id,
                error=str(last_exc),
            )
            await self._dead_letter(dlq, msg, str(last_exc))
            return

        if self._dedupe:
            await self._dedupe.mark_processed(self._group, env.event_id)

    async def _dead_letter(self, dlq: AIOKafkaProducer, msg, reason: str) -> None:
        await dlq.send_and_wait(
            msg.topic + DLQ_SUFFIX,
            value=msg.value,
            key=msg.key,
            headers=[
                ("dlq_reason", reason.encode()[:400]),
                ("dlq_origin_topic", msg.topic.encode()),
            ],
        )
