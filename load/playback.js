// k6 load test for the authenticated playback path: register, authorize, and
// stream progress events.
//
//   k6 run load/playback.js
//
// Each VU registers its own listener so the auth and playback services see
// realistic per-user load. Measures requests/sec and p50/p95/p99 for the
// authorize and progress endpoints.

import http from "k6/http";
import { check, sleep } from "k6";
import { Trend, Counter } from "k6/metrics";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

const BASE = __ENV.BASE || "http://localhost:8080";

const authorizeLatency = new Trend("playback_authorize_ms", true);
const progressLatency = new Trend("playback_progress_ms", true);
const eventsSent = new Counter("playback_events_sent");

export const options = {
  scenarios: {
    listen: {
      executor: "ramping-vus",
      startVUs: 2,
      stages: [
        { duration: "30s", target: 20 },
        { duration: "1m30s", target: 20 },
        { duration: "20s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.02"],
    playback_authorize_ms: ["p(95)<600", "p(99)<1200"],
    playback_progress_ms: ["p(95)<300", "p(99)<600"],
  },
};

export function setup() {
  const res = http.get(`${BASE}/api/catalog/shows?limit=40`);
  const shows = res.json("shows") || [];
  const episodes = [];
  for (const s of shows) {
    const detail = http.get(`${BASE}/api/catalog/shows/${s.slug}`).json();
    for (const e of detail.episodes || []) {
      if (!e.is_premium) episodes.push({ id: e.id, show_id: s.id, duration: e.duration_sec || 1200 });
    }
  }
  return { episodes };
}

export default function (data) {
  if (!data.episodes.length) return;

  const email = `k6-${uuidv4()}@auralis.local`;
  const reg = http.post(
    `${BASE}/api/auth/register`,
    JSON.stringify({ email, password: "k6-load-password-1", display_name: "k6" }),
    { headers: { "Content-Type": "application/json" } },
  );
  if (!check(reg, { "register 201": (r) => r.status === 201 })) {
    sleep(1); // do not spin the loop if registration is failing
    return;
  }
  const token = reg.json("tokens.access_token");
  const authHeaders = { headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` } };

  const ep = data.episodes[Math.floor(Math.random() * data.episodes.length)];

  let res = http.post(`${BASE}/api/playback/authorize`, JSON.stringify({ episode_id: ep.id }), authHeaders);
  authorizeLatency.add(res.timings.duration);
  if (!check(res, { "authorize 200": (r) => r.status === 200 })) return;
  const session = res.json("session_id");

  // Simulate ~90 seconds of listening compressed into a few progress ticks.
  let pos = 0;
  for (let i = 0; i < 6; i++) {
    pos += 15;
    res = http.post(
      `${BASE}/api/playback/progress`,
      JSON.stringify({
        episode_id: ep.id,
        show_id: ep.show_id,
        session_id: session,
        position_sec: pos,
        duration_sec: ep.duration,
        client_event_id: `${session}:p${pos}`,
      }),
      authHeaders,
    );
    progressLatency.add(res.timings.duration);
    eventsSent.add(1);
    check(res, { "progress 200": (r) => r.status === 200 });
    sleep(0.5);
  }
}
