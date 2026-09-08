"""SQLAlchemy models for the ai-media service database."""

from __future__ import annotations

import uuid
from datetime import datetime

from sqlalchemy import Boolean, DateTime, ForeignKey, Integer, String, Text, UniqueConstraint, func
from sqlalchemy.dialects.postgresql import JSONB, UUID
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column, relationship


class Base(DeclarativeBase):
    pass


def _uuid() -> str:
    return str(uuid.uuid4())


# Generation job lifecycle. The pipeline advances a job through these states and
# the same values are reported to the content service as processing states.
JOB_STATES = (
    "queued",
    "generating_bible",
    "generating_outline",
    "generating_script",
    "synthesizing",
    "assembling",
    "packaging",
    "completed",
    "failed",
)


class GenerationJob(Base):
    __tablename__ = "generation_jobs"

    id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True, default=_uuid)
    kind: Mapped[str] = mapped_column(String(20))  # "series" | "episode" | "media"
    status: Mapped[str] = mapped_column(String(32), default="queued", index=True)
    progress: Mapped[int] = mapped_column(Integer, default=0)  # 0..100
    show_id: Mapped[str | None] = mapped_column(UUID(as_uuid=False), nullable=True, index=True)
    episode_id: Mapped[str | None] = mapped_column(UUID(as_uuid=False), nullable=True, index=True)
    episode_number: Mapped[int | None] = mapped_column(Integer, nullable=True)
    requested_by: Mapped[str] = mapped_column(UUID(as_uuid=False))
    provider: Mapped[str] = mapped_column(String(40), default="local")
    prompt: Mapped[dict] = mapped_column(JSONB, default=dict)
    result: Mapped[dict] = mapped_column(JSONB, default=dict)
    error: Mapped[str] = mapped_column(Text, default="")
    attempts: Mapped[int] = mapped_column(Integer, default=0)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now()
    )
    completed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)

    events: Mapped[list[JobEvent]] = relationship(back_populates="job", cascade="all, delete-orphan")


class JobEvent(Base):
    __tablename__ = "job_events"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    job_id: Mapped[str] = mapped_column(ForeignKey("generation_jobs.id", ondelete="CASCADE"), index=True)
    status: Mapped[str] = mapped_column(String(32))
    note: Mapped[str] = mapped_column(Text, default="")
    at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())

    job: Mapped[GenerationJob] = relationship(back_populates="events")


class SeriesBible(Base):
    __tablename__ = "series_bibles"

    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    concept: Mapped[dict] = mapped_column(JSONB)
    arc: Mapped[dict] = mapped_column(JSONB)
    episode_count: Mapped[int] = mapped_column(Integer, default=8)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class BibleCharacter(Base):
    __tablename__ = "bible_characters"

    id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True, default=_uuid)
    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), index=True)
    name: Mapped[str] = mapped_column(String(80))
    role: Mapped[str] = mapped_column(String(60))
    description: Mapped[str] = mapped_column(Text)
    voice: Mapped[str] = mapped_column(String(40), default="narrator")
    motivation: Mapped[str] = mapped_column(Text, default="")
    __table_args__ = (UniqueConstraint("show_id", "name", name="uq_character_per_show"),)


class BibleWorldRule(Base):
    __tablename__ = "bible_world_rules"

    id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True, default=_uuid)
    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), index=True)
    title: Mapped[str] = mapped_column(String(100))
    detail: Mapped[str] = mapped_column(Text)


class BibleRelationship(Base):
    __tablename__ = "bible_relationships"

    id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True, default=_uuid)
    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), index=True)
    from_character: Mapped[str] = mapped_column(String(80))
    to_character: Mapped[str] = mapped_column(String(80))
    nature: Mapped[str] = mapped_column(String(120))


class EpisodeSummary(Base):
    __tablename__ = "episode_summaries"

    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    episode_number: Mapped[int] = mapped_column(Integer, primary_key=True)
    title: Mapped[str] = mapped_column(String(120), default="")
    summary: Mapped[str] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class PlotThreadRow(Base):
    __tablename__ = "plot_threads"

    id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True, default=_uuid)
    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), index=True)
    summary: Mapped[str] = mapped_column(Text)
    introduced_in: Mapped[int] = mapped_column(Integer)
    resolved: Mapped[bool] = mapped_column(Boolean, default=False)


class ProcessedEvent(Base):
    __tablename__ = "processed_events"

    consumer: Mapped[str] = mapped_column(String(64), primary_key=True)
    event_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    handled_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
