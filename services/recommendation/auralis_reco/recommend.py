"""The recommendation engine: candidate generation, ranking, and diversity."""

from __future__ import annotations

import structlog

from auralis_reco.ranking import Candidate, Normalizer, load_weights, score
from auralis_reco.repo import Repo, candidate_from

log = structlog.get_logger()


class Recommender:
    def __init__(self, weights_json: str | None, feed_size: int = 30, diversity: float = 0.35):
        self.weights = load_weights(weights_json)
        self.feed_size = feed_size
        # 0 = pure relevance, 1 = maximum spread across genres.
        self.diversity = diversity

    async def feed(self, repo: Repo, user_id: str, size: int | None = None) -> dict:
        size = size or self.feed_size
        shows = await repo.published_shows(500)
        if not shows:
            return {"items": [], "strategy": "empty_catalog"}

        signals = await repo.signals_map([s.show_id for s in shows])
        genre_scores = await repo.user_genre_scores(user_id)
        lang_scores = await repo.user_language_scores(user_id)
        user_shows = await repo.user_show_scores(user_id)
        prefs = await repo.user_prefs(user_id)

        started_shows = list(user_shows.keys())
        collab = await repo.collaborative_scores(started_shows) if started_shows else {}

        cold_start = not genre_scores and not user_shows
        pref_genres = set(prefs.genre_slugs) if prefs else set()
        pref_langs = set(prefs.language_codes) if prefs else set()

        gmax = max(genre_scores.values(), default=1.0) or 1.0
        lmax = max(lang_scores.values(), default=1.0) or 1.0

        candidates: list[Candidate] = []
        for s in shows:
            _score_ent, completed = user_shows.get(s.show_id, (0.0, False))
            if completed:
                continue  # do not resurface a finished show
            c = candidate_from(s, signals.get(s.show_id))
            c.genre_affinity = _avg([genre_scores.get(g, 0.0) / gmax for g in s.genre_ids]) if s.genre_ids else 0.0
            c.language_affinity = (lang_scores.get(s.language_code, 0.0) / lmax) if lang_scores else 0.0
            c.collaborative = collab.get(s.show_id, 0.0)
            c.user_started = s.show_id in user_shows
            c.preference_match = _pref_match(s, pref_genres, pref_langs)
            candidates.append(c)

        norm = Normalizer.from_candidates(candidates)
        for c in candidates:
            score(c, norm, self.weights)
            c.sources = _sources(c, cold_start)

        candidates.sort(key=lambda x: x.score, reverse=True)
        ranked = self._diversify(candidates, size)

        return {
            "items": [_view(c) for c in ranked],
            "strategy": "cold_start_popular_and_recent" if cold_start else "personalized_hybrid",
            "weights": self.weights,
        }

    async def similar(self, repo: Repo, show_id: str, size: int = 12) -> dict:
        from auralis_reco import models

        show = await repo.s.get(models.Show, show_id)
        if show is None:
            return {"items": [], "reason": "unknown_show"}
        pairs = await repo.similar_by_content(show, size * 2)
        collab = await repo.collaborative_scores([show_id], size * 2)
        signals = await repo.signals_map([m.show_id for m, _ in pairs])

        out: list[tuple[dict, float]] = []
        for m, content_sim in pairs:
            cf = collab.get(m.show_id, 0.0)
            sig = signals.get(m.show_id)
            pop = (sig.plays if sig else 0)
            blended = 0.6 * content_sim + 0.3 * cf + 0.1 * min(1.0, pop / 500.0)
            out.append((_similar_view(m, content_sim, cf), blended))
        out.sort(key=lambda x: x[1], reverse=True)
        return {"items": [v for v, _ in out[:size]], "of_show": show_id}

    async def trending(self, repo: Repo, size: int = 20) -> dict:
        shows = await repo.published_shows(400)
        signals = await repo.signals_map([s.show_id for s in shows])
        ranked = sorted(
            shows,
            key=lambda s: (signals[s.show_id].trending_score if s.show_id in signals else 0.0),
            reverse=True,
        )[:size]
        return {"items": [_basic_view(s, signals.get(s.show_id)) for s in ranked]}

    async def popular(self, repo: Repo, size: int = 20) -> dict:
        shows = await repo.published_shows(400)
        signals = await repo.signals_map([s.show_id for s in shows])
        ranked = sorted(
            shows,
            key=lambda s: (signals[s.show_id].plays if s.show_id in signals else 0),
            reverse=True,
        )[:size]
        return {"items": [_basic_view(s, signals.get(s.show_id)) for s in ranked]}

    def _diversify(self, ranked: list[Candidate], size: int) -> list[Candidate]:
        """Greedy re-rank: penalise a candidate whose primary genre is already
        well represented, controlled by self.diversity."""
        if self.diversity <= 0:
            return ranked[:size]
        chosen: list[Candidate] = []
        genre_seen: dict[str, int] = {}
        pool = list(ranked)
        while pool and len(chosen) < size:
            best_i, best_val = 0, float("-inf")
            for i, c in enumerate(pool[:40]):
                primary = c.genre_ids[0] if c.genre_ids else "_"
                penalty = self.diversity * genre_seen.get(primary, 0)
                val = c.score - penalty
                if val > best_val:
                    best_i, best_val = i, val
            pick = pool.pop(best_i)
            primary = pick.genre_ids[0] if pick.genre_ids else "_"
            genre_seen[primary] = genre_seen.get(primary, 0) + 1
            chosen.append(pick)
        return chosen


def _avg(xs: list[float]) -> float:
    return sum(xs) / len(xs) if xs else 0.0


def _pref_match(show, pref_genres: set[str], pref_langs: set[str]) -> float:
    if not pref_genres and not pref_langs:
        return 0.0
    g = 1.0 if (pref_genres and set(show.tags) & pref_genres) else 0.0
    lang = 1.0 if (pref_langs and show.language_code in pref_langs) else 0.0
    return max(g, lang)


def _sources(c: Candidate, cold_start: bool) -> list[str]:
    s = []
    if c.genre_affinity > 0.2:
        s.append("genre_affinity")
    if c.collaborative > 0.2:
        s.append("collaborative")
    if c.trending_score > 0:
        s.append("trending")
    if c.preference_match > 0:
        s.append("your_preferences")
    if cold_start or not s:
        s.append("popular_and_recent")
    return s


def _view(c: Candidate) -> dict:
    return {
        "show_id": c.show_id,
        "title": c.title,
        "slug": c.slug,
        "score": round(c.score, 4),
        "features": {k: round(v, 4) for k, v in c.features.items()},
        "sources": c.sources,
        "is_premium": c.is_premium,
    }


def _basic_view(show, signal) -> dict:
    return {
        "show_id": show.show_id,
        "title": show.title,
        "slug": show.slug,
        "plays": signal.plays if signal else 0,
        "trending_score": round(signal.trending_score, 3) if signal else 0.0,
        "is_premium": show.is_premium,
    }


def _similar_view(show, content_sim: float, collab: float) -> dict:
    return {
        "show_id": show.show_id,
        "title": show.title,
        "slug": show.slug,
        "content_similarity": round(content_sim, 4),
        "co_listen_score": round(collab, 4),
        "is_premium": show.is_premium,
    }
