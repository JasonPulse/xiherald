#!/usr/bin/env bash
#
# Prove the whole portrait pipeline, using the real renderer image.
#
# This is what the CronJob does every night, run end to end in seconds:
# MariaDB with the real xiserver schema, the Herald serving /api/v1/appearances,
# the renderer container reading that work-list and writing PNG blobs, and the
# Herald serving them back. Everything talks over a Docker network by service
# name, the same shape as in-cluster DNS.
#
#   ./verify_pipeline.sh                            # uses jasonpulse/vellichor-renderer:latest
#   IMAGE=... ./verify_pipeline.sh                  # a different renderer build
#   KEEP=1 ./verify_pipeline.sh                     # leave it up on :8098
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
XISERVER="${XISERVER:-$ROOT/../xiserver}"
IMAGE="${IMAGE:-jasonpulse/vellichor-renderer:latest}"
MARIADB_TAG="${MARIADB_TAG:-10.11}"
NET=xiportrait-pipeline
DB=xiportrait-db
APP=xiportrait-herald
PORT="${PORT:-8098}"
DBPASS=pipeline
PASSES=0
FAILURES=0

TABLES=(
    chars char_stats char_look char_history char_jobs char_exp
    char_job_points char_profile char_points char_skills char_inventory
    char_style item_equipment skill_caps zone_settings accounts_sessions
)

cleanup() {
    if [[ "${KEEP:-0}" == "1" ]]; then
        echo; echo "left running: http://localhost:$PORT"
        echo "tear down: docker rm -f $APP $DB && docker network rm $NET"
        return
    fi
    docker rm -f "$APP" "$DB" >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

check() {
    if grep -qF -- "$3" <<<"$2"; then
        printf '  ok    %s\n' "$1"; PASSES=$((PASSES + 1))
    else
        printf '  FAIL  %s (missing: %s)\n' "$1" "$3"; FAILURES=$((FAILURES + 1))
    fi
}

docker image inspect "$IMAGE" >/dev/null 2>&1 || {
    echo "renderer image $IMAGE not found locally; build it first" >&2
    exit 1
}

echo "== bringing up MariaDB $MARIADB_TAG"
docker rm -f "$APP" "$DB" >/dev/null 2>&1 || true
docker network rm "$NET" >/dev/null 2>&1 || true
docker network create "$NET" >/dev/null
docker run -d --name "$DB" --network "$NET" \
    -e MARIADB_ROOT_PASSWORD="$DBPASS" -e MARIADB_DATABASE=xidb \
    "mariadb:$MARIADB_TAG" >/dev/null

until docker exec "$DB" mariadb -uroot -p"$DBPASS" -e 'SELECT 1' >/dev/null 2>&1; do sleep 2; done

echo '== loading schema and fixtures'
for t in "${TABLES[@]}"; do
    docker exec -i "$DB" mariadb -uroot -p"$DBPASS" xidb < "$XISERVER/sql/$t.sql"
done
docker exec -i "$DB" mariadb -uroot -p"$DBPASS" xidb < "$ROOT/tools/fixtures/seed.sql"
docker exec -i "$DB" mariadb -uroot -p"$DBPASS" <<GRANT
CREATE DATABASE IF NOT EXISTS xiportraits;
CREATE USER IF NOT EXISTS 'xiherald'@'%' IDENTIFIED BY 'heraldpass';
GRANT SELECT ON xidb.* TO 'xiherald'@'%';
GRANT ALL PRIVILEGES ON xiportraits.* TO 'xiherald'@'%';
FLUSH PRIVILEGES;
GRANT

echo '== building and starting the herald'
docker build -q -t xiherald:pipeline "$ROOT" >/dev/null
docker run -d --name "$APP" --network "$NET" -p "$PORT:8080" \
    -e XI_HERALD_DB_HOST="$DB" -e XI_HERALD_DB_USER=xiherald \
    -e XI_HERALD_DB_PASS=heraldpass -e XI_HERALD_DB_NAME=xidb \
    -e XI_HERALD_PORTRAIT_SCHEMA=xiportraits -e XI_HERALD_CACHE_TTL=0s \
    xiherald:pipeline >/dev/null

until [[ "$(curl -fsS "http://localhost:$PORT/healthz" 2>/dev/null || true)" == "ok" ]]; do sleep 1; done

echo "== running the renderer ($IMAGE)"
# Exactly the CronJob's invocation: service names, secret-supplied password.
docker run --rm --network "$NET" \
    -e XI_PORTRAIT_DB_USER=xiherald \
    -e XI_PORTRAIT_DB_PASS=heraldpass \
    -e XI_PORTRAIT_DB_NAME=xiportraits \
    "$IMAGE" \
    --herald "http://$APP:8080" --db-host "$DB" 2>&1 | sed 's/^/   /'

echo
echo '== asserting'

rows="$(docker exec "$DB" mariadb -uroot -p"$DBPASS" -N -B -e \
    'SELECT COUNT(*) FROM xiportraits.portraits' 2>/dev/null)"
check 'the renderer wrote portraits' "$(( rows > 0 ))" '1'
echo "        ($rows rows)"

sizes="$(docker exec "$DB" mariadb -uroot -p"$DBPASS" -N -B -e \
    'SELECT MIN(LENGTH(bytes)), MIN(width), MIN(height) FROM xiportraits.portraits' 2>/dev/null)"
check 'portraits have real pixels' "$(awk '{print ($1 > 10000 && $2 > 0 && $3 > 0)}' <<<"$sizes")" '1'
echo "        (smallest blob / min dimensions: $sizes)"

id="$(docker exec "$DB" mariadb -uroot -p"$DBPASS" -N -B -e \
    'SELECT charid FROM xiportraits.portraits ORDER BY charid LIMIT 1' 2>/dev/null)"
hash="$(docker exec "$DB" mariadb -uroot -p"$DBPASS" -N -B -e \
    "SELECT hash FROM xiportraits.portraits WHERE charid=$id" 2>/dev/null)"

hdr="$(curl -sS -D - -o /tmp/pipeline-portrait.png "http://localhost:$PORT/portraits/$id.png?v=$hash")"
check 'the herald serves it'        "$hdr" '200'
check 'as a png'                    "$hdr" 'image/png'
check 'cached immutably'            "$hdr" 'immutable'
check 'it is a real png on the wire' "$(file -b /tmp/pipeline-portrait.png)" 'PNG image data'

name="$(docker exec "$DB" mariadb -uroot -p"$DBPASS" -N -B -e \
    "SELECT charname FROM xidb.chars WHERE charid=$id" 2>/dev/null)"
page="$(curl -fsS "http://localhost:$PORT/player/$name")"
check 'the character page links it' "$page" "/portraits/$id.png?v=$hash"

echo '== second run should render nothing'
again="$(docker run --rm --network "$NET" \
    -e XI_PORTRAIT_DB_USER=xiherald -e XI_PORTRAIT_DB_PASS=heraldpass \
    -e XI_PORTRAIT_DB_NAME=xiportraits "$IMAGE" \
    --herald "http://$APP:8080" --db-host "$DB" 2>&1 | tail -2)"
check 'unchanged characters are skipped' "$again" 'rendered 0'

echo
echo "== $PASSES passed, $FAILURES failed"
[[ "$FAILURES" -eq 0 ]] || { docker logs "$APP" 2>&1 | tail -20; exit 1; }
