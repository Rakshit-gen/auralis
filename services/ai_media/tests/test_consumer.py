"""Upload consumer: the processed-event claim and the queued job commit together."""

from __future__ import annotations

import os

import pytest
from auralis_common.db import make_engine, session_factory
from auralis_common.envelope import new_envelope
from sqlalchemy import text
from sqlalchemy.ext.asyncio import async_sessionmaker

from auralis_ai_media.consumer import CONSUMER_GROUP, UploadConsumerHandler
from auralis_ai_media.repo import Repo


def db_url() -> str:
    return os.environ.get("AI_MEDIA_TEST_DATABASE_URL", "postgresql://localhost:5432/ai_media_test")


@pytest.fixture
async def sm() -> async_sessionmaker:
    engine = make_engine(db_url(), pool_size=4)
    from auralis_ai_media.models import Base

    try:
        async with engine.begin() as conn:
            await conn.run_sync(Base.metadata.create_all)
            await conn.execute(text("TRUNCATE job_events, generation_jobs, processed_events"))
    except Exception as exc:
        pytest.skip(f"ai_media_test database not reachable ({exc})")
    return session_factory(engine)


def upload_event(episode_id: str) -> object:
    return new_envelope(
        "content.audio_uploaded",
        {
            "show_id": "11111111-1111-1111-1111-111111111111",
            "episode_id": episode_id,
            "source_key": "uploads/raw.wav",
            "size_bytes": 10,
        },
        producer="test",
    )


@pytest.mark.asyncio
async def test_claim_and_job_commit_together(sm):
    handler = UploadConsumerHandler(sm)
    ep = "22222222-2222-2222-2222-222222222222"

    async def job_count() -> int:
        async with sm() as session:
            return (await session.execute(text("SELECT count(*) FROM generation_jobs"))).scalar_one()

    async def seen(eid: str) -> bool:
        async with sm() as session:
            return await Repo(session).event_seen(CONSUMER_GROUP, eid)

    # create_job blows up after the claim: neither the claim nor the job survives.
    ev = upload_event(ep)
    orig = Repo.create_job

    async def boom(self, **kw):
        raise RuntimeError("job insert failed")

    Repo.create_job = boom
    with pytest.raises(RuntimeError):
        await handler.handle(ev)
    Repo.create_job = orig

    assert await job_count() == 0
    assert not await seen(ev.event_id), "event marked processed despite a failed job insert"

    # Redelivery of the same event id now queues exactly one job.
    await handler.handle(ev)
    await handler.handle(ev)  # duplicate delivery
    assert await job_count() == 1
