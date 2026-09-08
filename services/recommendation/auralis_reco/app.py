"""Wire the recommendation service."""

from __future__ import annotations

import asyncio
import contextlib

import structlog
from auralis_common.db import make_engine, ping, session_factory
from auralis_common.kafka import TOPIC_CONTENT, TOPIC_PLAYBACK, TOPIC_USER, Consumer
from auralis_common.logging import setup_logging
from auralis_common.service import create_app
from auralis_common.telemetry import setup_tracing

from auralis_reco.api import build_router
from auralis_reco.consumer import CONSUMER_GROUP, Handler
from auralis_reco.recommend import Recommender
from auralis_reco.settings import Settings

log = structlog.get_logger()


def create_api():
    settings = Settings()
    setup_logging("recommendation", settings.log_level)
    setup_tracing("recommendation", "1.0.0", settings.otel_exporter_otlp_endpoint)

    engine = make_engine(settings.recommendation_database_url, pool_size=8)
    sm = session_factory(engine)
    recommender = Recommender(settings.reco_weights, settings.reco_feed_size, settings.reco_diversity)

    async def db_ready() -> None:
        await ping(engine)

    app = create_app("recommendation", "1.0.0", cors_origins=settings.cors_origins, readiness={"database": db_ready})
    app.include_router(build_router(sm, recommender))

    handler = Handler(sm)
    consumer = Consumer(
        settings.kafka_brokers,
        CONSUMER_GROUP,
        [TOPIC_CONTENT, TOPIC_PLAYBACK, TOPIC_USER],
        "recommendation",
        dedupe=handler,
    )
    tasks: list[asyncio.Task] = []

    @app.on_event("startup")
    async def _startup() -> None:
        if settings.recommendation_run_consumer:
            tasks.append(asyncio.create_task(consumer.run(handler.handle)))

    @app.on_event("shutdown")
    async def _shutdown() -> None:
        consumer.stop()
        for t in tasks:
            with contextlib.suppress(asyncio.CancelledError):
                t.cancel()

    return app
