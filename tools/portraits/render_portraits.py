#!/usr/bin/env python3
"""Render character portraits by driving Vellichor, then publish the PNGs.

The Herald cannot do this itself. Rendering needs the retail DAT archives, which
are not redistributable and are gigabytes, so this runs wherever those archives
already live and the Herald only ever links to the result.

Vellichor already has the feature; this script is the work-list and the caching
around it. It reads /api/v1/appearances, which resolves char_look model ids and,
for style-locked characters, char_style item ids through item_equipment.MId, and
hands each one to Vellichor as:

    VELLICHOR_PC="<race>,<face>"
    VELLICHOR_PC_EQUIP="head=12,body=135,..."
    VELLICHOR_MAGENTA=1          flat magenta background, so it can be keyed out
    VELLICHOR_PC_ZOOM=2.4        pull the camera in to portrait framing
    VELLICHOR_SHOT="<out>/<charid>.png"

The magenta is keyed to transparency here rather than replaced with a backdrop,
so the Herald's CSS supplies the background. Restyling it then costs nothing;
baking a backdrop in would mean re-rendering everyone.

Each render is skipped when the appearance hash matches the last one drawn, so a
normal run costs nothing for the characters who have not changed gear.

    ./render_portraits.py --herald http://xiherald.homelab.svc.cluster.local \
                          --vellichor ~/Code/Godot/Vellichor \
                          --out ./portraits

Nothing here writes to the Herald. Point a web server at --out, then set
XI_HERALD_PORTRAIT_BASE_URL to its URL.
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import urllib.request

MANIFEST = "manifest.json"


def fetch_appearances(herald):
    url = herald.rstrip("/") + "/api/v1/appearances"
    with urllib.request.urlopen(url, timeout=30) as r:
        return json.load(r)["appearances"]


def load_manifest(out_dir):
    path = os.path.join(out_dir, MANIFEST)
    if not os.path.exists(path):
        return {}
    try:
        with open(path, encoding="utf-8") as fh:
            return json.load(fh)
    except (json.JSONDecodeError, OSError):
        # A corrupt manifest costs a full re-render, which is recoverable.
        # Refusing to run because of it is not.
        print("manifest unreadable, re-rendering everything", file=sys.stderr)
        return {}


def save_manifest(out_dir, manifest):
    path = os.path.join(out_dir, MANIFEST)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(manifest, fh, indent=2, sort_keys=True)
    os.replace(tmp, path)


def render_one(args, appearance, dest):
    env = dict(os.environ)
    env["VELLICHOR_PC"] = f"{appearance['race']},{appearance['face']}"
    env["VELLICHOR_SHOT"] = dest
    env["VELLICHOR_SHOT_FRAME"] = str(args.shot_frame)
    env["VELLICHOR_MAGENTA"] = "1"
    env["VELLICHOR_PC_ZOOM"] = str(args.zoom)
    env["VELLICHOR_PC_CAM"] = str(args.camera)
    if appearance["equip_arg"]:
        env["VELLICHOR_PC_EQUIP"] = appearance["equip_arg"]

    cmd = [
        args.godot,
        "--path", args.vellichor,
        "--rendering-driver", args.driver,
        "--resolution", f"{args.width}x{args.height}",
        "--quit-after", str(args.shot_frame + 240),
    ]

    if os.path.exists(dest):
        os.unlink(dest)

    try:
        proc = subprocess.run(cmd, env=env, timeout=args.timeout,
                              capture_output=True, text=True)
    except subprocess.TimeoutExpired:
        return False, "timed out"

    if not os.path.exists(dest):
        tail = (proc.stderr or proc.stdout or "").strip().split("\n")[-3:]
        return False, "no image written: " + " | ".join(tail)

    return True, None


def postprocess(path, args):
    """Key out the magenta, trim to the character, and pad to a fixed size.

    The border after the trim matters: without it the trim leaves the feet on
    the exact bottom edge and the extent clips them.

    Skipped when ImageMagick is absent. An unkeyed portrait on a magenta field
    is ugly but present, and failing the whole run over cosmetics is worse.
    """
    magick = shutil.which("magick") or shutil.which("convert")
    if not magick:
        print("  note   ImageMagick not found, leaving the raw render",
              file=sys.stderr)
        return

    size = f"{args.portrait_width}x{args.portrait_height}"
    subprocess.run(
        [magick, path,
         "-fuzz", f"{args.key_fuzz}%", "-transparent", "#FF00FF",
         "-trim", "+repage",
         "-bordercolor", "none", "-border", "10",
         "-resize", size,
         "-background", "none", "-gravity", "center", "-extent", size,
         "-strip", path],
        capture_output=True, check=False)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--herald", required=True, help="Herald base URL")
    ap.add_argument("--vellichor", required=True, help="path to the Vellichor project")
    ap.add_argument("--out", required=True, help="directory to write PNGs into")
    ap.add_argument("--godot", default=os.environ.get("GODOT", "godot"))
    ap.add_argument("--driver", default="opengl3",
                    help="Godot rendering driver; opengl3 works under xvfb")
    ap.add_argument("--width", type=int, default=512)
    ap.add_argument("--height", type=int, default=768)
    ap.add_argument("--portrait-width", type=int, default=384)
    ap.add_argument("--portrait-height", type=int, default=576)
    ap.add_argument("--shot-frame", type=int, default=45)
    ap.add_argument("--zoom", type=float, default=2.4,
                    help="camera zoom; higher is closer")
    ap.add_argument("--camera", type=float, default=270,
                    help="orbit degrees; 270 faces the character at the camera")
    ap.add_argument("--key-fuzz", type=int, default=18,
                    help="tolerance when keying out the magenta background")
    ap.add_argument("--timeout", type=int, default=180)
    ap.add_argument("--only", help="render just this character name")
    ap.add_argument("--force", action="store_true", help="ignore the manifest")
    ap.add_argument("--limit", type=int, help="stop after this many renders")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)

    appearances = fetch_appearances(args.herald)
    manifest = {} if args.force else load_manifest(args.out)

    rendered = skipped = failed = unrenderable = 0

    for a in appearances:
        if args.only and a["name"].lower() != args.only.lower():
            continue

        key = str(a["character_id"])
        dest = os.path.join(args.out, key + ".png")

        if not a["renderable"]:
            print(f"  skip   {a['name']}: no appearance data")
            unrenderable += 1
            continue

        if manifest.get(key) == a["hash"] and os.path.exists(dest):
            skipped += 1
            continue

        if args.limit is not None and rendered >= args.limit:
            break

        style = " (style-locked)" if a["style_locked"] else ""
        print(f"  render {a['name']} [{a['race_name']}]{style}")

        ok, err = render_one(args, a, dest)
        if not ok:
            print(f"  FAIL   {a['name']}: {err}", file=sys.stderr)
            failed += 1
            continue

        postprocess(dest, args)
        manifest[key] = a["hash"]
        rendered += 1
        # Written every time so an interrupted run keeps what it finished.
        save_manifest(args.out, manifest)

    save_manifest(args.out, manifest)

    print(f"\nrendered {rendered}, unchanged {skipped}, "
          f"no data {unrenderable}, failed {failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
