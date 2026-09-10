"""Wire the ai-media service: API process and worker process share this module."""

from __future__ import annotations

import asyncio
import contextlib

import structlog
from auralis_common.db import make_engine, ping, session_factory
from auralis_common.kafka import TOPIC_CONTENT, Consumer
from auralis_common.logging import setup_logging
from auralis_common.service import create_app
from auralis_common.svcclient import ServiceClient
from auralis_common.telemetry import setup_tracing

from auralis_ai_media.api import build_router
from auralis_ai_media.consumer import CONSUMER_GROUP, UploadConsumerHandler
from auralis_ai_media.pipeline import Pipeline
from auralis_ai_media.providers import select_llm, select_tts
from auralis_ai_media.settings import Settings
from auralis_ai_media.storage import ObjectStore
from auralis_ai_media.worker import Worker

log = structlog.get_logger()


def _build(settings: Settings):
    engine = make_engine(settings.ai_media_database_url, pool_size=8)
    sm = session_factory(engine)
    store = ObjectStore(
        endpoint=settings.s3_endpoint,
        access_key=settings.s3_access_key,
        secret_key=settings.s3_secret_key,
        bucket=settings.s3_bucket,
        secure=settings.s3_use_ssl,
        region=settings.s3_region,
    )
    content = ServiceClient(settings.content_service_url, settings.service_shared_token, "ai-media")
    llm = select_llm(settings.ai_default_provider, settings.groq_api_key, settings.groq_model)
    tts = select_tts(settings.piper_voices_dir, settings.piper_bin)
    pipeline = Pipeline(llm, tts, store, content, settings.ai_media_work_dir)
    return engine, sm, store, pipeline


def create_api():
    settings = Settings()
    setup_logging("ai-media", settings.log_level)
    setup_tracing("ai-media", "1.0.0", settings.otel_exporter_otlp_endpoint)
    engine, sm, store, pipeline = _build(settings)

    async def db_ready() -> None:
        await ping(engine)

    async def store_ready() -> None:
        store.ping()

    app = create_app(
        "ai-media",
        "1.0.0",
        cors_origins=settings.cors_origins,
        readiness={"database": db_ready, "object_storage": store_ready},
    )
    app.include_router(build_router(sm, pipeline.llm))

    consumer = Consumer(
        settings.kafka_brokers,
        CONSUMER_GROUP,
        [TOPIC_CONTENT],
        "ai-media",
        dedupe=None,
    )
    handler = UploadConsumerHandler(sm)
    consumer._dedupe = handler

    tasks: list[asyncio.Task] = []

    @app.on_event("startup")
    async def _startup() -> None:
        tasks.append(asyncio.create_task(consumer.run(handler.handle)))
        if settings.ai_media_run_worker:
            worker = Worker(engine, pipeline, settings.ai_media_max_attempts)
            app.state.worker = worker
            tasks.append(asyncio.create_task(worker.run()))

    @app.on_event("shutdown")
    async def _shutdown() -> None:
        consumer.stop()
        if getattr(app.state, "worker", None):
            app.state.worker.stop()
        for t in tasks:
            with contextlib.suppress(asyncio.CancelledError):
                t.cancel()
        await content_aclose(pipeline)

    return app


async def content_aclose(pipeline: Pipeline) -> None:
    with contextlib.suppress(Exception):
        await pipeline.content.aclose()


async def run_worker_only() -> None:
    """Entrypoint for a standalone worker process."""
    settings = Settings()
    setup_logging("ai-media-worker", settings.log_level)
    setup_tracing("ai-media-worker", "1.0.0", settings.otel_exporter_otlp_endpoint)
    engine, _sm, _store, pipeline = _build(settings)
    worker = Worker(engine, pipeline, settings.ai_media_max_attempts)
    try:
        await worker.run()
    finally:
        await content_aclose(pipeline)
