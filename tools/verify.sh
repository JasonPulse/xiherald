#!/usr/bin/env bash
#
# Prove the Herald works, end to end, against a real MariaDB carrying the real
# xiserver schema.
#
# It stands up MariaDB, loads the actual CREATE TABLE statements from the
# xiserver repo (not a hand-written copy, so a schema change upstream shows up
# here as a failure), seeds fixture characters, builds the Herald image, and
# asserts on the rendered HTML of every page.
#
#   ./tools/verify.sh                 # run everything, tear down after
#   KEEP=1 ./tools/verify.sh          # leave it running on http://localhost:8099
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
XISERVER="${XISERVER:-$ROOT/../xiserver}"
NET=xiherald-verify
DB=xiherald-verify-db
APP=xiherald-verify-app
PORT="${PORT:-8099}"
# The cluster runs arm64. Set ARCH=linux/arm64 to run this whole suite against
# the arm64 image instead of the host's native one.
ARCH="${ARCH:-}"
# Set IMAGE to verify an already-published image instead of building from this
# working tree, e.g. IMAGE=ghcr.io/jasonpulse/xiherald:latest to check that what
# the cluster would pull actually serves pages.
IMAGE="${IMAGE:-}"
# The server version is a real variable in this system: MariaDB renamed
# transaction_isolation in 11.1, and testing only against :11 once hid a bug
# that broke every page on an older server. Default low, override to sweep.
MARIADB_TAG="${MARIADB_TAG:-10.11}"
DBPASS=verify
PASSES=0
FAILURES=0

# Only the tables the Herald reads. Loading the whole sql/ directory would pull
# in 40 MB of item data the Herald never touches.
TABLES=(
    chars char_stats char_look char_history char_jobs char_exp
    char_job_points char_profile char_points char_skills char_inventory
    skill_caps zone_settings accounts_sessions
)

