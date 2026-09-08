"""Database access for the recommendation service."""

from __future__ import annotations

from datetime import UTC, datetime

from sqlalchemy import select, text
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession

from auralis_reco import models
from auralis_reco.ranking import Candidate


class Repo:
    def __init__(self, session: AsyncSession):
        self.s = session

    # --- catalog projection ---

    async def upsert_show(self, **kw) -> None:
        await self.s.execute(
            pg_insert(models.Show)
            .values(**kw)
            .on_conflict_do_update(index_elements=[models.Show.show_id], set_=kw)
        )
        await self.s.execute(
            pg_insert(models.ShowSignal)
            .values(show_id=kw["show_id"])
            .on_conflict_do_nothing()
        )

    async def bump_signal(self, show_id: str, **deltas) -> None:
        cols = ", ".join(f"{k} = reco_show_signals.{k} + :{k}" for k in deltas)
        await self.s.execute(
            text("INSERT INTO reco_show_signals (show_id) VALUES (:sid) ON CONFLICT (show_id) DO NOTHING"),
            {"sid": show_id},
        )
        await self.s.execute(
            text(f"UPDATE reco_show_signals SET {cols}, last_event_at = now() WHERE show_id = :sid"),
            {"sid": show_id, **deltas},
        )

    async def decay_and_bump_trending(self, show_id: str, half_life_hours: float = 36.0) -> None:
        await self.s.execute(
            text(
                """
                INSERT INTO reco_show_signals (show_id, trending_score, last_event_at)
                VALUES (:sid, 1.0, now())
                ON CONFLICT (show_id) DO UPDATE SET
                    trending_score = reco_show_signals.trending_score
                        * power(0.5, EXTRACT(EPOCH FROM (now() - coalesce(reco_show_signals.last_event_at, now())))
                                / (:hl * 3600.0))
                        + 1.0,
                    last_event_at = now()
                """
            ),
            {"sid": show_id, "hl": half_life_hours},
        )

    # --- user affinity ---

    async def bump_user_show(self, user_id: str, show_id: str, delta: float, completed: bool = False) -> None:
        await self.s.execute(
            pg_insert(models.UserShowAffinity)
            .values(user_id=user_id, show_id=show_id, score=delta, completed=completed, updated_at=datetime.now(UTC))
            .on_conflict_do_update(
                index_elements=[models.UserShowAffinity.user_id, models.UserShowAffinity.show_id],
                set_={
                    "score": models.UserShowAffinity.score + delta,
                    "completed": models.UserShowAffinity.completed | completed,
                    "updated_at": datetime.now(UTC),
                },
            )
        )

    async def bump_user_genres(self, user_id: str, genre_ids: list[str], delta: float) -> None:
        for gid in genre_ids:
            await self.s.execute(
                pg_insert(models.UserGenreAffinity)
                .values(user_id=user_id, genre_id=gid, score=delta)
                .on_conflict_do_update(
                    index_elements=[models.UserGenreAffinity.user_id, models.UserGenreAffinity.genre_id],
                    set_={"score": models.UserGenreAffinity.score + delta},
                )
            )

    async def bump_user_language(self, user_id: str, language_code: str, delta: float) -> None:
        await self.s.execute(
            pg_insert(models.UserLanguageAffinity)
            .values(user_id=user_id, language_code=language_code, score=delta)
            .on_conflict_do_update(
                index_elements=[models.UserLanguageAffinity.user_id, models.UserLanguageAffinity.language_code],
                set_={"score": models.UserLanguageAffinity.score + delta},
            )
        )

    async def set_prefs(self, user_id: str, **kw) -> None:
        await self.s.execute(
            pg_insert(models.UserPrefs)
            .values(user_id=user_id, **kw)
            .on_conflict_do_update(index_elements=[models.UserPrefs.user_id], set_=kw)
        )

    async def bump_co_play(self, user_id: str, show_id: str) -> None:
        # Link this show to every other show the user has already played.
        await self.s.execute(
            text(
                """
                INSERT INTO reco_co_play (show_a, show_b, count)
                SELECT a.show_id, :sid, 1 FROM reco_user_show_affinity a
                WHERE a.user_id = :uid AND a.show_id <> :sid
                ON CONFLICT (show_a, show_b) DO UPDATE SET count = reco_co_play.count + 1
                """
            ),
            {"uid": user_id, "sid": show_id},
        )
        await self.s.execute(
            text(
                """
                INSERT INTO reco_co_play (show_a, show_b, count)
                SELECT :sid, a.show_id, 1 FROM reco_user_show_affinity a
                WHERE a.user_id = :uid AND a.show_id <> :sid
                ON CONFLICT (show_a, show_b) DO UPDATE SET count = reco_co_play.count + 1
                """
            ),
            {"uid": user_id, "sid": show_id},
        )

    # --- reads for ranking ---

    async def published_shows(self, limit: int = 500) -> list[models.Show]:
        stmt = (
            select(models.Show)
            .where(models.Show.published_at.is_not(None))
            .order_by(models.Show.published_at.desc())
            .limit(limit)
        )
        return list((await self.s.execute(stmt)).scalars())

    async def signals_map(self, show_ids: list[str]) -> dict[str, models.ShowSignal]:
        if not show_ids:
            return {}
        stmt = select(models.ShowSignal).where(models.ShowSignal.show_id.in_(show_ids))
        rows = (await self.s.execute(stmt)).scalars()
        return {r.show_id: r for r in rows}

    async def user_genre_scores(self, user_id: str) -> dict[str, float]:
        rows = (
            await self.s.execute(
                select(models.UserGenreAffinity.genre_id, models.UserGenreAffinity.score).where(
                    models.UserGenreAffinity.user_id == user_id
                )
            )
        ).all()
        return {gid: s for gid, s in rows}

    async def user_language_scores(self, user_id: str) -> dict[str, float]:
        rows = (
            await self.s.execute(
                select(models.UserLanguageAffinity.language_code, models.UserLanguageAffinity.score).where(
                    models.UserLanguageAffinity.user_id == user_id
                )
            )
        ).all()
        return {code: s for code, s in rows}

    async def user_show_scores(self, user_id: str) -> dict[str, tuple[float, bool]]:
        rows = (
            await self.s.execute(
                select(
                    models.UserShowAffinity.show_id,
                    models.UserShowAffinity.score,
                    models.UserShowAffinity.completed,
                ).where(models.UserShowAffinity.user_id == user_id)
            )
        ).all()
        return {sid: (score, completed) for sid, score, completed in rows}

    async def user_prefs(self, user_id: str) -> models.UserPrefs | None:
        return await self.s.get(models.UserPrefs, user_id)

    async def collaborative_scores(self, seed_show_ids: list[str], limit: int = 60) -> dict[str, float]:
        if not seed_show_ids:
            return {}
        rows = (
            await self.s.execute(
                text(
                    """
                    SELECT show_b, sum(count)::float AS c
                    FROM reco_co_play
                    WHERE show_a = ANY(:seeds)
                    GROUP BY show_b
                    ORDER BY c DESC
                    LIMIT :lim
                    """
                ),
                {"seeds": seed_show_ids, "lim": limit},
            )
        ).all()
        if not rows:
            return {}
        top = max(c for _sid, c in rows)
        # asyncpg returns UUID columns as uuid.UUID; keys are compared against
        # string show ids elsewhere, so normalise here.
        return {str(sid): c / top for sid, c in rows}

    async def similar_by_content(self, show: models.Show, limit: int) -> list[tuple[models.Show, float]]:
        candidates = await self.published_shows(400)
        scored: list[tuple[models.Show, float]] = []
        target_genres = set(show.genre_ids)
        target_tags = set(show.tags)
        for c in candidates:
            if c.show_id == show.show_id:
                continue
            g_overlap = len(target_genres & set(c.genre_ids)) / (len(target_genres | set(c.genre_ids)) or 1)
            t_overlap = len(target_tags & set(c.tags)) / (len(target_tags | set(c.tags)) or 1)
            lang = 1.0 if c.language_code == show.language_code else 0.0
            sim = 0.6 * g_overlap + 0.25 * t_overlap + 0.15 * lang
            if sim > 0:
                scored.append((c, sim))
        scored.sort(key=lambda x: x[1], reverse=True)
        return scored[:limit]

    # --- idempotency ---

    async def mark_event(self, consumer: str, event_id: str) -> bool:
        result = await self.s.execute(
            pg_insert(models.ProcessedEvent).values(consumer=consumer, event_id=event_id).on_conflict_do_nothing()
        )
        return result.rowcount > 0


def candidate_from(show: models.Show, signal: models.ShowSignal | None) -> Candidate:
    return Candidate(
        show_id=show.show_id,
        title=show.title,
        slug=show.slug,
        language_code=show.language_code,
        genre_ids=list(show.genre_ids or []),
        tags=list(show.tags or []),
        is_premium=show.is_premium,
        ai_generated=show.ai_generated,
        published_at=show.published_at,
        plays=signal.plays if signal else 0,
        completes=signal.completes if signal else 0,
        unique_listeners=signal.unique_listeners if signal else 0,
        trending_score=signal.trending_score if signal else 0.0,
    )
