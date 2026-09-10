"""Recommendation engine and consumer, against a real Postgres projection."""

from __future__ import annotations

import os
import uuid

import pytest
from auralis_common.db import make_engine, session_factory
from auralis_common.envelope import new_envelope
from sqlalchemy import text
from sqlalchemy.ext.asyncio import async_sessionmaker

from auralis_reco.consumer import Handler
from auralis_reco.evaluate import ndcg_at_k, precision_at_k
from auralis_reco.recommend import Recommender
from auralis_reco.repo import Repo


def db_url() -> str:
    return os.environ.get("RECOMMENDATION_TEST_DATABASE_URL", "postgresql://localhost:5432/recommendation_test")


@pytest.fixture
async def sm() -> async_sessionmaker:
    engine = make_engine(db_url(), pool_size=4)
    from auralis_reco.models import Base

    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
        for t in (
            "reco_co_play",
            "reco_user_show_affinity",
            "reco_user_genre_affinity",
            "reco_user_language_affinity",
            "reco_user_prefs",
            "reco_show_signals",
            "reco_shows",
            "processed_events",
        ):
            await conn.execute(text(f"TRUNCATE {t}"))
    return session_factory(engine)


def ev(event_type: str, payload: dict):
    return new_envelope(event_type, payload, producer="test")


async def publish_show(handler: Handler, show_id: str, genres: list[str], lang: str = "en", premium: bool = False):
    await handler.handle(
        ev(
            "content.show_published",
            {
                "show_id": show_id,
                "title": f"Show {show_id[:4]}",
                "slug": show_id[:8],
                "language_code": lang,
                "genre_ids": genres,
                "tags": genres,
                "is_premium": premium,
            },
        )
    )


@pytest.mark.asyncio
async def test_feed_personalises_from_activity(sm):
    handler = Handler(sm)
    mystery = "genre-mystery"
    scifi = "genre-scifi"
    romance = "genre-romance"

    mystery_shows = [str(uuid.uuid4()) for _ in range(4)]
    scifi_shows = [str(uuid.uuid4()) for _ in range(4)]
    romance_shows = [str(uuid.uuid4()) for _ in range(4)]
    for s in mystery_shows:
        await publish_show(handler, s, [mystery])
    for s in scifi_shows:
        await publish_show(handler, s, [scifi])
    for s in romance_shows:
        await publish_show(handler, s, [romance])

    user = str(uuid.uuid4())
    # The user plays and completes mystery shows.
    for s in mystery_shows[:3]:
        await handler.handle(ev("playback.play", {"user_id": user, "show_id": s}))
        await handler.handle(ev("playback.completed", {"user_id": user, "show_id": s, "completed": True}))

    recommender = Recommender(None, feed_size=12, diversity=0.0)
    async with sm() as session:
        feed = await recommender.feed(Repo(session), user)

    assert feed["strategy"] == "personalized_hybrid"
    top_ids = [i["show_id"] for i in feed["items"][:5]]
    # A mystery show the user has not finished should outrank romance/scifi.
    unfinished_mystery = mystery_shows[3]
    assert unfinished_mystery in top_ids, feed["items"][:5]
    # Completed shows are not resurfaced.
    assert not any(i["show_id"] in mystery_shows[:3] for i in feed["items"])
    # Genre affinity is a real, non-zero feature for the surfaced mystery show.
    surfaced = next(i for i in feed["items"] if i["show_id"] == unfinished_mystery)
    assert surfaced["features"]["genre_affinity"] > 0
    assert "genre_affinity" in surfaced["sources"]


@pytest.mark.asyncio
async def test_similar_uses_content_and_co_listen(sm):
    handler = Handler(sm)
    a, b, c = (str(uuid.uuid4()) for _ in range(3))
    await publish_show(handler, a, ["genre-noir", "detective"])
    await publish_show(handler, b, ["genre-noir", "detective"])  # very similar to a
    await publish_show(handler, c, ["genre-comedy"])  # unrelated

    # Two users who play a also play b: co-listen signal.
    for _ in range(2):
        u = str(uuid.uuid4())
        await handler.handle(ev("playback.play", {"user_id": u, "show_id": a}))
        await handler.handle(ev("playback.play", {"user_id": u, "show_id": b}))

    recommender = Recommender(None)
    async with sm() as session:
        result = await recommender.similar(Repo(session), a, size=5)

    ids = [i["show_id"] for i in result["items"]]
    assert ids and ids[0] == b
    assert result["items"][0]["co_listen_score"] > 0
    assert c not in ids[:1]


@pytest.mark.asyncio
async def test_dedupe_is_atomic_with_projection_writes(sm):
    handler = Handler(sm)
    show = str(uuid.uuid4())
    user = str(uuid.uuid4())
    await publish_show(handler, show, ["genre-x"])

    async def signal_plays() -> int:
        async with sm() as session:
            row = await session.execute(text("SELECT plays FROM reco_show_signals WHERE show_id = :s"), {"s": show})
            return row.scalar_one()

    async def event_seen(eid: str) -> bool:
        async with sm() as session:
            return await Repo(session).event_seen("recommendation-service-2", eid)

    # A handler that raises after a partial write must roll back both the
    # dedupe claim and the write.
    broken = ev("playback.play", {"user_id": user, "show_id": show})
    orig = handler._dispatch

    async def boom(repo, et, p):
        await repo.bump_signal(show, plays=1)
        raise RuntimeError("handler blew up after a partial write")

    handler._dispatch = boom
    with pytest.raises(RuntimeError):
        await handler.handle(broken)
    handler._dispatch = orig

    assert await signal_plays() == 0, "partial write was not rolled back"
    assert not await event_seen(broken.event_id), "event marked processed despite failure"

    # Reprocessing the same event id now succeeds and applies exactly once.
    good = ev("playback.play", {"user_id": user, "show_id": show})
    good.event_id = broken.event_id
    await handler.handle(good)
    await handler.handle(good)  # duplicate delivery
    assert await signal_plays() == 1


def test_metric_math():
    recommended = ["x", "a", "y", "b"]
    relevant = {"a", "b"}
    assert precision_at_k(recommended, relevant, 4) == 0.5
    assert round(ndcg_at_k(recommended, relevant, 4), 3) == round((1 / 1.585 + 1 / 2.322) / (1 / 1.0 + 1 / 1.585), 3)
