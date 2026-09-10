"""Database access for the ai-media service."""

from __future__ import annotations

from datetime import UTC, datetime

from sqlalchemy import delete, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession

from auralis_ai_media import models
from auralis_ai_media.schemas import Character, SeriesConcept, StoryArc, StoryBible, WorldRule


class Repo:
    def __init__(self, session: AsyncSession):
        self.s = session

    # --- jobs ---

    async def create_job(self, **kw) -> models.GenerationJob:
        job = models.GenerationJob(**kw)
        self.s.add(job)
        await self.s.flush()
        self.s.add(models.JobEvent(job_id=job.id, status=job.status, note="created"))
        return job

    async def get_job(self, job_id: str) -> models.GenerationJob | None:
        return await self.s.get(models.GenerationJob, job_id)

    async def next_queued_job(self) -> models.GenerationJob | None:
        stmt = (
            select(models.GenerationJob)
            .where(models.GenerationJob.status.in_(("queued",)))
            .order_by(models.GenerationJob.created_at)
            .limit(1)
            .with_for_update(skip_locked=True)
        )
        return (await self.s.execute(stmt)).scalar_one_or_none()

    async def list_jobs(self, limit: int, status: str | None) -> list[models.GenerationJob]:
        stmt = select(models.GenerationJob).order_by(models.GenerationJob.created_at.desc()).limit(limit)
        if status:
            stmt = stmt.where(models.GenerationJob.status == status)
        return list((await self.s.execute(stmt)).scalars())

    async def advance_job(self, job: models.GenerationJob, status: str, progress: int, note: str = "") -> None:
        job.status = status
        job.progress = progress
        if status in ("completed", "failed"):
            job.completed_at = datetime.now(UTC)
        self.s.add(models.JobEvent(job_id=job.id, status=status, note=note[:1000]))

    async def fail_job(self, job: models.GenerationJob, error: str) -> None:
        # attempts is bumped once per run by the worker when it claims the job;
        # don't count the final failure twice.
        job.error = error[:2000]
        await self.advance_job(job, "failed", job.progress, error)

    # --- bible ---

    async def save_bible(self, show_id: str, bible: StoryBible) -> None:
        await self.s.execute(delete(models.BibleCharacter).where(models.BibleCharacter.show_id == show_id))
        await self.s.execute(delete(models.BibleWorldRule).where(models.BibleWorldRule.show_id == show_id))
        await self.s.execute(delete(models.BibleRelationship).where(models.BibleRelationship.show_id == show_id))

        await self.s.execute(
            pg_insert(models.SeriesBible)
            .values(
                show_id=show_id,
                concept=bible.concept.model_dump(),
                arc=bible.arc.model_dump(),
                episode_count=bible.episode_count,
                language=bible.language,
            )
            .on_conflict_do_update(
                index_elements=[models.SeriesBible.show_id],
                set_={
                    "concept": bible.concept.model_dump(),
                    "arc": bible.arc.model_dump(),
                    "episode_count": bible.episode_count,
                    "language": bible.language,
                },
            )
        )
        for c in bible.characters:
            self.s.add(models.BibleCharacter(show_id=show_id, **c.model_dump()))
        for w in bible.world_rules:
            self.s.add(models.BibleWorldRule(show_id=show_id, **w.model_dump()))
        for rel in bible.relationships:
            self.s.add(models.BibleRelationship(show_id=show_id, **rel.model_dump()))
        for _i, thread in enumerate(_seed_threads(bible)):
            self.s.add(models.PlotThreadRow(show_id=show_id, summary=thread, introduced_in=1, resolved=False))

    async def load_bible(self, show_id: str) -> StoryBible | None:
        row = await self.s.get(models.SeriesBible, show_id)
        if row is None:
            return None
        chars = (
            await self.s.execute(select(models.BibleCharacter).where(models.BibleCharacter.show_id == show_id))
        ).scalars()
        rules = (
            await self.s.execute(select(models.BibleWorldRule).where(models.BibleWorldRule.show_id == show_id))
        ).scalars()
        return StoryBible(
            concept=SeriesConcept.model_validate(row.concept),
            characters=[
                Character(name=c.name, role=c.role, description=c.description, voice=c.voice, motivation=c.motivation)
                for c in chars
            ],
            world_rules=[WorldRule(title=w.title, detail=w.detail) for w in rules],
            arc=StoryArc.model_validate(row.arc),
            episode_count=row.episode_count,
            language=getattr(row, "language", "en") or "en",
        )

    async def recent_summaries(self, show_id: str, before_number: int, count: int = 2) -> list[str]:
        stmt = (
            select(models.EpisodeSummary.summary)
            .where(models.EpisodeSummary.show_id == show_id, models.EpisodeSummary.episode_number < before_number)
            .order_by(models.EpisodeSummary.episode_number.desc())
            .limit(count)
        )
        rows = list((await self.s.execute(stmt)).scalars())
        return list(reversed(rows))

    async def unresolved_threads(self, show_id: str) -> list[str]:
        stmt = select(models.PlotThreadRow.summary).where(
            models.PlotThreadRow.show_id == show_id, models.PlotThreadRow.resolved.is_(False)
        )
        return list((await self.s.execute(stmt)).scalars())

    async def save_episode_summary(self, show_id: str, number: int, title: str, summary: str) -> None:
        await self.s.execute(
            pg_insert(models.EpisodeSummary)
            .values(show_id=show_id, episode_number=number, title=title, summary=summary)
            .on_conflict_do_update(
                index_elements=[models.EpisodeSummary.show_id, models.EpisodeSummary.episode_number],
                set_={"summary": summary, "title": title},
            )
        )

    async def resolve_one_thread(self, show_id: str, number: int) -> None:
        # Mark the oldest unresolved thread resolved once we are past the midpoint.
        stmt = (
            select(models.PlotThreadRow)
            .where(models.PlotThreadRow.show_id == show_id, models.PlotThreadRow.resolved.is_(False))
            .order_by(models.PlotThreadRow.introduced_in)
            .limit(1)
        )
        thread = (await self.s.execute(stmt)).scalar_one_or_none()
        if thread is not None:
            thread.resolved = True

    # --- idempotency ---

    async def mark_event(self, consumer: str, event_id: str) -> bool:
        result = await self.s.execute(
            pg_insert(models.ProcessedEvent).values(consumer=consumer, event_id=event_id).on_conflict_do_nothing()
        )
        return result.rowcount > 0

    async def event_seen(self, consumer: str, event_id: str) -> bool:
        row = await self.s.execute(
            select(models.ProcessedEvent.event_id).where(
                models.ProcessedEvent.consumer == consumer,
                models.ProcessedEvent.event_id == event_id,
            )
        )
        return row.first() is not None


def _seed_threads(bible: StoryBible) -> list[str]:
    lead = bible.characters[0].name
    foil = bible.characters[1].name
    return [
        f"the true reason {foil} started running the engine",
        f"what {lead} promised before the series began",
        "who paid the first cost, and whether they knew",
    ]
