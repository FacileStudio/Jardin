# Mycelium — Deployment

How the server is built, deployed to la ruche, and routed, plus how CLI releases are cut.

## The image

`Dockerfile` is three stages and produces one small image:

1. `oven/bun:1` installs `apps/client` dependencies from the lockfile and runs
   `bun run build`, producing the static SvelteKit output.
2. `golang:1.26-alpine` builds the Go binary with `CGO_ENABLED=0` and
   `-trimpath -ldflags="-s -w"`.
3. `gcr.io/distroless/static-debian12` receives the binary at `/mycelium` and the built client
   at `/client`, sets `CLIENT_DIR=/client`, exposes `8420`, and defaults to `serve`.

Two decisions in there are load-bearing:

- **Not the `:nonroot` variant.** The data directory is a named volume, which Docker creates
  owned by root, so a non-root process could not write to it.
- **`CLIENT_DIR` is set explicitly.** A relative `./client` would resolve against the
  image's working directory and the SPA would silently not be served — the API answers, the
  healthcheck stays green, and only a browser sees the failure.

## Compose topology

`docker-compose.yml` declares exactly one service. One binary, one container, one hostname.

```
Traefik ──▶ mycelium-svc :8420 ──▶ /mycelium serve ──▶ mycelium-data volume at /data
```

- `mycelium-web` on the `web` entrypoint redirects to HTTPS through `redirect-to-https@file`.
- `mycelium-secure` on `websecure` terminates TLS with the `letsencrypt` cert resolver.
- Both routers are `Host(mycelium.facile.studio)` and point at the same `mycelium-svc` service
  on port `8420`. **No `PathPrefix`, no `stripprefix`** — the Go binary owns the whole
  hostname and routes `/api/*` itself, with the SPA as the catch-all.
