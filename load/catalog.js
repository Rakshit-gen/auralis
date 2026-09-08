// k6 load test for the public catalog and search paths.
//
//   k6 run load/catalog.js
//   k6 run -e BASE=https://your-gateway load/catalog.js
//
// Reports requests/sec and p50/p95/p99 per the thresholds below. Results are
// whatever k6 measures; nothing here is fabricated.

import http from "k6/http";
import { check, sleep } from "k6";
import { Trend } from "k6/metrics";

const BASE = __ENV.BASE || "http://localhost:8080";

const listLatency = new Trend("catalog_list_ms", true);
const searchLatency = new Trend("catalog_search_ms", true);
const showLatency = new Trend("catalog_show_ms", true);

export const options = {
  scenarios: {
    browse: {
      executor: "ramping-vus",
      startVUs: 5,
      stages: [
        { duration: "30s", target: 40 },
        { duration: "1m", target: 40 },
        { duration: "20s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    "http_req_duration{expected_response:true}": ["p(95)<400", "p(99)<800"],
    catalog_search_ms: ["p(95)<500"],
  },
};

const SORTS = ["recent", "popular", "rating", "title"];
const TERMS = ["harbour", "signal", "memory", "night", "salt", "ledger", "tide"];

export function setup() {
  const res = http.get(`${BASE}/api/catalog/shows?limit=50`);
  const shows = res.json("shows") || [];
  return { slugs: shows.map((s) => s.slug) };
}

export default function (data) {
  const sort = SORTS[Math.floor(Math.random() * SORTS.length)];
  let res = http.get(`${BASE}/api/catalog/shows?limit=24&sort=${sort}`);
  listLatency.add(res.timings.duration);
  check(res, { "list 200": (r) => r.status === 200 });

  const term = TERMS[Math.floor(Math.random() * TERMS.length)];
  res = http.get(`${BASE}/api/catalog/search?q=${term}`);
  searchLatency.add(res.timings.duration);
  check(res, { "search 200": (r) => r.status === 200 });

  if (data.slugs.length) {
    const slug = data.slugs[Math.floor(Math.random() * data.slugs.length)];
    res = http.get(`${BASE}/api/catalog/shows/${slug}`);
    showLatency.add(res.timings.duration);
    check(res, { "show 200": (r) => r.status === 200 });
  }

  sleep(Math.random() * 1.5);
}
