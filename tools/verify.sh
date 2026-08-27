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
    char_style item_equipment skill_caps zone_settings accounts_sessions
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

echo '== creating the portrait database'
docker exec -i "$DB" mariadb -uroot -p"$DBPASS" -e 'CREATE DATABASE IF NOT EXISTS xiportraits;'

echo '== granting the herald user'
docker exec -i "$DB" mariadb -uroot -p"$DBPASS" <<GRANT
CREATE USER IF NOT EXISTS 'xiherald'@'%' IDENTIFIED BY 'heraldpass';
GRANT SELECT ON xidb.* TO 'xiherald'@'%';
GRANT ALL PRIVILEGES ON xiportraits.* TO 'xiherald'@'%';
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
    -e XI_HERALD_PORTRAIT_SCHEMA=xiportraits \
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

# Unfinished character slots have a blank charname. Fixture charid 6 is one,
# and it must be absent from every list, every count and every board.
check 'roster excludes the unnamed slot' \
    "$roster" '<span class="tile-n">5</span><span class="tile-l">Characters</span>'
refute 'roster has no empty name cell'  "$roster" '<a href="/player/"></a>'
refute 'roster has no empty name link'  "$roster" 'href="/player/"'

unnamed="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/player/")"
check 'a blank character name is not a page' "$unnamed" '404'

# Deaths come from char_history.times_knocked_out. char_stats.death is a
# weakness timer that reads zero for anyone not currently dead, and the fixture
# sets it to zero throughout, so any of these showing 0 means the old field is
# back.
check 'summary deaths use knockouts' \
    "$roster" '<span class="tile-n">1,125</span><span class="tile-l">Deaths</span>'
check 'roster deaths use knockouts'  "$roster" '812'

deathboard="$(curl -fsS "http://localhost:$PORT/stats/deaths")"
check 'deaths board counts knockouts' "$deathboard" '812'
refute 'deaths board is not all zero'  "$deathboard" 'Nobody has scored'

gone="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/stats/knockouts")"
check 'the duplicate knockouts board is gone' "$gone" '404'

levels="$(curl -fsS "http://localhost:$PORT/stats/levels")"
refute 'boards exclude the unnamed slot' "$levels" 'href="/player/"'

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

# ---- missions -------------------------------------------------------------
# chars.missions is a raw missionlog_t[15] blob. The fixture encodes one by
# hand, so these numbers exercise the decoder rather than echo it: a nation log
# mid-chain, an expansion log finished, a ToAU mission in progress, CoP whose
# completion is linear off current, and SoA whose ids exceed the 64-slot array.
check 'nation mission in progress'   "$player" 'The Heir to the Light'
check 'nation log counts completions' "$player" '23 / 24'
check 'toau mission shows its number' "$player" 'ToAU 41'
check 'toau mission shows its name'   "$player" 'Path of Darkness'
check 'toau counts completions'       "$player" '41 / 48'
check 'a finished log says complete'  "$player" 'Complete'
check 'zilart is finished'            "$player" '18 / 18'
# CoP ids are large and non-sequential, so this also proves the id is read as
# a mission id and not as an ordinal.
check 'cop completion is linear'      "$player" '20 / 43'
# SoA has ids past 64, which take the linear branch while lower ids use the bits.
check 'soa mixes both rules'          "$player" '55 / 105'

apimissions="$(curl -fsS "http://localhost:$PORT/api/v1/characters/Aldwyn")"
check 'api reports the toau log'      "$apimissions" '"short": "ToAU"'
check 'api reports current id'        "$apimissions" '"current_id": 41'
check 'api reports current name'      "$apimissions" '"current": "Path of Darkness"'
check 'api reports completed'         "$apimissions" '"completed": 41'
check 'api reports total'             "$apimissions" '"total": 48'
check 'api marks a finished log'      "$apimissions" '"finished": true'
check 'api reports every log'         "$apimissions" '"log": "Wings of the Goddess"'

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

# Caching contract. A stylesheet edit has to be able to reach a browser that
# already loaded the old one, which means the URL in the HTML must carry a
# content hash and the page itself must be revalidated.
check 'stylesheet URL is content-addressed' "$roster" '/static/css/herald.css?v='
check 'logo URL is content-addressed'       "$roster" '/static/img/logo-banner.jpg?v='