- The container joins the external `dokploy-network` so Traefik can reach it, and
  `traefik.docker.network` names it explicitly. It also joins the external `facile-ai`
  network, which is how it reaches the inference sidecars — see
  [Semantic search](#semantic-search) below.
- Persistent state is the `mycelium-data` named volume mounted at `/data`, with `DATA_DIR`
  set to match. Losing that volume loses the whole brain.

## Healthchecks

The compose healthcheck runs `/mycelium healthcheck`, which `main.go` intercepts through
`tronc/healthcheck` before cobra ever sees the arguments — so the same binary is both the
server and its own probe, with no `curl` in a distroless image.

Over HTTP, `tronc/health` mounts liveness and readiness at both the root and under `/api`.
The one readiness check Mycelium has is that `DATA_DIR` exists and is writable: a named volume
owned by root under a non-root process fails there rather than at the first write.

## Semantic search

Memory search has two halves. The lexical half (BM25) is always on. The semantic half
embeds every memory chunk with a model served by **ollama** and ranks by cosine similarity.
It is dormant until `OLLAMA_URL` is set, and it is the only part of Mycelium that needs
anything outside its own container.

### Turning it on

1. On the docker host, once:

   ```sh
   sh scripts/ai-sidecars.sh
   ```

   That creates the `facile-ai` network, starts `ollama` and `qdrant` if they are missing,
   pulls `bge-m3` (~1.2 GB), and verifies both answer on the network. It is idempotent.

2. On the Dokploy compose service, set the environment and redeploy:

   ```
   OLLAMA_URL=http://ollama:11434
   EMBED_MODEL=bge-m3
   VECTOR_STORE=flat
   ```

   A Dokploy variable reaches nothing unless the compose file forwards it. All four are
   already in the `environment:` block, so the panel is enough — but the change lands on the
   **next** deploy, and the panel is not proof. Confirm with `docker inspect <container>
   --format '{{range .Config.Env}}{{println .}}{{end}}'`.

`VECTOR_STORE=qdrant` additionally needs `QDRANT_URL=http://qdrant:6333`. `flat` is the
default and the right answer at this size: it keeps the index inside the `mycelium-data`
volume, so there is one thing to back up instead of two.

### What happens when it is off

Nothing breaks. No embedding worker starts and memory search returns lexical results only.
The same path covers a model that is up but unreachable: failures are logged and requeued,
search stays lexical, and the queue drains once ollama answers again. Semantic search is
never a reason for the server to refuse to boot or for a request to fail.

### Why ollama is not a service in this file

`docker-compose.yml` declares one service, as it always has. `ollama` and `qdrant` are
standalone containers on the host, and the compose file's only link to them is the external
`facile-ai` network. Three reasons:

- **The model volume must outlive a deploy.** Dokploy prefixes compose volumes with the
  stack's appName, so an `ollama:` service here would get a fresh, empty volume on first
  deploy — 1.2 GB re-downloaded and a full reindex.
- **A redeploy would bounce the model server.** `docker compose up` recreates what it owns,
  and restarting shared inference to ship a frontend fix is the wrong coupling.
- **A hard dependency would be a lie.** `depends_on: service_healthy` turns any healthcheck
  flake into a failed deploy, and the feature is designed to degrade instead.

The cost is a dependency living outside the repo. `scripts/ai-sidecars.sh` answers that: the
whole host-side setup, in version control, idempotent, with `--verify` and a `--recreate`
that changes flags without touching the volumes.

### Sidecar healthchecks

Neither image ships `curl` or `wget`, so both probes use something the image is actually
built around — a permanently-red probe is worse than no probe. `ollama` runs
`ollama list | grep -q '^bge-m3'`, which round-trips its own HTTP API **and** checks the model
is there: plain `ollama list` exits 0 against an empty model store, which is precisely the
state that makes every search silently fall back to lexical. `qdrant` runs `GET /healthz`
through bash's `/dev/tcp` and greps the status line, proving HTTP rather than just an open
socket. Both use the suite's `10s / 5s / 3`, with a 30s start period for ollama.

`ollama` runs with `OLLAMA_KEEP_ALIVE=-1`, which pins the model in RAM instead of unloading
it after five idle minutes — without it the first search after a quiet spell pays a ~2.5s
model load. Both have memory limits, 8 GB and 4 GB, against a ~1.2 GB resident model.

One failure mode no healthcheck catches: squeeze ollama's memory cap and the kernel kills the
`llama-server` **child** while the parent keeps running. The container stays green, no restart
policy fires, and `/api/embed` returns 500s. Budget generously — the limits above are already
several times the model — and treat a rise in embedding errors, not container health, as the
signal.

### The first index

Enabling the feature on a populated tree embeds everything once. On la ruche that is about
**40 minutes for 1516 chunks** with `bge-m3` on CPU — the box has no GPU and measures around
half a chunk per second. It runs in the background, debounced and coalesced by path; the
server serves normally throughout and searches stay lexical until the vectors land. Progress,
rate and ETA are on `GET /api/memory/index/status`.

After that only changed pages are re-embedded — chunks are keyed by content hash — so a
normal sync costs a handful of embeddings. The index survives redeploys: it lives in the
`mycelium-data` volume (`flat`) or in qdrant's own.

## Deploying

Mycelium autodeploys from `main` through Dokploy on la ruche, panel at
[gare.facile.studio](https://gare.facile.studio). Prefer the `dokploy` CLI over SSH and
docker.

Configuration is set as environment variables on the Dokploy compose service, not committed.
`cp .env.example .env` covers a local run:

```sh
cp .env.example .env
docker compose up -d
docker compose logs -f mycelium
```

Set `PASSWORD` before starting with `APP_ENV=production`, or the server exits 1 — see
[configuration.md](configuration.md) for the three refusals.

## Migrations

There are none. Mycelium has no database; state is markdown and JSON files under `DATA_DIR`.
Two in-place upgrades happen automatically on read:

- `tokens.json` entries written before scopes existed are re-keyed by SHA-256 hash and
  assigned a scope (`admin` for the old `session` entry, `sync` for machines), then saved
  back once.
- The tree that predates spaces stays exactly where it is and becomes the **common** scope.
  Nothing is moved.

Backing up means backing up the volume.

## Verifying a deploy

A green `/api/health` says the Go process is up. It says nothing about the SPA, which is
served by the catch-all and can be missing while every probe stays green. Check both:

```sh
curl -fsS https://mycelium.facile.studio/api/health
curl -fsSI https://mycelium.facile.studio/ | head -1
```

An unknown API path must answer a 404 error envelope rather than 200 plus HTML:

```sh
curl -s https://mycelium.facile.studio/api/nope
```

With semantic search on, check the sidecar path from the host — no deploy needed — and
confirm nothing became publicly reachable:

```sh
sh scripts/ai-sidecars.sh --verify
ss -ltnp | grep -E '11434|6333'   # must be 127.0.0.1 on both, never 0.0.0.0
```

## CLI releases

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs GoReleaser. It
builds `linux` and `darwin` for `amd64` and `arm64`, stamps the version into
`github.com/FacileStudio/Mycelium/cmd.version`, publishes tar.gz archives with a checksum
file, and pushes an updated formula to the `FacileStudio/homebrew-tap` tap.

```sh
git tag v0.9.0
git push --tags
```

The workflow needs `HOMEBREW_TAP_GITHUB_TOKEN` in repository secrets for the tap commit.

Back to the [documentation index](README.md).
