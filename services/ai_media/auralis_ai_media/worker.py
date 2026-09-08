"""Background worker: claims queued generation jobs and runs the pipeline.

Runs as a separate process (never in an HTTP handler). One job at a time per
worker; scale by running more worker replicas.
"""

from __future__ import annotations

import asyncio

import structlog
from auralis_common.db import session_factory
from auralis_common.telemetry import WORKER_DURATION, count, timed

from auralis_ai_media import models
from auralis_ai_media.pipeline import Pipeline
from auralis_ai_media.repo import Repo

log = structlog.get_logger()


class Worker:
    def __init__(self, engine, pipeline: Pipeline, max_attempts: int, poll_interval: float = 2.0):
        self._factory = session_factory(engine)
        self._pipeline = pipeline
        self._max_attempts = max_attempts
        self._poll = poll_interval
        self._stop = asyncio.Event()

    def stop(self) -> None:
        self._stop.set()

    async def run(self) -> None:
        log.info("ai-media worker started")
        while not self._stop.is_set():
            claimed = await self._run_one()
            if not claimed:
                try:
                    await asyncio.wait_for(self._stop.wait(), timeout=self._poll)
                except TimeoutError:
                    pass
        log.info("ai-media worker stopped")

    async def _run_one(self) -> bool:
        async with self._factory() as session:
            repo = Repo(session)
            job = await repo.next_queued_job()
            if job is None:
                await session.rollback()
                return False
            job.status = "generating_bible" if job.kind == "series" else "generating_script"
            job.attempts += 1
            job_id, kind, attempts = job.id, job.kind, job.attempts
            await session.commit()

        try:
            with timed(WORKER_DURATION, "ai-media", f"job_{kind}"):
                await self._execute(job_id, kind)
            count("ai-media", "job", "completed")
        except Exception as exc:
            log.error("job failed", job_id=job_id, kind=kind, attempts=attempts, error=str(exc), exc_info=exc)
            count("ai-media", "job", "failed")
            await self._record_failure(job_id, str(exc), attempts)
        return True

    async def _execute(self, job_id: str, kind: str) -> None:
        async with self._factory() as session:
            repo = Repo(session)
            job = await repo.get_job(job_id)
            if job is None:
                return
            if kind == "series":
                await self._pipeline.run_series(repo, job)
            elif kind == "episode":
                await self._pipeline.run_episode(repo, job)
            elif kind == "media":
                await self._pipeline.package_upload(repo, job)
            await session.commit()

    async def _record_failure(self, job_id: str, error: str, attempts: int) -> None:
        async with self._factory() as session:
            repo = Repo(session)
            job = await repo.get_job(job_id)
            if job is None:
                return
            if attempts < self._max_attempts:
                job.status = "queued"
                job.error = error[:2000]
                session.add(models.JobEvent(job_id=job_id, status="queued", note=f"retry after: {error[:400]}"))
            else:
                await repo.fail_job(job, error)
                if job.episode_id:
                    try:
                        await self._pipeline._patch_episode(
                            job_id, job.episode_id, {"processing": "failed", "processing_error": error[:400]}
                        )
                    except Exception:
                        pass
            await session.commit()