pagehdr="$(curl -fsS -D - -o /dev/null "http://localhost:$PORT/")"
check 'pages are revalidated' "$pagehdr" 'no-cache'

ver="$(curl -fsS "http://localhost:$PORT/" | sed -n 's|.*herald\.css?v=\([a-f0-9]*\).*|\1|p' | head -1)"
verhdr="$(curl -fsS -D - -o /dev/null "http://localhost:$PORT/static/css/herald.css?v=$ver")"
check 'fingerprinted assets are immutable' "$verhdr" 'max-age=31536000, immutable'

barehdr="$(curl -fsS -D - -o /dev/null "http://localhost:$PORT/static/css/herald.css")"
check 'unfingerprinted assets cache briefly' "$barehdr" 'max-age=300'
refute 'and are never immutable'             "$barehdr" 'immutable'

# Bastok is blue. Gold here means the nation palette regressed.
servedcss="$(curl -fsS "http://localhost:$PORT/static/css/herald.css")"
check 'Bastok is blue'      "$servedcss" '#6b8fd4'
refute 'Bastok is not gold' "$servedcss" '#cfa14a'

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


# ---- JSON API ------------------------------------------------------------
# Consumed by a public-facing guild site, so the contract is asserted rather
# than assumed: stable field names, arrays never null, and no gil anywhere.

apiidx="$(curl -fsS "http://localhost:$PORT/api/v1")"
check 'api index lists characters'   "$apiidx" '/api/v1/characters'
check 'api index states no gil'      "$apiidx" 'Gil is not exposed'

apilist="$(curl -fsS "http://localhost:$PORT/api/v1/characters")"
check 'api lists characters'         "$apilist" '"name": "Aldwyn"'
check 'api reports a count'          "$apilist" '"count": 5'
check 'api names the nation'         "$apilist" "\"nation\": \"San d'Oria\""
check 'api gives kd as a number'     "$apilist" '"kill_death_ratio":'
check 'api deaths use knockouts'     "$apilist" '"deaths": 812'
refute 'api omits the unnamed slot'  "$apilist" '"name": ""'

apichar="$(curl -fsS "http://localhost:$PORT/api/v1/characters/Aldwyn")"
check 'api character has jobs'       "$apichar" '"abbrev": "SAM"'
check 'api character has skills'     "$apichar" '"name": "Great Katana"'
check 'api flattens skill groups'    "$apichar" '"group": "Combat"'
check 'api character has crafts'     "$apichar" '"rank": "Veteran"'
check 'api character has history'    "$apichar" '"distance_travelled": 18402913'
check 'api character has fame'       "$apichar" '"area": "San d'"'"'Oria"'

# Gil is on the private HTML page and must not reach the API.
for doc in "$apilist" "$apichar"; do
    refute 'api never exposes gil' "$doc" 'gil'
done
refute 'api never exposes a gil figure' "$apichar" '48210934'

# ---- portraits ------------------------------------------------------------
# Portraits live in their own database with write access, while game data stays
# read-only. Both halves of that split are asserted, because the whole point of
# the separate schema is that one cannot become the other.
check 'portraits are enabled'        "$(docker logs "$APP" 2>&1)" 'portraits enabled'

# The Herald creates its own table, so a fresh database needs no migration step.
tbl="$(docker exec "$DB" mariadb -uroot -p"$DBPASS" -N -B -e \
    "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='xiportraits' AND table_name='portraits'")"
check 'the herald created the table' "$tbl" '1'

# An unrendered character must not emit an img tag that 404s on every view.
refute 'no img tag without a portrait' "$player" '/portraits/1.png'
missing_png="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/portraits/1.png")"
check 'an unrendered portrait is 404' "$missing_png" '404'
bad_png="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/portraits/notanumber.png")"
check 'a bad portrait id is 404'      "$bad_png" '404'

