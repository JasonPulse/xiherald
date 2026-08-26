#!/usr/bin/env bash
#
# Build the Herald's generated web assets.
#
# There are only two: the logo derivatives, and a noise tile used to keep the
# panels from reading as flat black. Every frame, bracket, rule and bar in the
# site is drawn with CSS gradients or with the hand-authored
# internal/web/static/img/fleuron.svg, so there is nothing else to generate and
# no third-party asset in the pipeline.
#
# Vollkorn is not built here. The woff2 files are committed, taken from the
# upstream Vollkorn typeface under the SIL Open Font License, whose text ships
# beside them as Vollkorn-OFL.txt.
#
# Requires: imagemagick.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMG="$ROOT/internal/web/static/img"
LOGO="${LOGO:-$ROOT/assets/logo/ffxi-herald-logo.jpg}"

mkdir -p "$IMG"

echo '== panel noise'
# 128px and 24 greys, because gaussian noise is incompressible and a full 8-bit
# 256px tile costs 116 KB for texture nobody can see at overlay strength.
# -seed keeps the build reproducible.
magick -size 128x128 xc:'#808080' -seed 42 -attenuate 0.55 +noise Gaussian \
    -colorspace Gray -blur 0x0.4 -colors 24 -strip "$IMG/noise.png"

echo '== logo'
if [[ ! -f "$LOGO" ]]; then
    echo "no logo at $LOGO" >&2
    exit 1
fi

# Full banner for the page hero.
magick "$LOGO" -resize 1200x -strip -quality 82 "$IMG/logo-banner.jpg"
# The crown-and-shield centre doubles as the nav mark and the favicon.
magick "$LOGO" -crop 480x480+490+60 +repage -resize 256x256 -strip "$IMG/logo-mark.png"
magick "$IMG/logo-mark.png" -resize 64x64 -strip "$IMG/favicon.png"

echo '== done'
ls -l "$IMG"
