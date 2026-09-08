#!/usr/bin/env python3
"""Critical end-to-end flows against a running Auralis stack.

Exercises the paths a real user takes and asserts the cross-service effects:
register, browse, authorize playback, save progress, resume, like, follow,
generate an AI series, and (as admin) read analytics. Exits non-zero on the
first failure.

    python scripts/e2e.py            # against http://localhost:8080
    GATEWAY_URL=https://... python scripts/e2e.py
"""

from __future__ import annotations

import os
import sys
import time
import uuid

import httpx

GATEWAY = os.environ.get("GATEWAY_URL", "http://localhost:8080").rstrip("/")
ADMIN_EMAIL = os.environ.get("AUTH_BOOTSTRAP_ADMIN_EMAIL", "admin@auralis.local")
ADMIN_PASSWORD = os.environ.get("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "auralis-admin-pw")

PASS = 0
FAIL = 0


def check(name: str, ok: bool, detail: str = "") -> None:
    global PASS, FAIL
    if ok:
        PASS += 1
        print(f"  ok   {name}")
    else:
        FAIL += 1
        print(f"  FAIL {name}  {detail}")


def main() -> None:
    http = httpx.Client(base_url=GATEWAY, timeout=30.0)

    # 1. Health
    check("gateway health", http.get("/health").status_code == 200)

    # 2. Public catalog
    shows = http.get("/api/catalog/shows", params={"limit": 5}).json()
    check("catalog returns published shows", shows.get("total", 0) > 0, str(shows)[:120])
    if not shows.get("shows"):
        print("no catalog; run scripts/seed.py first")
        sys.exit(1)
    a_show = shows["shows"][0]

    # 3. Search
    q = a_show["title"].split()[0]
    search = http.get("/api/catalog/search", params={"q": q}).json()
    check("search finds a show", search.get("total", 0) >= 1, str(search)[:120])

    # 4. Register a fresh listener
    email = f"e2e-{uuid.uuid4().hex[:10]}@auralis.local"
    reg = http.post("/api/auth/register", json={"email": email, "password": "e2e-password-1", "display_name": "E2E"})
    check("register", reg.status_code == 201, reg.text[:160])
    access = reg.json()["tokens"]["access_token"]
    refresh = reg.json()["tokens"]["refresh_token"]
    h = {"Authorization": f"Bearer {access}"}

    # 5. Refresh token rotation
    rot = http.post("/api/auth/refresh", json={"refresh_token": refresh})
    check("refresh rotates", rot.status_code == 200 and rot.json()["tokens"]["refresh_token"] != refresh)
    reuse = http.post("/api/auth/refresh", json={"refresh_token": refresh})
    check("reused refresh token is rejected", reuse.status_code == 401)

    # 6. Profile provisioned by the user.registered event (allow a moment for Kafka)
    profile_ok = False
    for _ in range(15):
        p = http.get("/api/me/profile", headers=h)
        if p.status_code == 200:
            profile_ok = True
            break
        time.sleep(1)
    check("profile provisioned from user.registered event", profile_ok)

    # 7. Episodes for the show
    detail = http.get(f"/api/catalog/shows/{a_show['slug']}").json()
    episodes = detail.get("episodes", [])
    check("show has published episodes", len(episodes) > 0)
    ep = next((e for e in episodes if not e.get("is_premium")), episodes[0])

    # 8. Playback authorization returns a signed URL
    auth = http.post("/api/playback/authorize", headers=h, json={"episode_id": ep["id"]})
    check("playback authorize", auth.status_code == 200, auth.text[:160])
    body = auth.json()
    check("authorize returns a signed master url", bool(body.get("hls_master_url")))
    check("authorize returns bitrate variants", len(body.get("variants", [])) >= 3)
    session = body["session_id"]

    # 9. Save progress, then confirm resume
    http.post("/api/playback/progress", headers=h, json={
        "episode_id": ep["id"], "show_id": a_show["id"], "session_id": session,
        "position_sec": 240, "duration_sec": ep["duration_sec"], "client_event_id": f"{session}:e2e-240",
    })
    # Out-of-order lower position must not roll back.
    http.post("/api/playback/progress", headers=h, json={
        "episode_id": ep["id"], "show_id": a_show["id"], "session_id": session,
        "position_sec": 30, "client_event_id": f"{session}:e2e-30",
        "occurred_at": "2000-01-01T00:00:00Z",
    })
    prog = http.get(f"/api/playback/progress/{ep['id']}", headers=h).json()
    check("resume position is monotonic", prog.get("position_sec") == 240, str(prog))

    re_auth = http.post("/api/playback/authorize", headers=h, json={"episode_id": ep["id"]}).json()
    check("re-authorize reports resume position", re_auth.get("resume_position_sec") == 240)

    # 10. Like + follow, and confirm the library reflects it
    http.post("/api/me/likes", headers=h, json={"type": "show", "id": a_show["id"], "show_id": a_show["id"]})
    http.post(f"/api/me/follows/{a_show['id']}", headers=h)
    likes = http.get("/api/me/likes", headers=h).json()["likes"]
    follows = http.get("/api/me/follows", headers=h).json()["follows"]
    check("like recorded", any(l["target_id"] == a_show["id"] for l in likes))
    check("follow recorded", any(f["show_id"] == a_show["id"] for f in follows))

    # 11. Continue listening shelf
    cont = http.get("/api/playback/continue", headers=h).json()["items"]
    check("continue listening lists the in-progress episode", any(c["episode_id"] == ep["id"] for c in cont))

    # 12. Recommendations feed
    feed = http.get("/api/recommendations/feed", headers=h)
    check("recommendation feed responds", feed.status_code == 200, feed.text[:160])

    # 13. AI generation job
    gen = http.post("/api/generate/series", headers=h, json={
        "brief": "A lighthouse keeper on a decommissioned coast starts logging ships that were lost decades ago.",
        "episode_count": 3,
    })
    check("AI series generation accepted", gen.status_code == 202, gen.text[:160])
    if gen.status_code == 202:
        job_id = gen.json()["job_id"]
        done = False
        for _ in range(90):
            j = http.get(f"/api/generate/jobs/{job_id}", headers=h).json()
            if j["status"] in ("completed", "failed"):
                done = j["status"] == "completed"
                break
            time.sleep(2)
        check("AI series job completes", done, "job did not complete in 3 minutes")

    # 14. Admin analytics
    admin_login = http.post("/api/auth/login", json={"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD})
    if admin_login.status_code == 200:
        ah = {"Authorization": f"Bearer {admin_login.json()['tokens']['access_token']}"}
        overview = http.get("/api/analytics/overview", headers=ah)
        check("admin analytics overview", overview.status_code == 200, overview.text[:160])
        show_perf = http.get(f"/api/analytics/shows/{a_show['id']}", headers=ah)
        check("per-show analytics reachable", show_perf.status_code in (200, 404))
    else:
        check("admin login", False, admin_login.text[:120])

    print(f"\n{PASS} passed, {FAIL} failed")
    sys.exit(1 if FAIL else 0)


if __name__ == "__main__":
    main()
