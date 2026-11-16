"""Who may start generation and read jobs."""

from __future__ import annotations

import os

import httpx
import pytest
from auralis_common.db import make_engine, session_factory
from auralis_common.errors import ApiError, install_exception_handlers
from auralis_common.identity import Identity, require_identity
from fastapi import FastAPI, Request
from sqlalchemy import text

from auralis_ai_media.api import build_router
from auralis_ai_media.providers.local_llm import LocalLLMProvider
from auralis_ai_media.repo import Repo

OWNER = "11111111-1111-1111-1111-111111111111"
OTHER = "33333333-3333-3333-3333-333333333333"
SHOW = "44444444-4444-4444-4444-444444444444"


@pytest.fixture
async def client():
    url = os.environ.get("AI_MEDIA_TEST_DATABASE_URL", "postgresql://localhost:5432/ai_media_test")
    eng = make_engine(url, pool_size=4)
    from auralis_ai_media.models import Base

    try:
        async with eng.begin() as conn:
            await conn.run_sync(Base.metadata.create_all)
            await conn.execute(text("TRUNCATE job_events, generation_jobs, series_bibles CASCADE"))
    except Exception as exc:
        pytest.skip(f"ai_media_test database not reachable ({exc})")
    sm = session_factory(eng)

    # A series job by OWNER that created SHOW, with its bible.
    bible = await LocalLLMProvider().generate_bible("a lighthouse keeper who hears the drowned", 6, seed=1)
    async with sm() as s:
        repo = Repo(s)
        await repo.create_job(kind="series", status="completed", requested_by=OWNER, show_id=SHOW, prompt={})
        await repo.save_bible(SHOW, bible)
        await s.commit()

    app = FastAPI()
    install_exception_handlers(app)
    app.include_router(build_router(sm))

    # Identity comes from test headers here instead of the signed gateway ones.
    def ident(request: Request) -> Identity:
        user = request.headers.get("x-test-user")
        if not user:
            raise ApiError.unauthorized()
        return Identity(user, request.headers.get("x-test-roles", "USER").split(","))

    app.dependency_overrides[require_identity] = ident
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://t") as c:
        yield c
    await eng.dispose()


def as_user(user: str, roles: str) -> dict[str, str]:
    return {"x-test-user": user, "x-test-roles": roles}


async def test_generation_needs_the_creator_role(client):
    r = await client.post("/generate/series", json={"brief": "a lighthouse keeper"}, headers=as_user(OWNER, "USER"))
    assert r.status_code == 403
    r = await client.post("/generate/series", json={"brief": "a lighthouse keeper"}, headers=as_user(OWNER, "CREATOR"))
    assert r.status_code == 202


async def test_episodes_only_for_your_own_show(client):
    body = {"show_id": SHOW, "number": 2}
    r = await client.post("/generate/episode", json=body, headers=as_user(OTHER, "CREATOR"))
    assert r.status_code == 404
    r = await client.post("/generate/episode", json=body, headers=as_user(OWNER, "CREATOR"))
    assert r.status_code == 202
    job_id = r.json()["job_id"]

    assert (await client.get(f"/generate/jobs/{job_id}", headers=as_user(OWNER, "CREATOR"))).status_code == 200
    assert (await client.get(f"/generate/jobs/{job_id}", headers=as_user(OTHER, "CREATOR"))).status_code == 404
    assert (await client.get(f"/generate/jobs/{job_id}", headers=as_user(OTHER, "ADMIN"))).status_code == 200
    assert (await client.get("/generate/jobs/not-a-uuid", headers=as_user(OWNER, "CREATOR"))).status_code == 404
