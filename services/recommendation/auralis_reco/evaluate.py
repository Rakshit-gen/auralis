"""Offline evaluation of the recommender.

Runs a leave-last-out split over the recorded user/show interactions: for each
user with at least `min_interactions`, the most recent liked/followed/completed
show is held out, the recommender is asked for a feed built only from the
remaining signals, and standard ranking metrics are computed.

    python -m auralis_reco evaluate --k 10

Results are printed and never fabricated; if there is not enough data the script
says so and exits.
"""

from __future__ import annotations

import argparse
import asyncio
import math

import structlog
from auralis_common.db import make_engine, session_factory
from auralis_common.logging import setup_logging
from sqlalchemy import select, text

from auralis_reco import models
from auralis_reco.recommend import Recommender
from auralis_reco.repo import Repo
from auralis_reco.settings import Settings

log = structlog.get_logger()


def dcg(relevances: list[int]) -> float:
    return sum(rel / math.log2(i + 2) for i, rel in enumerate(relevances))


def ndcg_at_k(recommended: list[str], relevant: set[str], k: int) -> float:
    rels = [1 if sid in relevant else 0 for sid in recommended[:k]]
    ideal = sorted(rels, reverse=True)
    idcg = dcg(ideal)
    return dcg(rels) / idcg if idcg > 0 else 0.0


def precision_at_k(recommended: list[str], relevant: set[str], k: int) -> float:
    if k == 0:
        return 0.0
    hits = sum(1 for sid in recommended[:k] if sid in relevant)
    return hits / k


def recall_at_k(recommended: list[str], relevant: set[str], k: int) -> float:
    if not relevant:
        return 0.0
    hits = sum(1 for sid in recommended[:k] if sid in relevant)
    return hits / len(relevant)


async def run(k: int, min_interactions: int) -> dict:
    settings = Settings()
    engine = make_engine(settings.recommendation_database_url, pool_size=4)
    sm = session_factory(engine)
    recommender = Recommender(settings.reco_weights, feed_size=max(k * 3, 30), diversity=settings.reco_diversity)

    async with sm() as session:
        repo = Repo(session)
        total_shows = len((await session.execute(select(models.Show.show_id))).all())
        if total_shows == 0:
            return {"error": "no shows in the projection; run the consumer against a seeded stream first"}

        rows = (
            await session.execute(
                text(
                    """
                    SELECT user_id, array_agg(show_id ORDER BY updated_at) AS shows
                    FROM reco_user_show_affinity
                    WHERE score > 0
                    GROUP BY user_id
                    HAVING count(*) >= :m
                    """
                ),
                {"m": min_interactions},
            )
        ).all()

    if not rows:
        return {"error": f"no users with at least {min_interactions} interactions"}

    p_sum = r_sum = n_sum = 0.0
    covered: set[str] = set()
    diversity_sum = 0.0
    evaluated = 0

    for user_id, shows in rows:
        # array_agg returns driver UUID objects; the recommender deals in strings.
        user_id = str(user_id)
        shows = [str(s) for s in shows]
        held_out = {shows[-1]}
        # Leave-last-out: score the recommender with the held-out show masked from
        # the user's history, then check where it lands in the returned feed. The
        # feed already excludes the user's other started shows.
        async with sm() as session:
            repo = Repo(session)
            feed = await recommender.feed(repo, user_id, size=max(k * 3, 30), holdout=held_out)
        rec_ids = [item["show_id"] for item in feed["items"]]

        p_sum += precision_at_k(rec_ids, held_out, k)
        r_sum += recall_at_k(rec_ids, held_out, k)
        n_sum += ndcg_at_k(rec_ids, held_out, k)
        covered.update(rec_ids[:k])
        diversity_sum += _intra_list_diversity(feed["items"][:k])
        evaluated += 1

    return {
        "k": k,
        "users_evaluated": evaluated,
        "precision_at_k": round(p_sum / evaluated, 4),
        "recall_at_k": round(r_sum / evaluated, 4),
        "ndcg_at_k": round(n_sum / evaluated, 4),
        "catalog_coverage": round(len(covered) / total_shows, 4),
        "mean_intra_list_diversity": round(diversity_sum / evaluated, 4),
        "note": "leave-last-out split over recorded interactions",
    }


def _intra_list_diversity(items: list[dict]) -> float:
    """1 minus the average pairwise feature-vector cosine similarity."""
    vecs = [list(it.get("features", {}).values()) for it in items if it.get("features")]
    if len(vecs) < 2:
        return 0.0
    sims: list[float] = []
    for i in range(len(vecs)):
        for j in range(i + 1, len(vecs)):
            sims.append(_cosine(vecs[i], vecs[j]))
    return 1.0 - (sum(sims) / len(sims) if sims else 0.0)


def _cosine(a: list[float], b: list[float]) -> float:
    dot = sum(x * y for x, y in zip(a, b, strict=False))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(y * y for y in b))
    return dot / (na * nb) if na and nb else 0.0


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate the Auralis recommender offline.")
    parser.add_argument("--k", type=int, default=10)
    parser.add_argument("--min-interactions", type=int, default=3)
    args = parser.parse_args()

    setup_logging("recommendation-eval", "info")
    result = asyncio.run(run(args.k, args.min_interactions))
    import json

    print(json.dumps(result, indent=2))
    if "error" in result:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
