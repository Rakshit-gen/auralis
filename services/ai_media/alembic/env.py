"""Alembic environment for the ai-media service."""

from __future__ import annotations

import os

from alembic import context
from sqlalchemy import engine_from_config, pool

from auralis_ai_media.models import Base

config = context.config

_url = os.environ.get("AI_MEDIA_DATABASE_URL", "postgresql://localhost:5432/ai_media_dev")
for prefix in ("postgresql+asyncpg://", "postgres://", "postgresql://"):
    if _url.startswith(prefix):
        _url = "postgresql+psycopg://" + _url[len(prefix) :]
        break
# psycopg does not accept libpq's sslmode as a SQLAlchemy URL query; keep it,
# psycopg understands sslmode natively.
config.set_main_option("sqlalchemy.url", _url)

target_metadata = Base.metadata


def run_migrations_offline() -> None:
    context.configure(url=_url, target_metadata=target_metadata, literal_binds=True, compare_type=True)
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    connectable = engine_from_config(
        config.get_section(config.config_ini_section, {}),
        prefix="sqlalchemy.",
        poolclass=pool.NullPool,
    )
    with connectable.connect() as connection:
        context.configure(connection=connection, target_metadata=target_metadata, compare_type=True)
        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
