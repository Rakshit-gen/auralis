#!/usr/bin/env bash
# Run the whole Auralis backend natively, without containers.
#
# Requires local Postgres, Redis, Kafka, and MinIO plus Go 1.25, a Python 3.13
# venv at .venv, and ffmpeg for the ai-media worker (run scripts/piper-setup.sh
# for neural voices, or install espeak-ng for the fallback). Intended for
# development on a machine without a container runtime; production uses the
# Docker/Helm path.
#
#   scripts/dev-native.sh up      build binaries, run migrations, start everything
#   scripts/dev-native.sh down    stop everything
#   scripts/dev-native.sh logs    tail all logs
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN="$ROOT/.native"
PIDS="$RUN/pids"
LOGS="$RUN/logs"
VENV="${VENV:-$ROOT/.venv}"
PG_HOST="${PG_HOST:-localhost}"
PG_USER="${PG_USER:-$USER}"

export IDENTITY_SECRET="${IDENTITY_SECRET:-dev-identity-secret-0123456789}"
export SERVICE_SHARED_TOKEN="${SERVICE_SHARED_TOKEN:-dev-service-token-0123456789}"
export JWT_SECRET="${JWT_SECRET:-dev-jwt-secret-0123456789}"
export JWT_ISSUER=auralis-auth JWT_AUDIENCE=auralis
export KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}"
export CORS_ALLOWED_ORIGINS="http://localhost:3000"
export S3_ENDPOINT="${S3_ENDPOINT:-localhost:9000}"
export S3_ACCESS_KEY="${S3_ACCESS_KEY:-minioadmin}"
export S3_SECRET_KEY="${S3_SECRET_KEY:-minioadmin}"
export S3_BUCKET=auralis-media S3_REGION=us-east-1 S3_USE_SSL=false
export ADMIN_EMAIL="${ADMIN_EMAIL:-admin@auralis.local}"
export ADMIN_PASSWORD="${ADMIN_PASSWORD:-auralis-admin-pw}"
export AUTH_BOOTSTRAP_ADMIN_EMAIL="$ADMIN_EMAIL"
export AUTH_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PASSWORD"
export LOG_LEVEL="${LOG_LEVEL:-info}"

pg() { echo "postgres://$PG_USER@$PG_HOST:5432/$1?sslmode=disable"; }
pgpy() { echo "postgresql://$PG_USER@$PG_HOST:5432/$1"; }

start() {
  local name="$1"; shift
  echo "  start $name"
  ( "$@" >"$LOGS/$name.log" 2>&1 & echo $! >"$PIDS/$name" )
  sleep 0.4
}

wait_http() {
  local url="$1" tries="${2:-40}"
  for _ in $(seq 1 "$tries"); do
    curl -fsS "$url" >/dev/null 2>&1 && return 0
    sleep 0.75
  done
  echo "  timed out waiting for $url" >&2
  return 1
}

cmd_up() {
  mkdir -p "$PIDS" "$LOGS"

  for db in auth users content playback analytics ai_media recommendation; do
    createdb "$db" 2>/dev/null || true
  done

  if ! curl -fsS "http://$S3_ENDPOINT/minio/health/live" >/dev/null 2>&1; then
    echo "  start minio"
    MINIO_ROOT_USER="$S3_ACCESS_KEY" MINIO_ROOT_PASSWORD="$S3_SECRET_KEY" \
      minio server "$RUN/minio-data" --address ":9000" --console-address ":9001" \
      >"$LOGS/minio.log" 2>&1 & echo $! >"$PIDS/minio"
    wait_http "http://$S3_ENDPOINT/minio/health/live"
  fi

  echo "building Go services..."
  mkdir -p "$RUN/bin"
  for s in gateway auth user content playback analytics; do
    (cd "$ROOT/services/$s" && go build -o "$RUN/bin/$s" .)
  done

  echo "starting services..."
  AUTH_DATABASE_URL="$(pg auth)" AUTH_HTTP_ADDR=":8081" \
    AUTH_RATE_LIMIT_RPM="${AUTH_RATE_LIMIT_RPM:-120}" AUTH_RATE_LIMIT_BURST="${AUTH_RATE_LIMIT_BURST:-40}" \
    start auth "$RUN/bin/auth"
  USER_DATABASE_URL="$(pg users)" USER_HTTP_ADDR=":8083" start user "$RUN/bin/user"
  CONTENT_DATABASE_URL="$(pg content)" CONTENT_HTTP_ADDR=":8082" start content "$RUN/bin/content"
  ANALYTICS_DATABASE_URL="$(pg analytics)" ANALYTICS_HTTP_ADDR=":8087" start analytics "$RUN/bin/analytics"
  PLAYBACK_DATABASE_URL="$(pg playback)" PLAYBACK_HTTP_ADDR=":8084" \
    CONTENT_SERVICE_URL="http://localhost:8082" USER_SERVICE_URL="http://localhost:8083" \
    start playback "$RUN/bin/playback"

  # Piper neural voices if scripts/piper-setup.sh has been run, else espeak-ng.
  PIPER_VOICES_DIR="${PIPER_VOICES_DIR:-$ROOT/.piper/voices}" \
    PIPER_BIN="${PIPER_BIN:-$ROOT/.piper/bin/piper}" \
    AI_MEDIA_DATABASE_URL="$(pgpy ai_media)" AI_MEDIA_HTTP_ADDR="0.0.0.0:8085" \
    CONTENT_SERVICE_URL="http://localhost:8082" USER_SERVICE_URL="http://localhost:8083" \
    AI_MEDIA_RUN_WORKER=true AI_DEFAULT_PROVIDER="${AI_DEFAULT_PROVIDER:-local}" \
    start ai-media bash -c "cd '$ROOT/services/ai_media' && exec '$VENV/bin/python' -m auralis_ai_media"
  RECOMMENDATION_DATABASE_URL="$(pgpy recommendation)" RECOMMENDATION_HTTP_ADDR="0.0.0.0:8086" \
    start recommendation bash -c "cd '$ROOT/services/recommendation' && exec '$VENV/bin/python' -m auralis_reco"

  GATEWAY_HTTP_ADDR=":8080" REDIS_URL="redis://localhost:6379/0" \
    GATEWAY_RATE_LIMIT="${GATEWAY_RATE_LIMIT:-2000}" \
    AUTH_SERVICE_URL="http://localhost:8081" USER_SERVICE_URL="http://localhost:8083" \
    CONTENT_SERVICE_URL="http://localhost:8082" PLAYBACK_SERVICE_URL="http://localhost:8084" \
    RECOMMENDATION_SERVICE_URL="http://localhost:8086" AI_MEDIA_SERVICE_URL="http://localhost:8085" \
    ANALYTICS_SERVICE_URL="http://localhost:8087" \
    start gateway "$RUN/bin/gateway"

  wait_http "http://localhost:8080/health"
  echo "gateway up on http://localhost:8080  (logs: $LOGS)"
}

cmd_down() {
  [ -d "$PIDS" ] || { echo "nothing running"; return; }
  for f in "$PIDS"/*; do
    [ -e "$f" ] || continue
    kill "$(cat "$f")" 2>/dev/null || true
    rm -f "$f"
  done
  echo "stopped"
}

cmd_logs() { tail -n 40 -f "$LOGS"/*.log; }

case "${1:-up}" in
  up) cmd_up ;;
  down) cmd_down ;;
  logs) cmd_logs ;;
  *) echo "usage: $0 {up|down|logs}" >&2; exit 1 ;;
esac
