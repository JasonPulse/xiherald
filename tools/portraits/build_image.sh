#!/usr/bin/env bash
#
# Build the portrait renderer image on the workstation.
#
# This has to run here rather than in CI: the image needs the retail DAT
# archives, which are gitignored out of every repository by design.
#
# The result contains game data. Push it to a PRIVATE registry only.
#
#   docker login
#   ./build_image.sh --vellichor ~/Code/Godot/Vellichor --push
#
# The default tag is jasonpulse/vellichor-renderer:latest. That Docker Hub
# repository must be PRIVATE: the image carries retail game data.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VELLICHOR=""
TAG="jasonpulse/vellichor-renderer:latest"
PLATFORM="linux/arm64"
PUSH=0
BUILDER="${BUILDER:-mybuilder}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --vellichor) VELLICHOR="$2"; shift 2 ;;
        --tag)       TAG="$2"; shift 2 ;;
        --platform)  PLATFORM="$2"; shift 2 ;;
        --push)      PUSH=1; shift ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

[[ -n "$VELLICHOR" ]] || { echo "--vellichor is required" >&2; exit 1; }
[[ -d "$VELLICHOR" ]] || { echo "no such directory: $VELLICHOR" >&2; exit 1; }

STAGE="$(mktemp -d -t vellichor-renderer-XXXXXX)"
trap 'rm -rf "$STAGE"' EXIT
echo "staging in $STAGE"

echo '== extracting the DAT subset'
python3 "$HERE/extract_corpus.py" --vellichor "$VELLICHOR" --out "$STAGE/corpus"

echo '== staging the Vellichor project'
# Everything except the full corpus, build output and editor scratch. Copying
# the 10 GiB corpus into the build context would defeat the whole point.
rsync -a --quiet \
    --exclude 'corpus/' --exclude '.godot/' --exclude 'bin/' --exclude 'obj/' \
    --exclude '.git/' --exclude 'export/' \
    "$VELLICHOR"/ "$STAGE/vellichor/"

cp "$HERE/render_portraits.py" "$HERE/entrypoint.sh" "$HERE/Dockerfile" "$STAGE/"

echo "== building $TAG for $PLATFORM"
du -sh "$STAGE" | sed 's/^/   context: /'

args=(buildx build --builder "$BUILDER" --platform "$PLATFORM"
      --provenance=false -t "$TAG" "$STAGE")
if [[ "$PUSH" == 1 ]]; then
    args+=(--push)
    echo '   NOTE: this image contains retail game data. The target registry'
    echo '         must be private.'
else
    args+=(--load)
fi

docker "${args[@]}"
echo "done: $TAG"
