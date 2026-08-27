#!/usr/bin/env bash
#
# Run the portrait renderer under a virtual display.
#
# Godot needs a display even to render offscreen: --headless disables rendering
# rather than making it offscreen, so an X server is not optional here.
#
# Xvfb is started explicitly rather than through xvfb-run. xvfb-run starts the
# server and then blocks in `wait` for a SIGUSR1 readiness signal from it; if
# that signal is missed the wrapper waits forever and never launches the
# command. That is exactly what happened on an arm64 node: Xvfb up and idle,
# xvfb-run parked in sigsuspend, no renderer process at all, and no output for
# twenty-five minutes. It worked locally, because the race is timing-dependent.
#
# Waiting for the socket to appear is deterministic and cannot be missed.
#
set -euo pipefail

DISPLAY_NUM="${XI_DISPLAY_NUM:-99}"
RESOLUTION="${XI_RENDER_RESOLUTION:-512x768}"
CORPUS="${XI_CORPUS_DIR:-/app/corpus}"

# Point Vellichor at the DAT subset baked into the image. The key is
# InstallPath and the file is flat: InstallLocator scans for that exact key and
# ignores any line without an '=', so an ini section header is silently useless.
mkdir -p /root/.config/Vellichor
echo "InstallPath=${CORPUS}" > /root/.config/Vellichor/settings.ini

# Fail loudly rather than rendering a hundred empty silhouettes: without a
# valid corpus Vellichor logs one line and carries on drawing nothing.
if [[ ! -f "$CORPUS/VTABLE.DAT" && ! -f "$CORPUS/ROM/VTABLE.DAT" ]]; then
    echo "no DAT archive at $CORPUS (VTABLE.DAT missing)" >&2
    exit 1
fi

Xvfb ":${DISPLAY_NUM}" -screen 0 "${RESOLUTION}x24" -nolisten tcp &
XVFB_PID=$!
trap 'kill "$XVFB_PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 100); do
    if [[ -e "/tmp/.X11-unix/X${DISPLAY_NUM}" ]]; then
        break
    fi
    if ! kill -0 "$XVFB_PID" 2>/dev/null; then
        echo "Xvfb exited before the display appeared" >&2
        exit 1
    fi
    sleep 0.2
done

if [[ ! -e "/tmp/.X11-unix/X${DISPLAY_NUM}" ]]; then
    echo "Xvfb never created display :${DISPLAY_NUM}" >&2
    exit 1
fi

export DISPLAY=":${DISPLAY_NUM}"
echo "display :${DISPLAY_NUM} ready (${RESOLUTION})"

exec python3 -u /app/render_portraits.py \
    --vellichor /app/vellichor \
    --godot /usr/local/bin/godot \
    "$@"
