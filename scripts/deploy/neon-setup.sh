#!/usr/bin/env bash
# Create the seven per-service databases on a Neon project.
#
# Each Auralis service owns its own database (database-per-service). Neon starts
# a project with a single database ("neondb"); this script adds the rest. It is
# idempotent: existing databases are left alone.
#
# Usage:
#   NEON_ADMIN_URL='postgresql://user:pass@ep-xxx.REGION.aws.neon.tech/neondb?sslmode=require' \
#     scripts/deploy/neon-setup.sh
#
# NEON_ADMIN_URL must point at the DIRECT (non-pooler) endpoint; CREATE DATABASE
# does not run over the transaction pooler. If .env.deploy exists it is sourced.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ -f "$here/.env.deploy" ]]; then
  # shellcheck disable=SC1091
  set -a && source "$here/.env.deploy" && set +a
fi

: "${NEON_ADMIN_URL:?set NEON_ADMIN_URL to the direct-endpoint neondb connection string}"

databases=(auth users content playback analytics ai_media recommendation)

for db in "${databases[@]}"; do
  exists="$(psql "$NEON_ADMIN_URL" -tAc "SELECT 1 FROM pg_database WHERE datname = '$db'")"
  if [[ "$exists" == "1" ]]; then
    echo "= $db already exists"
  else
    psql "$NEON_ADMIN_URL" -c "CREATE DATABASE $db"
    echo "+ created $db"
  fi
done

echo
echo "Databases on the project:"
psql "$NEON_ADMIN_URL" -tAc "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY 1" | sed 's/^/  /'
