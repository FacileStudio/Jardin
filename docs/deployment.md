# Jardin — Deployment

How the server is built, deployed to la ruche, and routed, plus how CLI releases are cut.

## The image

`Dockerfile` is three stages and produces one small image:

1. `oven/bun:1` installs `apps/client` dependencies from the lockfile and runs
   `bun run build`, producing the static SvelteKit output.
2. `golang:1.26-alpine` builds the Go binary with `CGO_ENABLED=0` and
   `-trimpath -ldflags="-s -w"`.
3. `gcr.io/distroless/static-debian12` receives the binary at `/jardin` and the built client
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
Traefik ──▶ jardin-svc :8420 ──▶ /jardin serve ──▶ jardin-data volume at /data
```

- `jardin-web` on the `web` entrypoint redirects to HTTPS through `redirect-to-https@file`.
- `jardin-secure` on `websecure` terminates TLS with the `letsencrypt` cert resolver.
- Both routers are `Host(jardin.facile.studio)` and point at the same `jardin-svc` service
  on port `8420`. **No `PathPrefix`, no `stripprefix`** — the Go binary owns the whole
  hostname and routes `/api/*` itself, with the SPA as the catch-all.
- The container joins the external `dokploy-network` so Traefik can reach it, and
  `traefik.docker.network` names it explicitly.
- Persistent state is the `jardin-data` named volume mounted at `/data`, with `DATA_DIR`
  set to match. Losing that volume loses the whole brain.

## Healthchecks

The compose healthcheck runs `/jardin healthcheck`, which `main.go` intercepts through
`tronc/healthcheck` before cobra ever sees the arguments — so the same binary is both the
server and its own probe, with no `curl` in a distroless image.

Over HTTP, `tronc/health` mounts liveness and readiness at both the root and under `/api`.
The one readiness check Jardin has is that `DATA_DIR` exists and is writable: a named volume
owned by root under a non-root process fails there rather than at the first write.

## Deploying

Jardin autodeploys from `main` through Dokploy on la ruche, panel at
[gare.facile.studio](https://gare.facile.studio). Prefer the `dokploy` CLI over SSH and
docker.

Configuration is set as environment variables on the Dokploy compose service, not committed.
`cp .env.example .env` covers a local run:

```sh
cp .env.example .env
docker compose up -d
docker compose logs -f jardin
```

Set `PASSWORD` before starting with `APP_ENV=production`, or the server exits 1 — see
[configuration.md](configuration.md) for the three refusals.

## Migrations

There are none. Jardin has no database; state is markdown and JSON files under `DATA_DIR`.
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
curl -fsS https://jardin.facile.studio/api/health
curl -fsSI https://jardin.facile.studio/ | head -1
```

An unknown API path must answer a 404 error envelope rather than 200 plus HTML:

```sh
curl -s https://jardin.facile.studio/api/nope
```

## CLI releases

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs GoReleaser. It
builds `linux` and `darwin` for `amd64` and `arm64`, stamps the version into
`github.com/FacileStudio/Jardin/cmd.version`, publishes tar.gz archives with a checksum
file, and pushes an updated formula to the `FacileStudio/homebrew-tap` tap.

```sh
git tag v0.9.0
git push --tags
```

The workflow needs `HOMEBREW_TAP_GITHUB_TOKEN` in repository secrets for the tap commit.

Back to the [documentation index](README.md).
