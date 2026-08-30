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

Rendered PNGs are written into the portrait database, which the Herald reads
and serves. Nothing is written to the Herald itself and nothing is left on disk,
so there is no file to back up and no upload endpoint to secure.

The hash already stored beside each portrait is the cache: a character whose
appearance has not moved is skipped without rendering.

This runs on the workstation, because that is where Vellichor and the retail
DAT archives are. Nothing about the renderer belongs in the cluster: only the
finished PNGs go there, into the portrait database.

With --kubectl it manages that gap itself. It reads the database password from
the Herald's own Kubernetes secret using the local kubectl, opens port-forwards
to MariaDB and to the Herald on free local ports, renders, and tears them down.
That makes the whole job one command with no credentials to paste and no
tunnels to babysit:

    ./render_portraits.py --kubectl --vellichor ~/Code/Godot/Vellichor

Requires PyMySQL (see requirements.txt).
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import atexit
import contextlib
import hashlib
import socket
import tempfile
import time
import urllib.request

import pymysql


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def kubectl(args, *rest, capture=True):
    cmd = ["kubectl"]
    if args.kube_context:
        cmd += ["--context", args.kube_context]
    cmd += ["-n", args.kube_namespace, *rest]
    return subprocess.run(cmd, capture_output=capture, text=True, check=True)


def secret_value(args):
    """Read the database password from the Herald's secret.

    Deliberately fetched with the operator's own kubectl rather than being
    passed in on a command line, so the password never lands in shell history
    or in a file.
    """
    out = kubectl(args, "get", "secret", args.kube_secret,
                  "-o", f"jsonpath={{.data.{args.kube_secret_key}}}").stdout
    if not out.strip():
        raise SystemExit(f"secret {args.kube_secret}/{args.kube_secret_key} is empty")
    import base64
    return base64.b64decode(out).decode()


def wait_for_port(port, timeout=20):
    deadline = time.time() + timeout
    while time.time() < deadline:
        with contextlib.suppress(OSError):
            with socket.create_connection(("127.0.0.1", port), timeout=1):
                return True
        time.sleep(0.3)
    return False


@contextlib.contextmanager
def port_forward(args, target, remote_port):
    """Hold a kubectl port-forward open for the duration of the block."""
    local = free_port()
    cmd = ["kubectl"]
    if args.kube_context:
        cmd += ["--context", args.kube_context]
    cmd += ["-n", args.kube_namespace, "port-forward", target,
            f"{local}:{remote_port}"]

    proc = subprocess.Popen(cmd, stdout=subprocess.DEVNULL,
                            stderr=subprocess.PIPE, text=True)
    # Registered as well as finally-closed, so a hard exit still cleans up.
    atexit.register(proc.terminate)

    try:
        if not wait_for_port(local):
            proc.terminate()
            err = (proc.stderr.read() if proc.stderr else "").strip()
            raise SystemExit(f"port-forward to {target} never came up: {err}")
        yield local
    finally:
        proc.terminate()
        with contextlib.suppress(subprocess.TimeoutExpired):
            proc.wait(timeout=5)


def fetch_appearances(herald):
    url = herald.rstrip("/") + "/api/v1/appearances"
    with urllib.request.urlopen(url, timeout=30) as r:
        return json.load(r)["appearances"]


SCHEMA_DDL = """CREATE TABLE IF NOT EXISTS {schema}.portraits (
  charid       int(10) unsigned NOT NULL,
  hash         varchar(32)      NOT NULL,
  content_type varchar(32)      NOT NULL DEFAULT 'image/png',
  width        smallint(5) unsigned NOT NULL DEFAULT 0,
  height       smallint(5) unsigned NOT NULL DEFAULT 0,
  bytes        mediumblob       NOT NULL,
  rendered_at  timestamp        NOT NULL DEFAULT CURRENT_TIMESTAMP
                                ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (charid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci"""


def connect(args):
    """Open the portrait database and make sure the table exists.

    The Herald creates this table too. Both are CREATE TABLE IF NOT EXISTS with
    the same definition, so whichever runs first wins and neither depends on
    the other having run.
    """
    conn = pymysql.connect(
        host=args.db_host, port=args.db_port, user=args.db_user,
        password=args.db_pass, autocommit=True, connect_timeout=10)
    with conn.cursor() as cur:
        cur.execute(SCHEMA_DDL.format(schema=args.db_schema))
    return conn


def stored_key(appearance_hash, renderer_version):
    """The value stored beside a portrait, and its cache key.

    Deliberately not the bare appearance hash. A portrait is a function of both
    the appearance and the renderer, so improving the renderer has to invalidate
    every portrait; otherwise every character is skipped and the improvement
    never reaches anyone.

    It also doubles as the URL version, so a re-render changes the URL and a
    consumer caching immutably still picks the new image up.
    """
    digest = hashlib.sha256(f"{appearance_hash}|{renderer_version}".encode())
    return digest.hexdigest()[:12]


def stored_hashes(conn, schema):
    with conn.cursor() as cur:
        cur.execute(f"SELECT charid, hash FROM {schema}.portraits")
        return {str(cid): h for cid, h in cur.fetchall()}


