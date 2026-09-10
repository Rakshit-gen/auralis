"""Worker retry accounting: attempts is counted once per run, not twice on the
final failure."""

from __future__ import annotations

import os

import pytest
from auralis_common.db import make_engine, session_factory
from sqlalchemy import text

from auralis_ai_media.repo import Repo
from auralis_ai_media.worker import Worker


def db_url() -> str:
    return os.environ.get("AI_MEDIA_TEST_DATABASE_URL", "postgresql://localhost:5432/ai_media_test")


@pytest.fixture
async def engine():
    eng = make_engine(db_url(), pool_size=4)
    from auralis_ai_media.models import Base

    try:
        async with eng.begin() as conn:
            await conn.run_sync(Base.metadata.create_all)
            await conn.execute(text("TRUNCATE job_events, generation_jobs, processed_events"))
    except Exception as exc:
        pytest.skip(f"ai_media_test database not reachable ({exc})")
    return eng


class AlwaysFailsPipeline:
    async def run_episode(self, repo, job):
        raise RuntimeError("generation exploded")


@pytest.mark.asyncio
async def test_attempts_counted_once_per_run(engine):
    sm = session_factory(engine)
    async with sm() as session:
        job = await Repo(session).create_job(
            kind="episode", status="queued", requested_by="00000000-0000-0000-0000-000000000000"
        )
        await session.commit()
        job_id = job.id

    worker = Worker(engine, AlwaysFailsPipeline(), max_attempts=2)
    await worker._run_one()  # attempt 1 -> requeued
    await worker._run_one()  # attempt 2 -> permanent failure

    async with sm() as session:
        job = await Repo(session).get_job(job_id)
        assert job.status == "failed"
        assert job.attempts == 2, f"expected 2 attempts, got {job.attempts}"
