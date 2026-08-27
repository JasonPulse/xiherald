# FFXI Herald

A read-only statistics site for a private LandSandBoat FFXI server, modelled on
the [Blackthorn DAoC herald](https://herald.blackthorn-daoc.com/). Blackthorn's
version is built on realm points, which FFXI has no equivalent of, so this one
trades one big PvP ladder for many small boards drawn from what the game server
already records.

Brass-on-black Norse styling, drawn entirely in CSS. Single Go binary,
everything embedded, multi-arch image published to GHCR.

## What it shows

**Roster.** Every character with nation, race, main and sub job, master level,
jobs at 99, total job levels, kills, deaths, K/D, playtime and last seen. Nine
sort orders. Online characters are marked live from `accounts_sessions`.

**Character detail.** Identity and title, gil, merits, limit points, exemplar
points. All 22 job levels with job points. Combat, defensive and magic skills
against their level-and-rank cap from `skill_caps`. Craft skills with synthesis
rank and guild points. The full `char_history` record. Nation ranks and fame
across all 15 fame areas. Mission standing per expansion, decoded from the
`chars.missions` blob.

**Fun stats.** Twenty-one leaderboards in three groups. Combat covers kills,
deaths, K/D, knockouts, battles, weapon skills, spells and abilities. Activity
covers playtime, distance travelled, NPCs talked to, chat lines, items used,
parties joined and Mog House visits. Progression covers total job levels, jobs
at 99, master level, merits, rank points and job points.

Adding a board is one entry in `Metrics` in `internal/xidb/leaders.go`. The
query, the page, the stats index and the sidebar all read from that registry.

## Character portraits

Rendered character models with worn gear, in the spirit of the FFXIV Lodestone.

The Herald does not render them. Rendering needs the retail DAT archives, which
are not redistributable and run to about 12 GB, so it happens wherever those
archives already are. **Vellichor** does the work; it already had every piece:
`ModelResolver.PcRecipe(EntityLook)` assembles a PC from a look vector whose
fields are exactly `char_look`'s, and `VELLICHOR_SHOT` writes the viewport to a
PNG. About 1.5 seconds per character.

Storage is a separate database, `xiportraits`, holding one row per character.
Nothing lands on disk, so there is no file to back up and no volume to provision.

**The renderer writes to that database directly, not through the Herald.** The
Herald therefore has no network write path at all: no upload endpoint, no shared
secret, nothing to secure if the site is ever put behind a tunnel. Its rights
are asymmetric on purpose, full control of `xiportraits` and `SELECT` only on
`xidb`, and the test suite asserts both halves.

```bash
pip install -r tools/portraits/requirements.txt

XI_PORTRAIT_DB_PASS=... ./tools/portraits/render_portraits.py \
    --herald http://localhost:8080 \
    --vellichor ~/Code/Godot/Vellichor \
    --godot /Applications/Godot_mono.app/Contents/MacOS/Godot
```

The `portraits` table is created by whichever of the Herald or the renderer runs
first; both issue the same `CREATE TABLE IF NOT EXISTS`, so there is no
migration step and no ordering requirement.

Set `XI_HERALD_PORTRAIT_SCHEMA=""` to switch portraits off. A schema that is
missing or unreachable is a warning, not a failed start: every page renders
without portraits.

### Caching

Portrait URLs carry the hash of the **stored** image, not of the current
appearance. That distinction matters. Using the appearance hash would mint a new
URL the moment somebody changed gear, before anything had re-rendered, and the
browser would cache the stale image under that new URL forever because the
response is `immutable`.

A character with no portrait emits no `img` tag at all, so an unrendered roster
costs no requests rather than one 404 per page view.

### The DAT archives

They never enter any repository. Vellichor reads them from a configured path
(`InstallLocator`, which also autodetects a retail install), and its
`.gitignore` excludes `corpus/`, `*.dat`, `ROM[0-9]/`, `VTABLE*` and `FTABLE*`.
Only the finished PNGs travel, roughly 120 KB each.

### Appearance resolution

`GET /api/v1/appearances` is the render work-list. It resolves the appearance
rather than dumping the tables, because two sources are involved and they are
not interchangeable:

- `char_look` holds **model** ids per visible slot. The normal case.
- `char_style` holds **item** ids, and applies when `chars.isstylelocked` is
  set. The server compares those values using `HasItem`
  (`charutils.cpp:1961`), which is what gives them away as item ids.

So a style-locked character's slots are mapped through `item_equipment.MId` to
get model ids. Rendering `char_style`'s raw values as models would silently
dress everybody in the wrong gear, and it would look plausible.

Each entry carries a `hash` of the resolved appearance. The renderer keeps the
hash it last drew and skips unchanged characters, which is the difference
between re-rendering a hundred people every run and re-rendering the two who
changed gear.

### Background

Renders use `VELLICHOR_MAGENTA` for a flat background that is keyed to
transparency, so the backdrop comes from CSS. Restyling it costs nothing;
baking a background into the PNGs would mean re-rendering everyone.

## Mission decoding

`chars.missions` is a raw dump of `missionlog_t[15]` from the server's
`common/mmo.h`: `uint16 current`, `statusUpper`, `statusLower`, then
`bool complete[64]`. Seventy bytes per log, fifteen logs, 1050 bytes,
little-endian.

Two rules in there are not inferable from the data and were taken from the
server's own accessors. Both change the answer.

**"No current mission" is spelled differently per log.** `setMissionStatus`
clears it with `logId > 2 ? 0 : uint16 max`, so the three nation logs use 65535
while every expansion log uses 0. Reading 0 as "none" everywhere hides San
d'Oria's first mission; reading it as a mission everywhere invents one for every
fresh character.

**Completion is a hybrid.** `hasCompletedMission` uses
`(log == CoP || id >= 64) ? id < current : complete[id]`, because the complete
array has only 64 slots while SoA has 105 missions and RoV has 94. Those logs
fall back to treating `current` as a high-water mark.

Mission ids are sparse and per-log, so completion is counted over the ids that
actually exist in the generated name table. That is what makes "41 of 48"
meaningful rather than a raw bit count.

One honest limitation: the number shown is the server's internal mission id. For
the numbered expansions (ToAU, WotG, SoA, RoV) that matches how players refer to
them, so ToAU 41 really is ToAU 41. Nation and CoP missions use chapter notation
at retail, and the server data holds no mapping to it, so CoP shows its internal
id with the mission name beside it.

`tools/gen_missions.py` regenerates the name table from
`scripts/globals/missions.lua`: 527 mission names across 15 logs. Assault and
Campaign have no mission table there, so they report no names.

```bash
./tools/gen_missions.py ../xiserver/scripts/globals/missions.lua
```

## JSON API

For other tools in the cluster. Read-only, no auth, same trust boundary as the
pages.

```
GET /api/v1                        endpoint index and caveats
GET /api/v1/summary                server-wide counters
GET /api/v1/characters             every character, ?sort= as the roster
GET /api/v1/characters/{name}      one character in full
GET /api/v1/leaderboards           the metric registry
GET /api/v1/leaderboards/{metric}  one board, ranked
```

The response types are defined separately from the internal query structs on
purpose. They are a published contract: renaming a Go field or reordering a
query must not silently break a consumer. Arrays are always arrays and never
`null`, so a caller iterating results needs no empty-server special case.

**Gil is not exposed by this API.** It is on the HTML character page, which is
private. The known consumer is a public-facing guild site, and gil is the one
number here that becomes a griefing target the moment it leaves a private
network. If something genuinely needs it, that should be a deliberate change
rather than a field discovered by accident.

Consume it server-side, from inside the cluster:

```
http://xiherald.homelab.svc.cluster.local/api/v1/characters
```

No CORS headers are sent, because a public visitor's browser cannot reach the
Herald and a public site fetching this client-side would not work anyway. If you
ever do want browser-side access from another origin, that needs a deliberate
CORS decision, not a header added quietly.

Version the path, not the fields. `v1` field meanings are stable; a breaking
change gets `v2`.

## Running it

The Herald connects straight to the game database as a read-only user. No game
server changes, and it keeps working while the map server is down.

```bash
export XI_HERALD_DB_HOST=mariadb
export XI_HERALD_DB_PASS=...
docker run --rm -p 8080:8080 \
  -e XI_HERALD_DB_HOST -e XI_HERALD_DB_PASS \
  ghcr.io/jasonpulse/xiherald:latest
```

| Variable | Default |
| --- | --- |
| `XI_HERALD_ADDR` | `:8080` |
| `XI_HERALD_SERVER_NAME` | `Vana'diel` |
| `XI_HERALD_CACHE_TTL` | `30s` |
| `XI_HERALD_DB_HOST` | `127.0.0.1` |
| `XI_HERALD_DB_PORT` | `3306` |
| `XI_HERALD_DB_USER` | `xiherald` |
| `XI_HERALD_DB_PASS` | empty |
| `XI_HERALD_DB_NAME` | `xidb` |

`CACHE_TTL` is what stops a page refresh reaching the game database. Set it to
`0s` to disable.

### Database access

`deploy/sql/grant.sql` creates the user with `SELECT` on the fourteen tables
the Herald reads and nothing else. `char_inventory` gets a column grant, so gil
is readable and the rest of everyone's inventory is not.

The connection also pins a five second `innodb_lock_wait_timeout` and a small
pool. A leaderboard query must never be the reason a player's zone-in stalls.

### Deployment

The Herald is stateless: no volumes, no local files, no database of its own. It
reads the existing game database and stores nothing.

`deploy/HANDOFF.md` is the full deployment contract: the env var table, the
required grants, the probe semantics and what the Herald deliberately does not
decide. `deploy/k8s/herald.yaml` is a reference manifest with placeholders, not
something to apply as-is. Namespace, service names, secret mechanism and ingress
are infrastructure decisions and live outside this repo.

Two probe endpoints, and the split is not interchangeable:

- `/livez` says the process is serving and never touches the database. Liveness.
- `/healthz` pings the database. Readiness.

Liveness deliberately does not depend on the database. The Herald tolerates the
game database being down; an outage should take it out of the load balancer, not
restart it.

## Building

The image is published by `.github/workflows/docker.yml` for `linux/amd64` and
`linux/arm64` on every push to `master`. It cross-compiles from the build
platform rather than emulating the target, so arm64 costs the same as amd64.

Locally, no Go install is needed:

```bash
./tools/go.sh build ./...     # Go toolchain in a container
./tools/verify.sh             # the real test, see below
```

## Verifying

`tools/verify.sh` is the whole proof. It runs gofmt and vet, stands up MariaDB,
loads the **actual** `CREATE TABLE` statements from the xiserver repo rather
than a hand-copied schema, seeds fixture characters, builds the image, and
asserts on the rendered HTML of every page.

```bash
./tools/verify.sh                      # 43 assertions
KEEP=1 ./tools/verify.sh               # leave it on http://localhost:8099
ARCH=linux/arm64 ./tools/verify.sh     # run the suite against the arm64 image
XISERVER=../xiserver ./tools/verify.sh # if the server repo is elsewhere
```

Loading the real schema means an upstream column rename shows up here as a
failure instead of as a broken page in production.

`tools/shots.sh` screenshots the running site with headless Chromium, desktop
and mobile.

## Design

Every frame, corner bracket, rule and bar on the site is drawn with CSS
gradients or with `internal/web/static/img/fleuron.svg`, which is authored here.
There are no raster frame assets, so borders stay crisp at any zoom and the
whole visual system is about 60 lines of CSS.

The frame recipe is one hairline box, a second inset hairline, and four corner
brackets built from eight one-pixel gradient layers. `--bracket` sets the arm
length, which is the only thing that differs between a stat tile, a panel and a
header panel. Header panels add the fleuron below their top edge.

The palette is brass over near-black with a crystal-blue accent, taken from the
logo's own colours. Type is Vollkorn.

**Vollkorn defaults to old-style figures.** It renders `1` as a small-cap `I`
and drops half the digits below the baseline, which is fine in prose and useless
in a table of numbers. The stylesheet forces lining figures throughout, and
tabular figures in number columns.

`tools/build_assets.sh` generates the only two things that need generating: the
logo derivatives, and a seeded noise tile that stops the panels reading as flat
black.

```bash
./tools/build_assets.sh
```

`tools/gen_titles.py` regenerates `internal/xidb/titles_gen.go` from the
server's `scripts/enum/title.lua`. 1100 title ids, so it is generated rather
than maintained by hand.

```bash
./tools/gen_titles.py ../xiserver/scripts/enum/title.lua
```

## Licensing and provenance

Nothing third-party and non-redistributable ships in this repository.

- **Vollkorn** is under the SIL Open Font License. The licence text travels with
  it at `internal/web/static/fonts/Vollkorn-OFL.txt`.
- **The logo** is the project owner's.
- **`fleuron.svg`, the CSS frame system and the noise tile** are authored here.

An earlier revision of this design traced its panel frames out of a paid UI kit.
Those assets were replaced with the CSS system above precisely so this
repository could be public. Do not reintroduce them.
