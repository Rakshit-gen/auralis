#!/usr/bin/env python3
"""Seed the running Auralis platform with a fictional catalog and synthetic
activity, exercising the full event pipeline (outbox to Kafka to the analytics
and recommendation projections).

It drives the platform through its public and service-to-service APIs, so the
services must already be up and migrated. Run it after `make up && make migrate`:

    python scripts/seed.py

Everything it creates is original and fictional. It is roughly idempotent: if a
catalog already exists it exits without adding more.

Configuration (env, with local defaults):
    GATEWAY_URL                 http://localhost:8080
    CONTENT_SERVICE_URL         http://localhost:8082
    SERVICE_SHARED_TOKEN        (required, matches the services)
    AUTH_BOOTSTRAP_ADMIN_EMAIL  admin@auralis.local
    AUTH_BOOTSTRAP_ADMIN_PASSWORD
"""

from __future__ import annotations

import os
import random
import sys
import time
from datetime import datetime, timezone

import httpx

GATEWAY = os.environ.get("GATEWAY_URL", "http://localhost:8080").rstrip("/")
CONTENT = os.environ.get("CONTENT_SERVICE_URL", "http://localhost:8082").rstrip("/")
SERVICE_TOKEN = os.environ.get("SERVICE_SHARED_TOKEN", "")
ADMIN_EMAIL = os.environ.get("AUTH_BOOTSTRAP_ADMIN_EMAIL", "admin@auralis.local")
ADMIN_PASSWORD = os.environ.get("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "auralis-admin-pw")

RNG = random.Random(20260908)

# --- fictional catalog material ---------------------------------------------

CREATORS = [
    ("mara.okonkwo@auralis.studio", "Mara Okonkwo", "Documentary-trained, writes audio drama about infrastructure and grief."),
    ("theo.vantol@auralis.studio", "Theo van Tol", "Ex-radio, obsessed with what a room sounds like when someone lies in it."),
    ("priya.raman@auralis.studio", "Priya Raman", "Speculative fiction, always one working machine away from disaster."),
    ("desmond.hale@auralis.studio", "Desmond Hale", "Noir and procedure. Believes the paperwork is the story."),
    ("noor.haddad@auralis.studio", "Noor Haddad", "Folk horror rooted in real places she refuses to name."),
    ("juno.park@auralis.studio", "Juno Park", "Warm, funny, quietly devastating slice-of-life."),
]

SHOW_BLUEPRINTS = [
    ("The Tide Ledger", "mystery", "en", "A harbour auditor finds a second set of books written in a language only the drowned still read.",
     ["harbour", "debt", "audio-drama"], False),
    ("Signal from Badwater", "science-fiction", "en", "The last working relay station picks up a voice from a place that should not have one.",
     ["desert", "first-contact", "isolation"], True),
    ("Every Room Remembers", "folk-horror", "en", "A memory-appraiser is hired to value a house that keeps adding rooms.",
     ["house", "memory", "ritual"], False),
    ("Night Shift, Delta Line", "thriller", "en", "A paramedic starts getting dispatched to addresses that do not exist yet.",
     ["paramedic", "city", "premonition"], True),
    ("The Long Way Down", "adventure", "en", "Two rival cartographers are trapped in a river delta that redraws itself every night.",
     ["river", "maps", "survival"], False),
    ("Ledger of Small Debts", "drama", "en", "A rural notary keeps a private list of the favours nobody will admit to owing.",
     ["village", "promises", "family"], False),
    ("Cold Open", "noir", "en", "A daytime-TV crime consultant is asked to solve a murder that copies one of her old scripts.",
     ["television", "murder", "guilt"], True),
    ("The Salt Year", "history", "en", "A fictional coastal town during a real drought, told through the water board's minutes.",
     ["drought", "bureaucracy", "coast"], False),
    ("Understudy", "comedy", "en", "A community-theatre understudy is the only one who read the whole script, and it is coming true.",
     ["theatre", "small-town", "prophecy"], False),
    ("The Quiet Part", "mystery", "en", "A speech therapist realises three unrelated clients are all rehearsing the same confession.",
     ["therapy", "confession", "pattern"], False),
    ("Rope Access", "thriller", "en", "A window cleaner on the tallest tower in the city sees something on the 60th floor that is not supposed to be there.",
     ["heights", "city", "witness"], True),
    ("Marisol and the Machine", "science-fiction", "en", "A dockworker is the only person who can run the engine that keeps the tidal city afloat, and the only one who knows the cost.",
     ["tidal-city", "labour", "sacrifice"], False),
    ("The Inheritance Nobody Names", "drama", "hi", "Three siblings return to a family house to divide an estate that includes a debt spoken only aloud.",
     ["family", "inheritance", "secret"], False),
    ("Aurora Post", "science-fiction", "en", "An orbital archive that remembers more than the people who built it starts writing letters home.",
     ["orbit", "archive", "memory"], True),
    ("The Crossing at Vela", "adventure", "es", "A night market that only assembles when someone is grieving, and the courier who keeps missing it.",
     ["market", "grief", "courier"], False),
    ("Interest Owed", "noir", "en", "A repo agent for a lender that takes memories instead of money.",
     ["debt", "memory", "collector"], True),
    ("Low Tide Choir", "folk-horror", "en", "A coastal choir practises a hymn that only sounds right when the water is out.",
     ["choir", "coast", "tradition"], False),
    ("The Patient Line", "thriller", "en", "A crisis-line volunteer recognises a caller's voice from a case that was closed years ago.",
     ["crisis-line", "cold-case", "voice"], False),
    ("Groundskeeper", "folk-horror", "en", "The new groundskeeper of a monastery that trades in other people's memories learns the filing system.",
     ["monastery", "memory", "labour"], True),
    ("Two Hands on the Wheel", "drama", "en", "A long-haul driver and the daughter she has not spoken to in a decade share a route for one week.",
     ["road", "family", "reconciliation"], False),
    ("The Vela Signal", "science-fiction", "pt", "A radio astronomer in a remote observatory starts hearing her own name in the background noise.",
     ["astronomy", "isolation", "signal"], False),
    ("Settlement", "mystery", "en", "An insurance investigator works a flood claim in a town that has flooded on the same day for a century.",
     ["insurance", "flood", "pattern"], False),
    ("The Understair Room", "folk-horror", "fr", "A family discovers a door under the stairs that was not there at the viewing.",
     ["house", "door", "family"], False),
    ("Harbourmaster", "drama", "en", "The last harbourmaster of a port that is closing, and the log she cannot stop keeping.",
     ["harbour", "closure", "duty"], False),
    ("Cover Version", "noir", "en", "A session musician recognises a hit song as one he wrote and was never paid for, then people start dying.",
     ["music", "theft", "revenge"], True),
    ("The Auditors", "thriller", "en", "Two forensic accountants are sent to a company town where the books balance too perfectly.",
     ["accounting", "company-town", "conspiracy"], False),
    ("Slack Water", "slice-of-life", "en", "A tide-pool researcher and the retired ferryman who reports the water to her every morning.",
     ["coast", "routine", "friendship"], False),
    ("The Rehearsal", "drama", "en", "A hospice music therapist helps a former conductor prepare one last performance nobody will hear.",
     ["hospice", "music", "ending"], False),
    ("Border Frequency", "thriller", "es", "A border-town radio DJ starts receiving song requests that predict who will cross that night.",
     ["border", "radio", "premonition"], True),
    ("The Kept List", "mystery", "en", "A small-town librarian keeps a private catalogue of the books people return without ever having checked out.",
     ["library", "small-town", "pattern"], False),
    ("Deadweight", "adventure", "en", "A salvage crew raises a ship that has been missing for forty years, with the crew still aboard and still arguing.",
     ["salvage", "ship", "time"], True),
    ("The Long Room", "history", "en", "A fictional records office during a real archival fire, told by the clerks deciding what to save.",
     ["archive", "fire", "choice"], False),
]


def die(msg: str) -> None:
    print(f"seed: {msg}", file=sys.stderr)
    sys.exit(1)


class Client:
    def __init__(self, base: str):
        self.http = httpx.Client(base_url=base, timeout=30.0)
        self.token: str | None = None

    def auth_headers(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self.token}"} if self.token else {}

    def register(self, email: str, password: str, name: str) -> dict:
        r = self.http.post("/api/auth/register", json={"email": email, "password": password, "display_name": name})
        if r.status_code == 409:
            return self.login(email, password)
        r.raise_for_status()
        body = r.json()
        self.token = body["tokens"]["access_token"]
        return body

    def login(self, email: str, password: str) -> dict:
        r = self.http.post("/api/auth/login", json={"email": email, "password": password})
        r.raise_for_status()
        body = r.json()
        self.token = body["tokens"]["access_token"]
        return body

    def get(self, path: str, **kw):
        return self.http.get(path, headers=self.auth_headers(), **kw)

    def post(self, path: str, json=None):
        return self.http.post(path, headers=self.auth_headers(), json=json)

    def put(self, path: str, json=None):
        return self.http.put(path, headers=self.auth_headers(), json=json)

    def patch(self, path: str, json=None):
        return self.http.patch(path, headers=self.auth_headers(), json=json)


def wait_for_gateway() -> None:
    for _ in range(60):
        try:
            if httpx.get(f"{GATEWAY}/health", timeout=3).status_code == 200:
                return
        except httpx.HTTPError:
            pass
        time.sleep(2)
    die(f"gateway at {GATEWAY} never became healthy")


def main() -> None:
    if not SERVICE_TOKEN:
        die("SERVICE_SHARED_TOKEN is required")
    wait_for_gateway()

    gw = Client(GATEWAY)
    content_svc = httpx.Client(base_url=CONTENT, timeout=30.0, headers={"X-Auralis-Service-Token": SERVICE_TOKEN})

    admin = Client(GATEWAY)
    admin.login(ADMIN_EMAIL, ADMIN_PASSWORD)
    print(f"logged in as admin {ADMIN_EMAIL}")

    # Skip if the catalog already has shows.
    existing = gw.get("/api/catalog/shows", params={"limit": 1}).json()
    if existing.get("total", 0) >= len(SHOW_BLUEPRINTS):
        print(f"catalog already has {existing['total']} shows; nothing to do")
        return

    genres = {g["slug"]: g["id"] for g in gw.get("/api/catalog/genres").json()["genres"]}
    if not genres:
        die("no genres found; run migrations first (make migrate)")

    # --- creators -----------------------------------------------------------
    creator_tokens: dict[str, Client] = {}
    creator_ids: dict[str, str] = {}
    for email, name, _bio in CREATORS:
        c = Client(GATEWAY)
        body = c.register(email, "creator-account-1", name)
        uid = body["user"]["id"]
        admin.post(f"/api/admin/users/{uid}/roles", json={"roles": ["USER", "CREATOR"]})
        c.login(email, "creator-account-1")  # refresh token with the new role
        creator_tokens[email] = c
        creator_ids[email] = uid
    print(f"created {len(CREATORS)} creators")

    # --- shows, seasons, episodes ----------------------------------------
    published_show_ids: list[str] = []
    published_episode_ids: list[tuple[str, str]] = []  # (show_id, episode_id)
    for i, (title, genre_slug, lang, synopsis, tags, premium) in enumerate(SHOW_BLUEPRINTS):
        email = CREATORS[i % len(CREATORS)][0]
        c = creator_tokens[email]
        gid = genres.get(genre_slug) or next(iter(genres.values()))
        r = c.post("/api/content/shows", json={
            "title": title, "synopsis": synopsis, "description": synopsis + "\n\nAn original serialized audio drama.",
            "language_code": lang, "genre_ids": [gid], "tags": tags, "is_premium": premium,
            "creator_name": CREATORS[i % len(CREATORS)][1],
        })
        r.raise_for_status()
        show = r.json()
        show_id = show["id"]

        season = c.post(f"/api/content/shows/{show_id}/seasons", json={"number": 1, "title": "Season 1"}).json()
        season_id = season["id"]

        episode_count = RNG.randint(5, 9)
        for n in range(1, episode_count + 1):
            ep = c.post(f"/api/content/seasons/{season_id}/episodes", json={
                "number": n, "title": f"{title}: Part {n}",
                "synopsis": f"Episode {n} of {title}.",
                "is_premium": premium and n > 2,
                "free_preview_sec": 90 if (premium and n > 2) else 0,
            }).json()
            ep_id = ep["id"]

            # Attach fictional packaged-audio metadata so the episode is "ready".
            duration = RNG.randint(900, 2400)
            key = f"hls/{show_id}/{ep_id}"
            content_svc.patch(f"/internal/authoring/episodes/{ep_id}", json={
                "script": f"Narrator: {title}, part {n}. The scene is set. A conversation begins, and does not end well.\n"
                          f"Narrator: The episode closes on a question nobody wants to answer.",
                "media": {
                    "hls_master_key": f"{key}/master.m3u8",
                    "variants": [
                        {"bitrate_kbps": 64, "key": f"{key}/audio_64.m3u8", "codec": "aac", "size_bytes": duration * 8000},
                        {"bitrate_kbps": 128, "key": f"{key}/audio_128.m3u8", "codec": "aac", "size_bytes": duration * 16000},
                        {"bitrate_kbps": 256, "key": f"{key}/audio_256.m3u8", "codec": "aac", "size_bytes": duration * 32000},
                    ],
                    "codec": "aac", "sample_rate_hz": 44100, "channels": 2,
                    "file_size_bytes": duration * 32000, "checksum_sha256": f"{ep_id.replace('-', '')}00",
                    "duration_sec": duration,
                },
            }).raise_for_status()

            c.post(f"/api/content/episodes/{ep_id}/submit")
            admin.post(f"/api/content/admin/review/episode/{ep_id}", json={"action": "approve"})
            admin.post(f"/api/content/admin/review/episode/{ep_id}", json={"action": "publish"})
            published_episode_ids.append((show_id, ep_id))

        c.post(f"/api/content/shows/{show_id}/submit")
        admin.post(f"/api/content/admin/review/show/{show_id}", json={"action": "approve"})
        admin.post(f"/api/content/admin/review/show/{show_id}", json={"action": "publish"})
        published_show_ids.append(show_id)
        print(f"  published {title} ({episode_count} episodes)")

    print(f"published {len(published_show_ids)} shows, {len(published_episode_ids)} episodes")

    # --- listeners --------------------------------------------------------
    listeners: list[Client] = []
    for n in range(55):
        c = Client(GATEWAY)
        c.register(f"listener{n:02d}@auralis.local", "listener-account-1", f"Listener {n:02d}")
        listeners.append(c)
    print(f"created {len(listeners)} listeners")

    # A few listeners redeem the promo code.
    for c in listeners[:8]:
        c.post("/api/me/entitlement/redeem", json={"code": "AURALIS-PREMIUM"})

    # Preferences: give each listener a couple of genre affinities.
    genre_slugs = list(genres.keys())
    for c in listeners:
        picks = RNG.sample(genre_slugs, k=RNG.randint(1, 3))
        c.put("/api/me/preferences", json={
            "genre_slugs": picks, "language_codes": ["en"], "autoplay": True,
            "playback_speed": RNG.choice([1.0, 1.0, 1.25, 1.5]), "explicit_ok": True, "email_updates": True,
        })

    # --- synthetic activity: plays, progress, completes, likes, follows ---
    shows_meta = {s["id"]: s for s in _all_shows(gw)}
    episodes_by_show: dict[str, list[dict]] = {}
    for sid in published_show_ids:
        eps = gw.get(f"/api/catalog/shows/{sid}/episodes").json()["episodes"]
        episodes_by_show[sid] = sorted(eps, key=lambda e: e["number"])

    total_events = 0
    for c in listeners:
        # Each listener has 2 to 5 shows they engage with, weighted by their genre picks.
        n_shows = RNG.randint(2, 5)
        chosen = RNG.sample(published_show_ids, k=min(n_shows, len(published_show_ids)))
        for sid in chosen:
            eps = episodes_by_show.get(sid, [])
            if not eps:
                continue
            if RNG.random() < 0.6:
                c.post(f"/api/me/follows/{sid}")
            if RNG.random() < 0.4:
                c.post("/api/me/likes", json={"type": "show", "id": sid, "show_id": sid})

            # Listen to a prefix of the episodes, sometimes finishing, sometimes dropping off.
            depth = RNG.randint(1, len(eps))
            for ep in eps[:depth]:
                ep_id = ep["id"]
                auth = c.post("/api/playback/authorize", json={"episode_id": ep_id})
                if auth.status_code != 200:
                    continue
                duration = ep["duration_sec"] or 1200
                session = auth.json()["session_id"]
                finishes = RNG.random() < 0.55
                stop_at = duration if finishes else RNG.randint(30, max(60, int(duration * 0.8)))
                events = [{
                    "type": "PLAY", "episode_id": ep_id, "show_id": sid, "session_id": session,
                    "position_sec": 0, "client_event_id": f"{session}:play",
                    "occurred_at": datetime.now(timezone.utc).isoformat(),
                }]
                pos = 0
                while pos < stop_at:
                    pos = min(stop_at, pos + RNG.randint(60, 240))
                    events.append({
                        "type": "PROGRESS", "episode_id": ep_id, "show_id": sid, "session_id": session,
                        "position_sec": pos, "client_event_id": f"{session}:p{pos}",
                        "occurred_at": datetime.now(timezone.utc).isoformat(),
                    })
                if finishes:
                    events.append({
                        "type": "COMPLETE", "episode_id": ep_id, "show_id": sid, "session_id": session,
                        "position_sec": duration, "client_event_id": f"{session}:complete",
                        "occurred_at": datetime.now(timezone.utc).isoformat(),
                    })
                elif RNG.random() < 0.3:
                    events.append({
                        "type": "SKIP", "episode_id": ep_id, "show_id": sid, "session_id": session,
                        "position_sec": stop_at, "from_sec": stop_at, "to_sec": min(duration, stop_at + 120),
                        "client_event_id": f"{session}:skip",
                        "occurred_at": datetime.now(timezone.utc).isoformat(),
                    })
                c.post("/api/playback/events", json={"events": events})
                total_events += len(events)

                if finishes and RNG.random() < 0.25:
                    c.post("/api/me/likes", json={"type": "episode", "id": ep_id, "show_id": sid})
                if RNG.random() < 0.15:
                    c.post("/api/me/bookmarks", json={"episode_id": ep_id, "show_id": sid, "note": ""})

    print(f"emitted ~{total_events} playback events across {len(listeners)} listeners")
    print("seed complete. Analytics and recommendation projections will catch up within a minute.")
    _ = shows_meta


def _all_shows(gw: Client) -> list[dict]:
    out: list[dict] = []
    offset = 0
    while True:
        page = gw.get("/api/catalog/shows", params={"limit": 100, "offset": offset}).json()
        out.extend(page["shows"])
        if len(page["shows"]) < 100:
            return out
        offset += 100


if __name__ == "__main__":
    main()
