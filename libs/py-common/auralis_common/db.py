"""Async SQLAlchemy engine and session helpers."""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import structlog
from sqlalchemy import text
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
    connect_args: dict[str, object] = {}
    # Behind a transaction pooler (PgBouncer: Neon's "-pooler" host, Supabase,
    # and anything passing pgbouncer=true) asyncpg's prepared-statement cache
    # collides across pooled backends. Disabling it and randomising statement
    # names keeps the driver pooler-safe at a small planning cost.
    if "-pooler." in url or "pgbouncer=true" in url:
        url = _strip_query_param(url, "pgbouncer")
        connect_args["statement_cache_size"] = 0
        connect_args["prepared_statement_name_func"] = _unique_statement_name
    return create_async_engine(
        url,
        pool_size=pool_size,
        max_overflow=5,
        pool_pre_ping=True,
        pool_recycle=1800,
        connect_args=connect_args,
    )


def _unique_statement_name() -> str:
    import uuid

    return f"__asyncpg_{uuid.uuid4().hex}__"


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
    async with engine.connect() as conn:
        await conn.execute(text("SELECT 1"))


# Same window the Go services keep their dedupe records for.
PRUNE_PROCESSED_EVENTS = "DELETE FROM processed_events WHERE handled_at < now() - interval '14 days'"


async def prune_once(engine: AsyncEngine, stmts: dict[str, str]) -> dict[str, int]:
    """Run each DELETE once and return rows removed per table. A failure is
    logged and the other statements still run."""
    log = structlog.get_logger().bind(component="prune")
    removed: dict[str, int] = {}
    for name, sql in stmts.items():
        try:
            async with engine.begin() as conn:
                removed[name] = (await conn.execute(text(sql))).rowcount
        except Exception as e:
            log.warning("prune failed", table=name, error=str(e))
            continue
        if removed[name]:
            log.info("pruned old rows", table=name, rows=removed[name])
    return removed


async def prune_forever(engine: AsyncEngine, stmts: dict[str, str], every_sec: float = 3600) -> None:
    """prune_once now and then every every_sec, for tables that only grow,
    like the processed_events dedupe ledger. Mirrors the Go db.Prune."""
    while True:
        await prune_once(engine, stmts)
        await asyncio.sleep(every_sec)
