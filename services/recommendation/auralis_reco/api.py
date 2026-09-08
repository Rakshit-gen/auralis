"""HTTP API for the recommendation service."""

from __future__ import annotations

from auralis_common.identity import Identity, require_identity
from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import async_sessionmaker

from auralis_reco.recommend import Recommender
from auralis_reco.repo import Repo


def build_router(sm: async_sessionmaker, recommender: Recommender) -> APIRouter:
    router = APIRouter()

    async def repo_dep():
        async with sm() as session:
            yield Repo(session)

    @router.get("/recommendations/feed")
    async def feed(
        identity: Identity = Depends(require_identity),
        repo: Repo = Depends(repo_dep),
        size: int = Query(default=30, ge=1, le=60),
    ):
        return await recommender.feed(repo, identity.user_id, size)

    @router.get("/recommendations/similar/{show_id}")
    async def similar(show_id: str, repo: Repo = Depends(repo_dep), size: int = Query(default=12, ge=1, le=30)):
        return await recommender.similar(repo, show_id, size)

    @router.get("/recommendations/trending")
    async def trending(repo: Repo = Depends(repo_dep), size: int = Query(default=20, ge=1, le=50)):
        return await recommender.trending(repo, size)

    @router.get("/recommendations/popular")
    async def popular(repo: Repo = Depends(repo_dep), size: int = Query(default=20, ge=1, le=50)):
        return await recommender.popular(repo, size)

    return router
