"""Kafka consumer: build the catalog projection and per-user affinity from the
event stream."""

from __future__ import annotations

from datetime import UTC

import structlog
from auralis_common.envelope import Envelope
from sqlalchemy.ext.asyncio import async_sessionmaker

from auralis_reco.repo import Repo

log = structlog.get_logger()

# Bumped to force a full replay from the start of the log: the projection this
# consumer builds is disposable and rebuilds from content/playback/user events.
CONSUMER_GROUP = "recommendation-service-2"

# Interaction weights: how much each signal moves an affinity score.
W_PLAY = 0.5
W_COMPLETE = 1.5
W_LIKE = 2.0
W_BOOKMARK = 1.0
W_FOLLOW = 2.5
W_PROGRESS = 0.02  # per progress event, capped by the play weight over time


class Handler:
    def __init__(self, sm: async_sessionmaker):
        self._sm = sm

    async def already_processed(self, consumer: str, event_id: str) -> bool:
        # Read-only pre-check to skip the retry loop for known duplicates. The
        # authoritative claim happens in handle() in the same transaction as the
        # projection writes, so an event is never marked done with its effects
        # missing (crash between the two commits, or a dead-lettered event).
        async with self._sm() as session:
            return await Repo(session).event_seen(consumer, event_id)

    async def mark_processed(self, consumer: str, event_id: str) -> None:
        return None

    async def handle(self, env: Envelope) -> None:
        async with self._sm() as session:
            repo = Repo(session)
            if not await repo.mark_event(CONSUMER_GROUP, env.event_id):
                return  # already applied, or a concurrent delivery won the race
            await self._dispatch(repo, env.event_type, env.payload)
            await session.commit()

    async def _dispatch(self, repo: Repo, event_type: str, p: dict) -> None:
        if event_type == "content.show_published":
            await repo.upsert_show(
                show_id=p["show_id"],
                title=p.get("title", ""),
                slug=p.get("slug", ""),
                language_code=p.get("language_code", "en"),
                genre_ids=list(p.get("genre_ids", [])),
                tags=list(p.get("tags", [])),
                is_premium=bool(p.get("is_premium", False)),
                ai_generated=bool(p.get("ai_generated", False)),
                creator_id=p.get("creator_id", "") or "",
                published_at=_now(),
            )
            return

        if event_type == "content.episode_published":
            # Keep episode_count roughly current; a bump is enough for ranking.
            await repo.bump_signal(p["show_id"], plays=0)
            return

        if event_type in ("playback.play", "playback.progress", "playback.completed"):
            user_id, show_id = p.get("user_id"), p.get("show_id")
            if not user_id or not show_id:
                return
            completed = event_type == "playback.completed" or bool(p.get("completed"))
            if event_type == "playback.play":
                await repo.bump_signal(show_id, plays=1, unique_listeners=0)
                await repo.decay_and_bump_trending(show_id)
                await repo.bump_user_show(user_id, show_id, W_PLAY)
                await repo.bump_co_play(user_id, show_id)
                await self._propagate_genres(repo, show_id, user_id, W_PLAY)
            elif event_type == "playback.progress":
                await repo.bump_user_show(user_id, show_id, W_PROGRESS)
            if completed:
                await repo.bump_signal(show_id, completes=1)
                await repo.bump_user_show(user_id, show_id, W_COMPLETE, completed=True)
                await self._propagate_genres(repo, show_id, user_id, W_COMPLETE)
            return

        if event_type == "user.liked" and p.get("show_id"):
            await repo.bump_signal(p["show_id"], likes=1)
            await repo.bump_user_show(p["user_id"], p["show_id"], W_LIKE)
            await self._propagate_genres(repo, p["show_id"], p["user_id"], W_LIKE)
            return
        if event_type == "user.unliked" and p.get("show_id"):
            await repo.bump_signal(p["show_id"], likes=-1)
            return
        if event_type == "user.bookmarked" and p.get("show_id"):
            await repo.bump_signal(p["show_id"], bookmarks=1)
            await repo.bump_user_show(p["user_id"], p["show_id"], W_BOOKMARK)
            return
        if event_type == "user.followed" and p.get("show_id"):
            await repo.bump_signal(p["show_id"], follows=1)
            await repo.bump_user_show(p["user_id"], p["show_id"], W_FOLLOW)
            await self._propagate_genres(repo, p["show_id"], p["user_id"], W_FOLLOW)
            return
        if event_type == "user.unfollowed" and p.get("show_id"):
            await repo.bump_signal(p["show_id"], follows=-1)
            return

        if event_type == "user.preferences_changed":
            await repo.set_prefs(
                p["user_id"],
                genre_slugs=list(p.get("genre_slugs", [])),
                language_codes=list(p.get("language_codes", [])),
                explicit_ok=bool(p.get("explicit_ok", True)),
            )
            return
        if event_type == "user.entitlement_changed":
            prefs = await repo.user_prefs(p["user_id"])
            await repo.set_prefs(
                p["user_id"],
                genre_slugs=list(prefs.genre_slugs) if prefs else [],
                language_codes=list(prefs.language_codes) if prefs else [],
                premium=p.get("plan") == "premium",
            )
            return

    async def _propagate_genres(self, repo: Repo, show_id: str, user_id: str, delta: float) -> None:
        from auralis_reco import models

        show = await repo.s.get(models.Show, show_id)
        if show is None:
            return
        if show.genre_ids:
            await repo.bump_user_genres(user_id, list(show.genre_ids), delta)
        await repo.bump_user_language(user_id, show.language_code, delta)


def _now():
    from datetime import datetime

    return datetime.now(UTC)
