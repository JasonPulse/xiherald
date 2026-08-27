#!/usr/bin/env python3
"""Extract the minimal DAT subset needed to render character portraits.

The full retail corpus is about 10 GiB, which is too much to ship to a cluster.
A character portrait only ever touches skeletons, faces and worn-equipment
models, and that subset is 454 MiB: 4.3% of the archive.

The subset is complete rather than demand-driven. It covers every face, every
skeleton and every equipment model id in every PC race and slot table, for all
eight races, so it never needs re-extracting when somebody equips something new.
A demand-driven subset would be smaller and would break the first time a player
bought a hat.

Paths come from Vellichor's own lookup tables, so this cannot drift from what
the renderer actually resolves.

    ./extract_corpus.py --vellichor ~/Code/Godot/Vellichor --out ./portrait-corpus

The output is game data. It is not redistributable: keep it out of version
control and out of any public registry.
"""

import argparse
import json
import os
import shutil
import sys

# ModelResolver.Skeleton. Race 6 shares race 5's skeleton.
SKELETONS = {
    1: "ROM/27/82.dat", 2: "ROM/32/58.dat", 3: "ROM/37/31.dat", 4: "ROM/42/4.dat",
    5: "ROM/46/93.dat", 6: "ROM/46/93.dat", 7: "ROM/51/89.dat", 8: "ROM/56/59.dat",
}

# ModelResolver slot constants: head, body, hands, legs, feet, main, sub, ranged.
SLOTS = (2, 3, 4, 5, 6, 7, 8, 9)
RACES = range(1, 9)

# Vellichor needs these to index the archive at all.
ARCHIVE_INDEX = ("VTABLE.DAT", "FTABLE.DAT", "ROM/VTABLE.DAT", "ROM/FTABLE.DAT")


def dest_name(rel):
    """The name Vellichor will look for.

    ModelResolver.Abs uppercases a .dat extension to .DAT before touching the
    filesystem, while the JSON tables spell it lowercase. On a case-insensitive
    filesystem the difference is invisible; on Linux, which is where this subset
    is actually used, writing the lowercase name means every lookup misses and
    the renderer silently draws nothing.
    """
    return rel[:-4] + ".DAT" if rel.endswith(".dat") else rel


def find_source(corpus, rel):
    """Locate a file whose extension case may differ from the table's."""
    for candidate in (rel, dest_name(rel), rel[:-4] + ".dat" if rel.endswith(".DAT") else rel):
        path = os.path.join(corpus, candidate)
        if os.path.isfile(path):
            return path
    return None


def load_tables(data_dir):
    with open(os.path.join(data_dir, "model-dat-paths.json"), encoding="utf-8") as fh:
        models = json.load(fh)
    with open(os.path.join(data_dir, "face-paths.json"), encoding="utf-8") as fh:
        faces = json.load(fh)
    return models, faces


def required_paths(models, faces):
    needed = set(SKELETONS.values())

    for race in RACES:
        for face in faces.get(str(race), []):
            needed.add(face["path"])
        for slot in SLOTS:
            needed.update(models.get(f"{race}:{slot}", {}).values())

    return needed


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--vellichor", required=True, help="path to the Vellichor project")
    ap.add_argument("--corpus", help="DAT root; defaults to <vellichor>/corpus")
    ap.add_argument("--out", required=True, help="directory to write the subset into")
    ap.add_argument("--dry-run", action="store_true", help="report the size and stop")
    args = ap.parse_args()

    data_dir = os.path.join(args.vellichor, "data", "models")
    corpus = args.corpus or os.path.join(args.vellichor, "corpus")

    if not os.path.isdir(data_dir):
        sys.exit(f"no model tables at {data_dir}")
    if not os.path.isdir(corpus):
        sys.exit(f"no DAT corpus at {corpus}; pass --corpus")

    models, faces = load_tables(data_dir)
    needed = required_paths(models, faces)
    needed.update(ARCHIVE_INDEX)

    present, missing, total = [], [], 0
    for rel in sorted(needed):
        src = find_source(corpus, rel)
        if src:
            present.append((src, dest_name(rel)))
            total += os.path.getsize(src)
        else:
            missing.append(rel)

    print(f"{len(present):,} files, {total / 1024 / 1024:,.0f} MiB")
    if missing:
        # Reported rather than fatal: a handful of ids in the tables have no
        # file in every install, and a portrait missing one hat is still a
        # portrait.
        print(f"{len(missing):,} referenced files absent from this install",
              file=sys.stderr)

    if args.dry_run:
        return 0

    copied = 0
    for src, rel in present:
        dst = os.path.join(args.out, rel)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        # Skipping identical files makes a re-run cheap and interruptible.
        if os.path.exists(dst) and os.path.getsize(dst) == os.path.getsize(src):
            continue
        shutil.copy2(src, dst)
        copied += 1

    print(f"copied {copied:,} files into {args.out}")
    print("This is game data. Do not commit it or push it to a public registry.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
