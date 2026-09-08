"""Async SQLAlchemy engine and session helpers."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)


def make_engine(url: str, pool_size: int = 10) -> AsyncEngine:
    # Accept plain postgres:// URLs and route them through asyncpg.
    if url.startswith("postgres://"):
        url = url.replace("postgres://", "postgresql+asyncpg://", 1)
    elif url.startswith("postgresql://"):
        url = url.replace("postgresql://", "postgresql+asyncpg://", 1)
    # asyncpg does not accept libpq's sslmode query parameter.
    if "sslmode=" in url:
        url = _strip_query_param(url, "sslmode")
    return create_async_engine(url, pool_size=pool_size, max_overflow=5, pool_pre_ping=True, pool_recycle=1800)


def _strip_query_param(url: str, key: str) -> str:
    base, _, query = url.partition("?")
    kept = [p for p in query.split("&") if p and not p.startswith(f"{key}=")]
    return base + ("?" + "&".join(kept) if kept else "")


def session_factory(engine: AsyncEngine) -> async_sessionmaker[AsyncSession]:
    return async_sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)


@asynccontextmanager
async def session_scope(
    factory: async_sessionmaker[AsyncSession],
) -> AsyncIterator[AsyncSession]:
    async with factory() as session:
        try:
            yield session
            await session.commit()
        except Exception:
            await session.rollback()
            raise


async def ping(engine: AsyncEngine) -> None:
    from sqlalchemy import text

    async with engine.connect() as conn:
        await conn.execute(text("SELECT 1"))
