"""SQLAlchemy models for the recommendation service database.

Everything here is a projection built from Kafka events: the catalog it needs to
rank, aggregate item signals, and per-user affinity. It never reads another
service's database.
"""

from __future__ import annotations

from datetime import datetime

from sqlalchemy import ARRAY, BigInteger, Boolean, DateTime, Float, Integer, String, Text, func, text
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column


class Base(DeclarativeBase):
    pass


class Show(Base):
    __tablename__ = "reco_shows"

    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    title: Mapped[str] = mapped_column(Text, default="")
    slug: Mapped[str] = mapped_column(Text, default="")
    language_code: Mapped[str] = mapped_column(String(8), default="en", index=True)
    genre_ids: Mapped[list[str]] = mapped_column(ARRAY(String), default=list)
    tags: Mapped[list[str]] = mapped_column(ARRAY(String), default=list)
    is_premium: Mapped[bool] = mapped_column(Boolean, default=False)
    ai_generated: Mapped[bool] = mapped_column(Boolean, default=False)
    creator_id: Mapped[str] = mapped_column(String(64), default="")
    published_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True, index=True)
    episode_count: Mapped[int] = mapped_column(Integer, default=0)


class ShowSignal(Base):
    __tablename__ = "reco_show_signals"

    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    plays: Mapped[int] = mapped_column(BigInteger, default=0, server_default=text("0"))
    completes: Mapped[int] = mapped_column(BigInteger, default=0, server_default=text("0"))
    likes: Mapped[int] = mapped_column(BigInteger, default=0, server_default=text("0"))
    bookmarks: Mapped[int] = mapped_column(BigInteger, default=0, server_default=text("0"))
    follows: Mapped[int] = mapped_column(BigInteger, default=0, server_default=text("0"))
    unique_listeners: Mapped[int] = mapped_column(BigInteger, default=0, server_default=text("0"))
    # Exponentially decayed play count, refreshed on each play event.
    trending_score: Mapped[float] = mapped_column(Float, default=0.0, server_default=text("0"))
    last_event_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)


class UserShowAffinity(Base):
    __tablename__ = "reco_user_show_affinity"

    user_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    show_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    score: Mapped[float] = mapped_column(Float, default=0.0)
    completed: Mapped[bool] = mapped_column(Boolean, default=False)
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class UserGenreAffinity(Base):
    __tablename__ = "reco_user_genre_affinity"

    user_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    genre_id: Mapped[str] = mapped_column(String(64), primary_key=True)
    score: Mapped[float] = mapped_column(Float, default=0.0)


class UserLanguageAffinity(Base):
    __tablename__ = "reco_user_language_affinity"

    user_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    language_code: Mapped[str] = mapped_column(String(8), primary_key=True)
    score: Mapped[float] = mapped_column(Float, default=0.0)


class UserPrefs(Base):
    __tablename__ = "reco_user_prefs"

    user_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    genre_slugs: Mapped[list[str]] = mapped_column(ARRAY(String), default=list)
    language_codes: Mapped[list[str]] = mapped_column(ARRAY(String), default=list)
    explicit_ok: Mapped[bool] = mapped_column(Boolean, default=True)
    premium: Mapped[bool] = mapped_column(Boolean, default=False)


class CoPlay(Base):
    """Item-item co-occurrence for collaborative filtering: how often show B is
    played by users who also played show A."""

    __tablename__ = "reco_co_play"

    show_a: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    show_b: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    count: Mapped[int] = mapped_column(BigInteger, default=0)


class ProcessedEvent(Base):
    __tablename__ = "processed_events"

    consumer: Mapped[str] = mapped_column(String(64), primary_key=True)
    event_id: Mapped[str] = mapped_column(UUID(as_uuid=False), primary_key=True)
    handled_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
