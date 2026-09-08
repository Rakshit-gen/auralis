"""Transparent, configurable ranking model.

A candidate show is scored as a weighted sum of normalized features. Weights come
from configuration (RECO_WEIGHTS as JSON, or the documented defaults), so the
model can be tuned or A/B tested without a code change. Nothing here returns a
hard-coded ranking.
"""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field
from datetime import UTC, datetime

DEFAULT_WEIGHTS: dict[str, float] = {
    "genre_affinity": 1.6,
    "language_affinity": 0.8,
    "collaborative": 1.4,
    "popularity": 0.7,
    "trending": 0.9,
    "recency": 0.6,
    "completion_quality": 0.5,
    "preference_match": 1.0,
    "already_started_penalty": -2.5,
    "ai_generated_bonus": 0.1,
}

FEATURE_NAMES = tuple(k for k in DEFAULT_WEIGHTS)


def load_weights(raw: str | None) -> dict[str, float]:
    weights = dict(DEFAULT_WEIGHTS)
    if raw:
        try:
            override = json.loads(raw)
            for k, v in override.items():
                if k in weights:
                    weights[k] = float(v)
        except (json.JSONDecodeError, TypeError, ValueError):
            pass
    return weights


@dataclass
class Candidate:
    show_id: str
    title: str
    slug: str
    language_code: str
    genre_ids: list[str]
    tags: list[str]
    is_premium: bool
    ai_generated: bool
    published_at: datetime | None
    # raw signals
    plays: int = 0
    completes: int = 0
    unique_listeners: int = 0
    trending_score: float = 0.0
    # per-user
    genre_affinity: float = 0.0
    language_affinity: float = 0.0
    collaborative: float = 0.0
    user_started: bool = False
    preference_match: float = 0.0
    features: dict[str, float] = field(default_factory=dict)
    score: float = 0.0
    sources: list[str] = field(default_factory=list)


def _recency(published_at: datetime | None) -> float:
    if published_at is None:
        return 0.0
    days = max(0.0, (datetime.now(UTC) - published_at).total_seconds() / 86400)
    # 30-day half life.
    return math.exp(-days / 43.28)


@dataclass
class Normalizer:
    max_plays: float = 1.0
    max_listeners: float = 1.0
    max_trending: float = 1.0

    @classmethod
    def from_candidates(cls, cands: list[Candidate]) -> Normalizer:
        return cls(
            max_plays=max((c.plays for c in cands), default=1) or 1,
            max_listeners=max((c.unique_listeners for c in cands), default=1) or 1,
            max_trending=max((c.trending_score for c in cands), default=1.0) or 1.0,
        )


def compute_features(c: Candidate, norm: Normalizer) -> dict[str, float]:
    completion = (c.completes / c.plays) if c.plays else 0.0
    return {
        "genre_affinity": _clip01(c.genre_affinity),
        "language_affinity": _clip01(c.language_affinity),
        "collaborative": _clip01(c.collaborative),
        "popularity": math.log1p(c.plays) / math.log1p(norm.max_plays) if norm.max_plays > 1 else 0.0,
        "trending": c.trending_score / norm.max_trending if norm.max_trending else 0.0,
        "recency": _recency(c.published_at),
        "completion_quality": _clip01(completion),
        "preference_match": _clip01(c.preference_match),
        "already_started_penalty": 1.0 if c.user_started else 0.0,
        "ai_generated_bonus": 1.0 if c.ai_generated else 0.0,
    }


def score(c: Candidate, norm: Normalizer, weights: dict[str, float]) -> float:
    c.features = compute_features(c, norm)
    c.score = sum(weights.get(name, 0.0) * value for name, value in c.features.items())
    return c.score


def _clip01(x: float) -> float:
    return 0.0 if x < 0 else (1.0 if x > 1 else x)
