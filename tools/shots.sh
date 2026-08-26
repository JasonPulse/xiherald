#!/usr/bin/env bash
#
# Screenshot the running Herald with headless Chromium.
#
# Run KEEP=1 ./tools/verify.sh first, then this. Chromium joins the verify
# network and talks to the app container directly, which sidesteps Docker
# Desktop's host-port behaviour on macOS.
#
set -euo pipefail

OUT="${OUT:-/tmp/xiherald-shots}"
NET="${NET:-xiherald-verify}"
HOST="${HOST:-xiherald-verify-app:8080}"
WIDTH="${WIDTH:-1360}"
HEIGHT="${HEIGHT:-2400}"

mkdir -p "$OUT"

shot() {
    local name="$1" path="$2"
    docker run --rm --network "$NET" -v "$OUT":/shots \
        --entrypoint chromium-browser zenika/alpine-chrome \
        --headless --no-sandbox --disable-gpu --disable-dev-shm-usage \
        --hide-scrollbars \
        --window-size="$WIDTH,$HEIGHT" \
        --virtual-time-budget=5000 \
        --screenshot="/shots/$name.png" \
        "http://$HOST$path" >/dev/null 2>&1
    echo "$OUT/$name.png  <-  $path"
}

shot roster      /
shot player      /player/Aldwyn
shot stats       /stats
shot leaderboard /stats/kd
