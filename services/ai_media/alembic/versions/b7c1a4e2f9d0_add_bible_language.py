"""add language to series_bibles

Revision ID: b7c1a4e2f9d0
Revises: 09d19fb21634
Create Date: 2026-09-09 10:00:00.000000
"""

from __future__ import annotations

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "b7c1a4e2f9d0"
down_revision: str | None = "09d19fb21634"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "series_bibles",
        sa.Column("language", sa.String(length=8), nullable=False, server_default="en"),
    )


def downgrade() -> None:
    op.drop_column("series_bibles", "language")
