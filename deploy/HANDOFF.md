# Deployment contract

This is what the Herald needs in order to run. It is written for whoever owns
the infrastructure; nothing in this directory is applied by the Herald repo or
by anyone working in it.

The two files alongside this one are references, not production manifests.
`k8s/herald.yaml` uses placeholder values for anything cluster-specific and is
there to show the shape, not to be applied as-is.

## The image

`ghcr.io/jasonpulse/xiherald:latest`, published for `linux/amd64` and
`linux/arm64` on every push to `master`.

Roughly 10 MiB, distroless static base, runs as `nonroot`. Verified working on
arm64: `ARCH=linux/arm64 ./tools/verify.sh` runs the full 43-assertion suite
against the arm64 image.

## Statefulness

None. The Herald stores nothing.

- No volumes, no PVCs, no emptyDir, no hostPath
- No filesystem writes at all, so `readOnlyRootFilesystem: true` is safe
- No schema, no migrations, no local database
- Nothing to back up, and nothing lost when the pod is replaced

Every page is a live `SELECT` against the game database. Replica count can be
anything, including zero, without consequence.

## Configuration

All via environment variables. Everything except the password has a working
default.

| Variable | Default | Notes |
| --- | --- | --- |
| `XI_HERALD_ADDR` | `:8080` | listen address |
| `XI_HERALD_SERVER_NAME` | `Vana'diel` | shown in the page header |
| `XI_HERALD_CACHE_TTL` | `30s` | see below |
| `XI_HERALD_DB_HOST` | `127.0.0.1` | the game MariaDB service |
| `XI_HERALD_DB_PORT` | `3306` | |
| `XI_HERALD_DB_USER` | `xiherald` | |
| `XI_HERALD_DB_PASS` | empty | must come from a secret |
| `XI_HERALD_DB_NAME` | `xidb` | the game database |

`XI_HERALD_DB_PASS` is the only value that has to be supplied. It must match the
password used in the `CREATE USER` statement in `sql/grant.sql`.

`CACHE_TTL` is what stops a page refresh reaching the game database. It holds
rendered query results in memory for that window. `0s` disables it.

## Database access

The Herald needs a read-only MySQL user on the **existing** game database. It
does not want a database of its own.

`sql/grant.sql` is the exact grant set: `SELECT` on the fourteen tables it reads
and nothing else, plus a four-column grant on `char_inventory` so that gil is
readable while the rest of every character's inventory is not.

The connection pins `innodb_lock_wait_timeout=5`, `READ-COMMITTED` isolation and
a four-connection pool. A leaderboard query must never be why a player's zone-in
stalls.

## Probes

The split matters and is not interchangeable.

- `GET /livez` returns 200 whenever the process is serving. It never touches the
  database. **Use this for liveness.**
- `GET /healthz` pings the database and returns 503 if it cannot answer. **Use
  this for readiness.**

The Herald starts successfully with the database unreachable and recovers on its
own when it returns, so a database outage should pull it out of the load balancer
rather than restart it. Pointing liveness at `/healthz` would defeat that.

## Networking

HTTP only, no TLS termination, no auth. All URLs it emits are relative, so it
works unchanged behind a path prefix, a tunnel or a reverse proxy.

There is currently **no authentication of any kind**, and the character page
shows gil. That is fine on a private network and is a decision to revisit before
anything puts it on the public internet. Note that this source repository being
public says nothing about the Herald itself being reachable; they are separate
decisions.

## Values this repo deliberately does not assert

Namespace, MariaDB service name, secret name and mechanism, ingress, resource
limits and image tag policy are all infrastructure decisions. The placeholders in
`k8s/herald.yaml` are marked `CHANGE_ME` or left obviously generic; treat every
one of them as needing a real value rather than a default worth keeping.