# Insert one directly, the way the renderer does, and check it is served.
docker exec -i "$DB" mariadb -uroot -p"$DBPASS" xiportraits <<'PNG'
INSERT INTO portraits (charid, hash, content_type, width, height, bytes)
VALUES (1, 'deadbeefcafe', 'image/png', 384, 576, UNHEX('89504E470D0A1A0A'))
ON DUPLICATE KEY UPDATE hash = VALUES(hash), bytes = VALUES(bytes);
PNG

served="$(curl -sS -D - -o /dev/null "http://localhost:$PORT/portraits/1.png?v=deadbeefcafe")"
check 'a stored portrait is served'   "$served" '200'
check 'portraits are sent as png'     "$served" 'image/png'
check 'portrait carries an etag'      "$served" 'deadbeefcafe'
check 'hashed portrait is immutable'  "$served" 'max-age=31536000, immutable'
bare="$(curl -sS -D - -o /dev/null "http://localhost:$PORT/portraits/1.png")"
check 'unhashed portrait caches briefly' "$bare" 'max-age=300'

# With a portrait present the page must link it, and with the hash so that a
# re-render is never hidden behind a cached image.
withimg="$(curl -fsS "http://localhost:$PORT/player/Aldwyn")"
check 'page links the portrait'       "$withimg" '/portraits/1.png?v=deadbeefcafe'

# The read-only guarantee over game data is unchanged.
denied="$(docker exec -i "$DB" mariadb -uxiherald -pheraldpass xidb \
    -e "UPDATE chars SET charname='hacked' WHERE charid=1" 2>&1 || true)"
check 'game data is still read-only'  "$denied" 'denied'

# ---- appearances ----------------------------------------------------------
# The render work-list. char_look holds model ids, char_style holds item ids
# that only mean anything after item_equipment.MId, and the two must not be
# confused: Aldwyn is style-locked in the fixture with different items to his
# real gear, so these numbers prove which source was used.
apilook="$(curl -fsS "http://localhost:$PORT/api/v1/appearances")"
check 'appearances are listed'        "$apilook" '"appearances"'
check 'appearance names the race'     "$apilook" '"race_name": "Elvaan"'
check 'appearance carries a hash'     "$apilook" '"hash":'
check 'appearance builds an equip arg' "$apilook" '"equip_arg":'
refute 'unnamed slots are excluded'   "$apilook" '"name": "",'

# Aldwyn: style-locked, so head must be the model behind the style item (250)
# and not the raw char_look value (98).
check 'style lock resolves via MId'   "$apilook" '"head": 323'
refute 'style lock ignores raw look'  "$apilook" '"head": 98'
check 'style lock is reported'        "$apilook" '"style_locked": true'

# Muunbeam: not style-locked, so char_look wins even though a style row exists.
check 'style lock body via MId'       "$apilook" '"body": 5'
check 'unlocked look is used'         "$apilook" '"body": 61'

look1="$(curl -fsS "http://localhost:$PORT/api/v1/characters/Aldwyn")"
check 'character carries appearance'  "$look1" '"appearance":'

apiboards="$(curl -fsS "http://localhost:$PORT/api/v1/leaderboards")"
check 'api lists leaderboards'       "$apiboards" '"slug": "deaths"'
refute 'api dropped the duplicate'   "$apiboards" '"slug": "knockouts"'

apiboard="$(curl -fsS "http://localhost:$PORT/api/v1/leaderboards/kills")"
check 'api board is ranked'          "$apiboard" '"rank": 1'
check 'api board carries values'     "$apiboard" '"value": 148902'

apisum="$(curl -fsS "http://localhost:$PORT/api/v1/summary")"
check 'api summary counts characters' "$apisum" '"characters": 5'
check 'api summary uses knockouts'    "$apisum" '"total_deaths": 1125'

apimiss="$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/api/v1/characters/Nobody")"
check 'api unknown character is 404' "$apimiss" '404'
apijson="$(curl -fsS -D - -o /dev/null "http://localhost:$PORT/api/v1/summary")"
check 'api serves json'              "$apijson" 'application/json'

echo
echo "== $PASSES passed, $FAILURES failed"

if [[ "$FAILURES" -gt 0 ]]; then
    echo
    echo '-- herald log'
    docker logs "$APP" 2>&1 | tail -30
    exit 1
fi
