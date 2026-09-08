"""Entrypoints for the recommendation service.

    python -m auralis_reco            API server (also runs the Kafka consumer)
    python -m auralis_reco migrate    run Alembic migrations and exit
    python -m auralis_reco evaluate   run offline evaluation (Precision/Recall/NDCG@K, coverage, diversity)
"""

from __future__ import annotations

import subprocess
import sys


def main() -> None:
    arg = sys.argv[1] if len(sys.argv) > 1 else "api"

    if arg == "migrate":
        raise SystemExit(subprocess.call(["alembic", "upgrade", "head"]))

    if arg == "evaluate":
        sys.argv = [sys.argv[0], *sys.argv[2:]]
        from auralis_reco.evaluate import main as eval_main

        eval_main()
        return

    import uvicorn

    from auralis_reco.settings import Settings

    settings = Settings()
    uvicorn.run(
        "auralis_reco.app:create_api",
        factory=True,
        host="0.0.0.0",
        port=settings.port,
        log_config=None,
    )


if __name__ == "__main__":
    main()
