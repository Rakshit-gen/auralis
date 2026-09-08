# Service Boundaries

Each service owns a slice of the domain, its own database, and its own
deployable. This document states what each service is responsible for, what it
explicitly is not, and how other services get at data it owns.

The rule: a service reads and writes only its own database. If service A needs
data owned by service B, it either calls B's API synchronously or maintains a
local projection built from B's events.

## gateway

**Owns**: nothing. It is stateless.

**Responsible for**: terminating client requests, verifying the access token,
per-caller rate limiting, attaching signed identity headers, and reverse
proxying to the right service based on the path prefix.

**Not responsible for**: authorization decisions beyond "is this route public".
Role checks belong to the domain services.

## auth

**Owns**: `users` (credentials, roles, status), `refresh_tokens` (rotation
families), `login_attempts`.

**Responsible for**: registration, login, password change, JWT access token
issuance (15 min), refresh token rotation with reuse detection (30 days),
admin user management (list, set roles, set status), bootstrapping the admin
account on first start.

**Not responsible for**: profile data, preferences, entitlements. It emits
`user.registered` and lets the user service build the profile.

**How others read it**: the gateway verifies tokens with the shared JWT secret,
no call to auth needed. Admin user listing is proxied through the gateway.

## user

**Owns**: `profiles`, `preferences`, `likes`, `bookmarks`, `follows`,
`listening_history`, `entitlements`, `promo_codes`, `promo_redemptions`.

**Responsible for**: the "me" API (profile, preferences, library), the social
graph (follow a show or creator), redeeming a promo code, and the entitlement
record that grants Auralis Premium. Emits `user.entitlement_changed`.

**Consumes**: `user.registered` (create the profile, default preferences, and
the base entitlement row), content publish events and playback events (to
maintain `listening_history` and derived signals).

**Not responsible for**: enforcing entitlements at playback time. That is the
playback service reading the entitlement.

## content

**Owns**: `languages`, `genres`, `creators`, `shows`, `seasons`, `episodes`,
`media_uploads`, `review_events`, and the full-text search index.

**Responsible for**: the catalog (public read), authoring (create and edit
shows, seasons, episodes as a creator), the review workflow
(`ready_for_review` to `approved` to `published`, plus `reject` and `archive`),
Postgres full-text search, and the internal API that the ai-media service uses
to create AI-generated draft shows and attach packaged media.

**Emits**: `content.show_published`, `content.episode_published`,
`content.episode_unpublished`, `content.episode_archived`,
`content.audio_uploaded`.

**Not responsible for**: transcoding or packaging audio. It stores media
pointers and processing status; ai-media does the work and patches the result
back through the internal API.

## playback

**Owns**: `episode_cache` (a projection of publishable episodes and their media
keys), `devices`, `playback_sessions`, `playback_progress`, `seen_events`.

**Responsible for**: authorizing playback (checking the episode is published
and, if premium, that the caller is entitled), returning presigned HLS URLs,
recording progress and playback events, "continue listening", and device
registration.

**Consumes**: `auralis.content.events` to keep `episode_cache` current.

**Calls synchronously**: `content` for episode media metadata on a cache miss,
`user` for the premium entitlement check.

**Emits**: `playback.play`, `.pause`, `.seek`, `.skip`, `.progress`,
`.completed`, `.buffer_start`, `.buffer_end`.

## recommendation

**Owns**: `reco_shows`, `reco_show_signals`, `reco_user_genre_affinity`,
`reco_user_language_affinity`, `reco_user_show_affinity`, `reco_user_prefs`,
`reco_co_play`, `processed_events`. All are projections.

**Responsible for**: the personalized feed, similar shows, trending, popular,
the ranking model, and the offline evaluation harness.

**Consumes**: `auralis.content.events`, `auralis.playback.events`,
`auralis.user.events`.

**Not responsible for**: any authoritative state. If its database is wiped it
rebuilds from the event log.

## ai-media

**Owns**: `generation_jobs`, `job_events`, `series_bibles`, `bible_characters`,
`bible_relationships`, `bible_world_rules`, `plot_threads`,
`episode_summaries`, `processed_events`.

**Responsible for**: accepting generation requests (202, queued), running the
series and episode pipelines in a worker, maintaining story continuity state,
synthesizing speech, transcoding and packaging HLS, uploading to object
storage, and patching the episode media back onto content.

**Consumes**: `content.audio_uploaded` (a creator upload becomes a packaging
job).

**Calls synchronously**: `content` internal API (create draft show, attach
media, update processing status), `user` (verify the requester is a creator).

## analytics

**Owns**: `processed_events`, `listener_days`, `daily_totals`, `episode_stats`,
`show_stats`, `show_listeners`, `retention`, `user_cohorts`. All are rollups.

**Responsible for**: consuming every domain event topic and maintaining
aggregate metrics; serving the read-only analytics API (per-show and
per-episode performance for any authenticated caller, platform overview and
retention for admins).

**Not responsible for**: storing raw events. It aggregates on ingest and keeps
only rollups.