cleanup() {
    if [[ "${KEEP:-0}" == "1" ]]; then
        echo
        echo "left running: http://localhost:$PORT"
        echo "tear down with: docker rm -f $APP $DB && docker network rm $NET"
        return
    fi
    docker rm -f "$APP" "$DB" >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

check() {
    local label="$1" haystack="$2" needle="$3"
    if grep -qF -- "$needle" <<<"$haystack"; then
        printf '  ok    %s\n' "$label"
        PASSES=$((PASSES + 1))
    else
        printf '  FAIL  %s (missing: %s)\n' "$label" "$needle"
        FAILURES=$((FAILURES + 1))
    fi
}

refute() {
    local label="$1" haystack="$2" needle="$3"
    if grep -qF -- "$needle" <<<"$haystack"; then
        printf '  FAIL  %s (unexpected: %s)\n' "$label" "$needle"
        FAILURES=$((FAILURES + 1))
    else
        printf '  ok    %s\n' "$label"
        PASSES=$((PASSES + 1))
    fi
}

if [[ ! -d "$XISERVER/sql" ]]; then
    echo "xiserver sql/ not found at $XISERVER/sql; set XISERVER=" >&2
    exit 1
fi

# The published workflow fails the build on either of these, so the local loop
# checks them first rather than finding out from CI.
if [[ -z "$IMAGE" ]]; then
echo '== gofmt and vet'
fmtout="$(docker run --rm -v "$ROOT":/src -w /src golang:1.24-alpine gofmt -l . || true)"
if [[ -n "$fmtout" ]]; then
    echo 'not gofmt clean:' >&2
    echo "$fmtout" >&2
    exit 1
fi
"$ROOT/tools/go.sh" vet ./...
fi

echo "== bringing up MariaDB $MARIADB_TAG"
docker rm -f "$APP" "$DB" >/dev/null 2>&1 || true
docker network rm "$NET" >/dev/null 2>&1 || true
docker network create "$NET" >/dev/null

docker run -d --name "$DB" --network "$NET" \
    -e MARIADB_ROOT_PASSWORD="$DBPASS" \
    -e MARIADB_DATABASE=xidb \
    "mariadb:$MARIADB_TAG" >/dev/null

for _ in $(seq 1 60); do
    if docker exec "$DB" mariadb -uroot -p"$DBPASS" -e 'SELECT 1' >/dev/null 2>&1; then
        break
    fi
    sleep 2
done
docker exec "$DB" mariadb -uroot -p"$DBPASS" -e 'SELECT 1' >/dev/null

echo '== loading the real xiserver schema'
for table in "${TABLES[@]}"; do
    file="$XISERVER/sql/$table.sql"
    [[ -f "$file" ]] || { echo "missing $file" >&2; exit 1; }
    docker exec -i "$DB" mariadb -uroot -p"$DBPASS" xidb < "$file"
done

echo '== seeding fixtures'
docker exec -i "$DB" mariadb -uroot -p"$DBPASS" xidb < "$ROOT/tools/fixtures/seed.sql"

echo '== granting the read-only herald user'
docker exec -i "$DB" mariadb -uroot -p"$DBPASS" <<GRANT
CREATE USER IF NOT EXISTS 'xiherald'@'%' IDENTIFIED BY 'heraldpass';
GRANT SELECT ON xidb.* TO 'xiherald'@'%';
FLUSH PRIVILEGES;
GRANT

if [[ -n "$IMAGE" ]]; then
    echo "== pulling $IMAGE${ARCH:+ ($ARCH)}"
    docker pull ${ARCH:+--platform "$ARCH"} -q "$IMAGE" >/dev/null
    docker tag "$IMAGE" xiherald:verify
elif [[ -n "$ARCH" ]]; then
    echo "== building the herald image for $ARCH"
    docker buildx build --builder "${BUILDER:-mybuilder}" --platform "$ARCH" \
        --provenance=false --load -t xiherald:verify "$ROOT" >/dev/null
else
    echo '== building the herald image'
    docker build -q -t xiherald:verify "$ROOT" >/dev/null
fi
echo "   image $(docker image inspect xiherald:verify --format '{{.Architecture}}')," \
     "$(( $(docker image inspect xiherald:verify --format '{{.Size}}') / 1048576 )) MiB"

echo '== starting the herald'
docker run -d --name "$APP" --network "$NET" -p "$PORT:8080" \
    -e XI_HERALD_DB_HOST="$DB" \
    -e XI_HERALD_DB_USER=xiherald \
    -e XI_HERALD_DB_PASS=heraldpass \
    -e XI_HERALD_DB_NAME=xidb \
    -e XI_HERALD_SERVER_NAME="Vana'diel" \
    -e XI_HERALD_CACHE_TTL=0s \
    xiherald:verify >/dev/null

for _ in $(seq 1 40); do
    if [[ "$(curl -fsS "http://localhost:$PORT/healthz" 2>/dev/null || true)" == "ok" ]]; then
        break
    fi
    sleep 1
done

echo
echo '== asserting'

health="$(curl -fsS "http://localhost:$PORT/healthz")"
check 'healthz reports ok' "$health" 'ok'
live="$(curl -fsS "http://localhost:$PORT/livez")"
check 'livez reports ok'   "$live" 'ok'

roster="$(curl -fsS "http://localhost:$PORT/")"
check 'roster lists Aldwyn'            "$roster" 'Aldwyn'
check 'roster lists Muunbeam'          "$roster" 'Muunbeam'
check 'roster lists the never-played'  "$roster" 'Neverwas'
check 'roster shows a nation'          "$roster" 'd&#39;Oria'
check 'roster shows a race'            "$roster" 'Tarutaru'
check 'roster shows main/sub jobs'     "$roster" 'SAM99'
check 'roster shows master level'      "$roster" '(ML12)'
check 'roster counts jobs at 99'       "$roster" 'Jobs 99'
check 'roster groups thousands'        "$roster" '148,902'
check 'roster resolves the zone name'  "$roster" 'Windurst'
refute 'roster leaks a template error' "$roster" 'no such template'
refute 'roster leaves a raw action'    "$roster" '{{'

sorted="$(curl -fsS "http://localhost:$PORT/?sort=kills")"
check 'sort=kills is accepted'         "$sorted" 'Enemies defeated'
bogus="$(curl -fsS "http://localhost:$PORT/?sort=%27%3B+DROP+TABLE+chars%3B+--")"
check 'a bogus sort falls back safely' "$bogus" 'Aldwyn'

player="$(curl -fsS "http://localhost:$PORT/player/Aldwyn")"
check 'player page names the character' "$player" 'Aldwyn'
check 'player page decodes the title'   "$player" 'title-line'
check 'player page shows every job'     "$player" 'RUN'
check 'player page shows gil'           "$player" '48,210,934'
check 'player page shows a skill'       "$player" 'Great Katana'
check 'player page shows the skill cap' "$player" 'cap 424'
check 'player page shows a craft rank'  "$player" 'Veteran'
check 'player page shows craft tenths'  "$player" '96.4'
check 'player page shows fame'          "$player" '9,999'
check 'player page shows the record'    "$player" 'Distance travelled'
check 'player page shows job points'    "$player" '1204 JP'

fresh="$(curl -fsS "http://localhost:$PORT/player/Neverwas")"
check 'a never-played character renders' "$fresh" 'Neverwas'
refute 'and does not divide by zero'     "$fresh" 'NaN'

missing="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/player/Nobody")"
check 'an unknown character is a 404' "$missing" '404'

stats="$(curl -fsS "http://localhost:$PORT/stats")"
check 'stats index lists a combat board'      "$stats" 'Enemies defeated'
check 'stats index lists an activity board'   "$stats" 'Distance travelled'
check 'stats index lists a progression board' "$stats" 'Jobs at 99'

board="$(curl -fsS "http://localhost:$PORT/stats/kd")"
check 'the K/D board renders'      "$board" 'Kill / death ratio'
check 'the K/D board computes'     "$board" '183.38'
check 'the board marks the podium' "$board" 'podium p1'

hours="$(curl -fsS "http://localhost:$PORT/stats/playtime")"
check 'the playtime board converts to hours' "$hours" '413'

capped="$(curl -fsS "http://localhost:$PORT/stats/capped")"
check 'the jobs-at-99 board counts' "$capped" 'Jobs at 99'

badboard="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/stats/nonsense")"
check 'an unknown board is a 404' "$badboard" '404'

css="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/static/css/herald.css")"
check 'the stylesheet is served' "$css" '200'
logo="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/static/img/logo-banner.jpg")"
check 'the logo is served' "$logo" '200'
font="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/static/fonts/Vollkorn-VariableFont_wght.woff2")"
check 'the font is served' "$font" '200'
fleuron="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/static/img/fleuron.svg")"
check 'the fleuron is served' "$fleuron" '200'
noise="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/static/img/noise.png")"
check 'the noise tile is served' "$noise" '200'

# Nothing traced from the paid kit may ship in a public repository.
for traced in panel-ornate.png panel-plain.png button.png rule.png seal.png; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/static/img/$traced")"
    check "no licensed asset: $traced" "$code" '404'
done

echo
echo "== $PASSES passed, $FAILURES failed"

if [[ "$FAILURES" -gt 0 ]]; then
    echo
    echo '-- herald log'
    docker logs "$APP" 2>&1 | tail -30
    exit 1
fi
