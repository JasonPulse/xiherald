#!/usr/bin/env bash
#
# Run the portrait renderer under a virtual display.
#
# Godot needs a display even to render offscreen, and --headless disables
# rendering entirely rather than making it offscreen, so Xvfb is not optional
# here. Everything is software-rasterised; there is no GPU in the cluster.
#
set -euo pipefail

# Point Vellichor at the DAT subset baked into the image. The key is
# InstallPath and the file is flat: InstallLocator scans for that exact key and
# ignores anything without an '=', so an ini section header is silently useless.
CORPUS="${XI_CORPUS_DIR:-/app/corpus}"
mkdir -p /root/.config/Vellichor
echo "InstallPath=${CORPUS}" > /root/.config/Vellichor/settings.ini

# Fail loudly here rather than rendering 101 featureless silhouettes: without a
# valid corpus Vellichor logs one line and carries on drawing nothing.
if [[ ! -f "$CORPUS/VTABLE.DAT" && ! -f "$CORPUS/ROM/VTABLE.DAT" ]]; then
    echo "no DAT archive at $CORPUS (VTABLE.DAT missing)" >&2
    exit 1
fi

exec xvfb-run -a -s "-screen 0 ${XI_RENDER_RESOLUTION:-512x768}x24" \
    python3 /app/render_portraits.py \
        --vellichor /app/vellichor \
        --godot /usr/local/bin/godot \
        "$@"
