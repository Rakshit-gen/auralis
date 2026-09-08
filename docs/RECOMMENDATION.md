# Recommendation

The recommendation service produces the personalized feed, "similar shows",
trending, and popular. It holds no authoritative state: every table is a
projection rebuilt from `auralis.content.events`, `auralis.playback.events`,
and `auralis.user.events`.

## Model

The ranking function is a transparent weighted sum of normalized features. No
black box, no trained weights checked into the repo; the weights are constants
and can be overridden with the `RECO_WEIGHTS` environment variable (JSON).

### Features

| Feature | Meaning |
| --- | --- |
| `genre_affinity` | how much the user listens to this show's genres, normalized |
| `language_affinity` | user's listening share for the show's language |
| `collaborative` | co-listen score: users who played the user's shows also played this |
| `popularity` | normalized play count |
| `trending` | recent play velocity from `reco_show_signals` |
| `recency` | `exp(-days_since_publish / 43.28)`, a 30-day half life |
| `completion_quality` | average completion ratio the show earns |
| `preference_match` | show matches an explicit genre or language preference |
| `already_started_penalty` | negative, applied to shows the user began |
| `ai_generated_bonus` | small nudge for AI-generated shows |

### Default weights

```
genre_affinity 1.6   language_affinity 0.8   collaborative 1.4
popularity 0.7       trending 0.9            recency 0.6
completion_quality 0.5   preference_match 1.0
already_started_penalty -2.5   ai_generated_bonus 0.1
```

Each candidate's feature vector is min-max normalized across the candidate set,
multiplied by the weights, and summed. The `score` and the individual feature
contributions are returned in the API response so a client (or a reviewer) can
see why a show was ranked where it was.

### Diversity re-rank

After scoring, a greedy pass builds the final list: at each step it picks the
highest-scoring remaining candidate after subtracting `diversity *
count_of_that_primary_genre_already_chosen`. `diversity` defaults to 0.35
(`RECO_DIVERSITY`). This stops the feed from being ten thrillers.

### Cold start

A user with no genre affinity and no started shows gets
`strategy: "cold_start_popular_and_recent"`: popularity, trending, recency, and
any explicit preferences carry the ranking. Once they listen, the personalized
signals take over.

## Endpoints

| Endpoint | Auth | Returns |
| --- | --- | --- |
| `GET /api/recommendations/feed` | user | personalized feed |
| `GET /api/recommendations/similar/{showId}` | public | content + co-listen similar shows |
| `GET /api/recommendations/trending` | public | ranked by `trending_score` |
| `GET /api/recommendations/popular` | public | ranked by play count |

`similar` blends content similarity (shared genres and tags), co-listen score,
and a small popularity term (`0.6 / 0.3 / 0.1`).

## Projections

- `reco_shows`: published shows with genre ids, tags, language, premium and
  AI flags.
- `reco_show_signals`: plays, completes, trending score per show.
- `reco_user_genre_affinity`, `reco_user_language_affinity`: per-user weighted
  interaction counts.
- `reco_user_show_affinity`: per-user, per-show score and a `completed` flag,
  updated from playback progress, completion, and skip events.
- `reco_co_play`: show-to-show co-listen counts, the collaborative signal.
- `reco_user_prefs`: mirror of the user's explicit genre and language
  preferences.

## Offline evaluation

`python -m auralis_reco evaluate --k 10` (or `make reco-eval`) runs a
leave-last-out split: for each user with at least `--min-interactions` (default
3) recorded show interactions, the most recent one is held out, the feed is
computed from the rest with that show masked from the user's history, and
precision, recall, and NDCG at k are measured along with catalog coverage and
mean intra-list diversity.

Latest run against the seed dataset (37 users):

| k | precision | recall | NDCG | catalog coverage | mean intra-list diversity |
| --- | --- | --- | --- | --- | --- |
| 5 | 0.1405 | 0.7027 | 0.5529 | 0.8438 | 0.0614 |
| 10 | 0.0838 | 0.8378 | 0.5937 | 0.9375 | 0.0627 |
| 20 | 0.0486 | 0.9730 | 0.6269 | 1.0000 | 0.0675 |

Reading these: precision is bounded low because there is exactly one held-out
relevant item per user (max precision at k=5 is 0.2). Recall rising to 0.97 by
k=20 and NDCG around 0.55 to 0.63 are the meaningful signals: the held-out show
the user actually engaged with usually lands high in the feed. Catalog coverage
reaching 1.0 means the feed is not collapsing onto a handful of popular shows.
Mean intra-list diversity is low (about 0.06) because the normalized feature
vectors are dominated by a few features; this is a known limitation of the
current linear model, not a bug, and the diversity re-rank is what keeps the
visible genre mix reasonable despite it.

These numbers are produced by the harness and are not hand-tuned. Re-run after
changing weights or the seed data.
