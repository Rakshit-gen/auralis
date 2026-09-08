# Auralis web client

Next.js 16 (App Router) + TypeScript + Tailwind + TanStack Query + Zustand.

## Develop

```
cp .env.example .env
npm install
npm run dev            # http://localhost:3000
```

The browser calls the API gateway directly using `NEXT_PUBLIC_API_BASE`
(default `http://localhost:8080/api`). Start the backend with the repo's
`make up` first, or point the variable at a deployed gateway.

## Scripts

| Command | What it does |
| --- | --- |
| `npm run dev` | Dev server |
| `npm run build` | Production build (`output: standalone`) |
| `npm run start` | Serve the production build |
| `npm run lint` | ESLint (Next flat config) |
| `npm run typecheck` | `tsc --noEmit` |
| `npm run test` | Vitest (formatters, player store) |

## Structure

- `src/lib/api.ts` — fetch wrapper: attaches the access token, refreshes once on 401.
- `src/lib/hooks.ts` / `src/lib/creator.ts` — typed query and mutation hooks.
- `src/stores/auth.ts` — session, login/register/logout.
- `src/stores/player.ts` — queue, transport state, resume, and playback-event emission.
- `src/lib/playback-events.ts` — batches PLAY/PAUSE/SEEK/PROGRESS/COMPLETE/SKIP/BUFFER events;
  important ones flush immediately, PROGRESS on a 15s interval, and a `sendBeacon` on unload.
- `src/components/player/audio-engine.tsx` — the single `<audio>` element, HLS via hls.js
  with a native fallback, preview-limit enforcement for premium episodes.

## Pages

Landing, auth, home feed, discover, trending, genres, search, show detail, episode deep link,
library (continue / bookmarks / likes / history / following), profile, preferences, premium,
AI generate, creator dashboard + script editor + review status + analytics, and the admin
console (overview, content review, processing jobs, users, system health).
