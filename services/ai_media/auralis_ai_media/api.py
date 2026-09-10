"""HTTP API for the ai-media service. Generation requests are accepted and
queued; no generation, TTS, or FFmpeg work happens in a request handler."""

from __future__ import annotations

from auralis_common.errors import ApiError
from auralis_common.identity import Identity, require_identity, require_roles
from fastapi import APIRouter, Depends, Query
from pydantic import BaseModel, Field
from sqlalchemy.ext.asyncio import async_sessionmaker

from auralis_ai_media.providers.base import LLMProvider
from auralis_ai_media.repo import Repo


class SeriesRequest(BaseModel):
    brief: str = Field(min_length=10, max_length=2000)
    episode_count: int = Field(default=8, ge=3, le=24)
    language_code: str = Field(default="en", max_length=8)
    is_premium: bool = False
    seed: int | None = None


class EpisodeRequest(BaseModel):
    show_id: str
    number: int = Field(ge=1)
    title: str | None = Field(default=None, max_length=120)
    seed: int | None = None


def build_router(sessionmaker: async_sessionmaker, llm: LLMProvider) -> APIRouter:
    router = APIRouter()

    async def repo_dep():
        async with sessionmaker() as session:
            yield Repo(session), session

    @router.post("/generate/series", status_code=202)
    async def generate_series(
        body: SeriesRequest,
        identity: Identity = Depends(require_identity),
        rs=Depends(repo_dep),
    ):
        repo, session = rs
        job = await repo.create_job(
            kind="series",
            status="queued",
            requested_by=identity.user_id,
            prompt=body.model_dump(exclude_none=True),
        )
        await session.commit()
        return {"job_id": job.id, "status": job.status}

    @router.post("/generate/episode", status_code=202)
    async def generate_episode(
        body: EpisodeRequest,
        identity: Identity = Depends(require_identity),
        rs=Depends(repo_dep),
    ):
        repo, session = rs
        bible = await repo.load_bible(body.show_id)
        if bible is None:
            raise ApiError.not_found("no story bible exists for this show; generate a series first")
        outlines = await llm.generate_outlines(bible, body.seed or 0, bible.language)
        outline = next((o for o in outlines if o.number == body.number), None)
        if outline is None:
            raise ApiError.bad_request("episode number is outside the planned season")
        job = await repo.create_job(
            kind="episode",
            status="queued",
            requested_by=identity.user_id,
            show_id=body.show_id,
            episode_number=body.number,
            prompt={"outline": outline.model_dump(), "seed": body.seed or 0},
        )
        await session.commit()
        return {"job_id": job.id, "status": job.status}

    @router.get("/generate/jobs/{job_id}")
    async def job_status(job_id: str, _identity: Identity = Depends(require_identity), rs=Depends(repo_dep)):
        repo, _ = rs
        job = await repo.get_job(job_id)
        if job is None:
            raise ApiError.not_found("job not found")
        return _job_view(job)

    @router.get("/ai/jobs")
    async def list_jobs(
        _identity: Identity = Depends(require_roles("ADMIN")),
        rs=Depends(repo_dep),
        status: str | None = Query(default=None),
        limit: int = Query(default=50, ge=1, le=200),
    ):
        repo, _ = rs
        jobs = await repo.list_jobs(limit, status)
        return {"jobs": [_job_view(j) for j in jobs]}

    return router


def _job_view(job) -> dict:
    return {
        "id": job.id,
        "kind": job.kind,
        "status": job.status,
        "progress": job.progress,
        "show_id": job.show_id,
        "episode_id": job.episode_id,
        "episode_number": job.episode_number,
        "provider": job.provider,
        "error": job.error,
        "attempts": job.attempts,
        "result": job.result,
        "created_at": job.created_at.isoformat() if job.created_at else None,
        "completed_at": job.completed_at.isoformat() if job.completed_at else None,
        "events": [
            {"status": e.status, "note": e.note, "at": e.at.isoformat()} for e in sorted(job.events, key=lambda x: x.id)
        ]
        if job.events
        else [],
    }