def store_portrait(conn, schema, char_id, appearance_hash, path):
    with open(path, "rb") as fh:
        blob = fh.read()

    width = height = 0
    # PNG header: 8-byte signature, then IHDR length+type, then w/h big-endian.
    if len(blob) >= 24 and blob[:8] == b"\x89PNG\r\n\x1a\n":
        width = int.from_bytes(blob[16:20], "big")
        height = int.from_bytes(blob[20:24], "big")

    with conn.cursor() as cur:
        cur.execute(
            f"""INSERT INTO {schema}.portraits
                    (charid, hash, content_type, width, height, bytes)
                VALUES (%s, %s, 'image/png', %s, %s, %s)
                ON DUPLICATE KEY UPDATE
                    hash = VALUES(hash), width = VALUES(width),
                    height = VALUES(height), bytes = VALUES(bytes)""",
            (char_id, appearance_hash, width, height, blob))

    return len(blob), width, height


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
    ap.add_argument("--herald", help="Herald base URL; implied by --kubectl")
    ap.add_argument("--kubectl", action="store_true",
                    help="read the password from the Herald secret and manage "
                         "port-forwards automatically")
    ap.add_argument("--kube-context", default=os.environ.get("XI_KUBE_CONTEXT", "pulse-clift"))
    ap.add_argument("--kube-namespace", default=os.environ.get("XI_KUBE_NAMESPACE", "homelab"))
    ap.add_argument("--kube-secret", default="xiherald-db")
    ap.add_argument("--kube-secret-key", default="password")
    ap.add_argument("--kube-db-service", default="svc/mariadb-service")
    ap.add_argument("--kube-herald", default="deploy/xiherald")
    ap.add_argument("--vellichor", required=True, help="path to the Vellichor project")
    ap.add_argument("--db-host", default=os.environ.get("XI_PORTRAIT_DB_HOST", "127.0.0.1"))
    ap.add_argument("--db-port", type=int, default=int(os.environ.get("XI_PORTRAIT_DB_PORT", "3306")))
    ap.add_argument("--db-user", default=os.environ.get("XI_PORTRAIT_DB_USER", "xiherald"))
    ap.add_argument("--db-pass", default=os.environ.get("XI_PORTRAIT_DB_PASS", ""))
    ap.add_argument("--db-schema", default=os.environ.get("XI_PORTRAIT_DB_NAME", "xiportraits"))
    ap.add_argument("--keep", help="also keep the PNGs in this directory")
    ap.add_argument("--godot", default=os.environ.get("GODOT", "godot"))
    ap.add_argument("--driver", default="opengl3",
                    help="Godot rendering driver. opengl3 is not a preference: "
                         "on arm64 the Vulkan path through llvmpipe aborts with "
                         "an LLVM 'Cannot select AArch64ISD::VLSHR' shader "
                         "compile failure, and the cluster has no GPU.")
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
    ap.add_argument("--force", action="store_true",
                    help="re-render everything regardless of stored hashes")
    ap.add_argument("--renderer-version",
                    default=os.environ.get("XI_RENDERER_VERSION", "dev"),
                    help="baked into the image; changing it re-renders everyone")
    ap.add_argument("--limit", type=int,
                    help="stop after this many render attempts, successful or not")
    args = ap.parse_args()

    if not args.db_schema.replace("_", "").isalnum():
        sys.exit(f"refusing to use {args.db_schema!r} as a schema name")

    if not args.kubectl and not args.herald:
        sys.exit("--herald is required unless --kubectl is given")

    if args.kubectl:
        return run_tunnelled(args)

    return run(args, connect(args))


def run_tunnelled(args):
    """Open both tunnels, fill in the connection details, then run normally."""
    args.db_pass = args.db_pass or secret_value(args)

    with port_forward(args, args.kube_db_service, 3306) as db_port, \
         port_forward(args, args.kube_herald, 8080) as herald_port:
        args.db_host = "127.0.0.1"
        args.db_port = db_port
        args.herald = args.herald or f"http://127.0.0.1:{herald_port}"
        print(f"tunnels up: db :{db_port}, herald :{herald_port}")
        return run(args, connect(args))


def run(args, conn):
    appearances = fetch_appearances(args.herald)
    stored = {} if args.force else stored_hashes(conn, args.db_schema)

    version = args.renderer_version
    todo = sum(1 for a in appearances
               if a["renderable"]
               and stored.get(str(a["character_id"])) != stored_key(a["hash"], version))
    print(f"renderer {version}")
    print(f"{len(appearances)} characters, {len(stored)} already stored, "
          f"{todo} to render")

    if args.keep:
        os.makedirs(args.keep, exist_ok=True)
    work_dir = args.keep or tempfile.mkdtemp(prefix="xi-portraits-")

    rendered = skipped = failed = unrenderable = 0

    for a in appearances:
        if args.only and a["name"].lower() != args.only.lower():
            continue

        key = str(a["character_id"])
        dest = os.path.join(work_dir, key + ".png")

        if not a["renderable"]:
            print(f"  skip   {a['name']}: no appearance data")
            unrenderable += 1
            continue

        if stored.get(key) == stored_key(a["hash"], version):
            skipped += 1
            continue

        if args.limit is not None and (rendered + failed) >= args.limit:
            break

        style = " (style-locked)" if a["style_locked"] else ""
        # Numbered, so a stalled run is obvious from the log alone.
        print(f"  [{rendered + failed + 1}/{todo}] render {a['name']} "
              f"[{a['race_name']}]{style}")

        ok, err = render_one(args, a, dest)
        if not ok:
            print(f"  FAIL   {a['name']}: {err}", file=sys.stderr)
            failed += 1
            continue

        postprocess(dest, args)

        try:
            size, w, h = store_portrait(conn, args.db_schema,
                                        a["character_id"],
                                        stored_key(a["hash"], version), dest)
        except pymysql.Error as err:
            print(f"  FAIL   {a['name']}: store failed: {err}", file=sys.stderr)
            failed += 1
            continue

        # Stored one at a time, so an interrupted run keeps what it finished.
        print(f"         stored {w}x{h}, {size // 1024} KiB")
        rendered += 1

        if not args.keep:
            os.unlink(dest)

    conn.close()

    print(f"\nrendered {rendered}, unchanged {skipped}, "
          f"no data {unrenderable}, failed {failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
