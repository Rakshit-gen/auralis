"""Entrypoints for the ai-media service.

python -m auralis_ai_media           API server (also runs the worker unless AI_MEDIA_RUN_WORKER=false)
python -m auralis_ai_media worker    worker process only
python -m auralis_ai_media migrate   run Alembic migrations and exit
"""

from __future__ import annotations

import asyncio
import subprocess
import sys


def main() -> None:
    arg = sys.argv[1] if len(sys.argv) > 1 else "api"

    if arg == "migrate":
        raise SystemExit(subprocess.call(["alembic", "upgrade", "head"]))

    if arg == "worker":
        from auralis_ai_media.app import run_worker_only

        asyncio.run(run_worker_only())
        return

    import uvicorn

    from auralis_ai_media.settings import Settings

    settings = Settings()
    uvicorn.run(
        "auralis_ai_media.app:create_api",
        factory=True,
        host="0.0.0.0",
        port=settings.port,
        log_config=None,
    )


if __name__ == "__main__":
    main()
