"""Alembic environment for the recommendation service."""

from __future__ import annotations

import os

from alembic import context
from sqlalchemy import engine_from_config, pool

from auralis_reco.models import Base

config = context.config

_url = os.environ.get("RECOMMENDATION_DATABASE_URL", "postgresql://localhost:5432/recommendation_dev")
for prefix in ("postgresql+asyncpg://", "postgres://", "postgresql://"):
    if _url.startswith(prefix):
        _url = "postgresql+psycopg://" + _url[len(prefix):]
        break
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
