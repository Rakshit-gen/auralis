"""Kafka consumer: a creator upload (content.audio_uploaded) becomes a media
packaging job."""

from __future__ import annotations

import structlog
from auralis_common.envelope import Envelope
from sqlalchemy.ext.asyncio import async_sessionmaker

from auralis_ai_media.repo import Repo

log = structlog.get_logger()

CONSUMER_GROUP = "ai-media-service"


class UploadConsumerHandler:
    def __init__(self, sessionmaker: async_sessionmaker):
        self._sm = sessionmaker

    async def already_processed(self, consumer: str, event_id: str) -> bool:
        # Read-only pre-check only. The authoritative claim happens in handle()
        # in the same transaction as create_job, so a crash or a dead-lettered
        # event never leaves the upload marked processed with no job queued.
        async with self._sm() as session:
            return await Repo(session).event_seen(consumer, event_id)

    async def mark_processed(self, consumer: str, event_id: str) -> None:
        return None

    async def handle(self, env: Envelope) -> None:
        if env.event_type != "content.audio_uploaded":
            return
        p = env.payload
        async with self._sm() as session:
            repo = Repo(session)
            if not await repo.mark_event(CONSUMER_GROUP, env.event_id):
                return  # already queued, or a concurrent delivery won the race
            await repo.create_job(
                kind="media",
                status="queued",
                requested_by=p.get("requested_by", "00000000-0000-0000-0000-000000000000"),
                show_id=p["show_id"],
                episode_id=p["episode_id"],
                prompt={"source_key": p["source_key"], "size_bytes": p.get("size_bytes", 0)},
            )
            await session.commit()
        log.info("queued media job from upload", episode_id=p.get("episode_id"))
