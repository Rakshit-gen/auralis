# Local Development

Two ways to run the whole platform locally. Both give you the eight services,
the databases, Kafka, Redis, object storage, and the web client.

## Option A: Docker Compose (recommended)

Requires Docker with Compose v2.

```
cp .env.example .env
make up          # build and start everything
make migrate     # run every service's migrations
make seed        # load the fictional catalog and synthetic activity
make e2e         # run the critical end-to-end flows
```

Then open:

| URL | What |
| --- | --- |
| http://localhost:3000 | web client |
| http://localhost:8080/api | gateway API |
| http://localhost:9001 | MinIO console (`auralis` / `auralis-secret`) |
| http://localhost:9090 | Prometheus |
| http://localhost:3001 | Grafana (`admin` / `admin`) |

`make logs` tails everything, `make ps` shows status, `make down` stops and
keeps data, `make clean` stops and deletes volumes.

One Postgres 16 container holds all seven logical databases (created by
`infra/compose/postgres-init.sql`). One Kafka broker. The ai-media worker runs
as its own container (`ai-media-worker`, `command: ["worker"]`).

## Option B: Native, no containers

`scripts/dev-native.sh` runs the Go services as compiled binaries and the
Python services in the venv, against Homebrew Postgres, Kafka, and Redis plus a
downloaded MinIO. Faster iteration, no Docker.

```
brew install postgresql@16 kafka redis
brew services start postgresql@16 kafka redis
make dev-setup
scripts/dev-native.sh up      # builds, migrates, seeds, starts all 8 + minio
scripts/dev-native.sh logs
scripts/dev-native.sh down
```

State lives in `.native/` (`pids`, `logs`, `bin`, `minio-data`). Ports match
Compose: gateway 8080, auth 8081, content 8082, user 8083, playback 8084,
ai-media 8085, recommendation 8086, analytics 8087.

Rate limits can be relaxed for load testing with env overrides, for example
`GATEWAY_RATE_LIMIT=200000 AUTH_RATE_LIMIT_RPM=100000 AUTH_RATE_LIMIT_BURST=5000
scripts/dev-native.sh up`. Bounce the stack afterward to restore defaults.

## Working on one service

Each Go service is its own module in the `go.work` workspace. Build and run one:

```
cd services/playback
go run . migrate
go run .
```

Python services:

```
cd services/recommendation
../../.venv/bin/python -m auralis_reco migrate
../../.venv/bin/python -m auralis_reco
```

Each service reads its config from the environment; see the service's
`settings` file for the full list and defaults.

## Frontend

```
cd frontend
npm install
npm run dev       # http://localhost:3000, expects the gateway on :8080
```

`NEXT_PUBLIC_API_BASE` points the client at the gateway.

## AI generation locally

No API key is needed. The bundled `LocalLLMProvider` and espeak-ng
`LocalTTSProvider` are the defaults and fully deterministic by seed. Set
`GROQ_API_KEY` to use Groq for scripts; it falls back to local on any error.
`ffmpeg`, `ffprobe`, and `espeak-ng` must be on `PATH` for the worker (the
Docker image installs them; `brew install ffmpeg espeak-ng` for native).

## Common tasks

| Command | Effect |
| --- | --- |
| `make fmt` | format Go and Python |
| `make lint` | vet, ruff, eslint |
| `make typecheck` | mypy and tsc |
| `make test` | every test suite |
| `make reco-eval` | offline recommendation metrics |
| `make load` | k6 load tests (needs k6) |
