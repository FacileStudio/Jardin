#!/usr/bin/env sh
#
# Provisions the shared inference sidecars on the docker host: the `facile-ai`
# network, the `ollama` container holding the embedding model, and `qdrant`.
#
#   sh scripts/ai-sidecars.sh             create what is missing, pull the model, verify
#   sh scripts/ai-sidecars.sh --recreate  replace the containers, keeping their volumes
#   sh scripts/ai-sidecars.sh --verify    check only, change nothing
#
# Idempotent. Run it once on the host before the first deploy that sets
# OLLAMA_URL: `facile-ai` is declared `external` in docker-compose.yml, so
# compose refuses to start while it does not exist.
#
# These containers deliberately do NOT belong to Mycelium's compose project. A
# Dokploy redeploy recreates every service of the stack it owns, and Dokploy
# prefixes compose volumes with the project's appName — so an `ollama:` service
# here would bounce the model server on each deploy and, the first time, hand it
# an empty volume: 1.2 GB re-downloaded and ~40 min of re-indexing. Owning them
# separately is what keeps the model volume stable.
#
# Nothing below publishes a port beyond 127.0.0.1. The container-to-container
# path is the `facile-ai` network alone.

set -eu

NETWORK=facile-ai

OLLAMA_NAME=ollama
OLLAMA_IMAGE=ollama/ollama:0.32.14
OLLAMA_VOLUME=ollama
OLLAMA_BIND=127.0.0.1:11434
OLLAMA_MEMORY=8g

QDRANT_NAME=qdrant
QDRANT_IMAGE=qdrant/qdrant:v1.19.0
QDRANT_VOLUME=qdrant_storage
QDRANT_BIND=127.0.0.1:6333
QDRANT_MEMORY=4g

MODEL="${EMBED_MODEL:-bge-m3}"

mode="ensure"
case "${1:-}" in
--recreate) mode="recreate" ;;
--verify) mode="verify" ;;
"") ;;
*)
  echo "usage: $0 [--recreate|--verify]" >&2
  exit 2
  ;;
esac

say() { printf '\033[1m==>\033[0m %s\n' "$1"; }

exists() { docker container inspect "$1" >/dev/null 2>&1; }

# join attaches a running container to the network unless it is already there.
# Attaching is hot: it never restarts the container.
join() {
  if docker network inspect "$NETWORK" --format '{{range .Containers}}{{println .Name}}{{end}}' |
    grep -qx "$1"; then
    say "$1 already on $NETWORK"
  else
    docker network connect "$NETWORK" "$1"
    say "$1 joined $NETWORK"
  fi
}

run_ollama() {
  # OLLAMA_KEEP_ALIVE=-1 pins the model in RAM. Without it ollama unloads after
  # five idle minutes and the next search pays a ~2.5 s reload, which for an
  # embedding sidecar is the whole latency budget.
  #
  # OLLAMA_HOST already defaults to 0.0.0.0:11434 in this image, unlike the bare
  # binary. It is pinned anyway so a base-image change cannot quietly move the
  # bind address to loopback and cut the network path.
  #
  # The probe greps for the model rather than only running `ollama list`: a
  # server with an empty model store still exits 0, so the plain form is
  # liveness, not readiness, and would report green on the one state that makes
  # every search fall back to lexical.
  docker run -d \
    --name "$OLLAMA_NAME" \
    --restart unless-stopped \
    --network "$NETWORK" \
    --memory "$OLLAMA_MEMORY" \
    -p "$OLLAMA_BIND:11434" \
    -v "$OLLAMA_VOLUME:/root/.ollama" \
    -e OLLAMA_HOST=0.0.0.0:11434 \
    -e OLLAMA_KEEP_ALIVE=-1 \
    -e OLLAMA_MAX_LOADED_MODELS=2 \
    --health-cmd "ollama list | grep -q '^$MODEL' || exit 1" \
    --health-interval 10s \
    --health-timeout 5s \
    --health-retries 3 \
    --health-start-period 30s \
    "$OLLAMA_IMAGE" >/dev/null
}

run_qdrant() {
  # The image carries neither curl nor wget, so the probe is bash's /dev/tcp —
  # which still speaks HTTP and reads the status line, rather than only proving
  # something accepted a socket.
  docker run -d \
    --name "$QDRANT_NAME" \
    --restart unless-stopped \
    --network "$NETWORK" \
    --memory "$QDRANT_MEMORY" \
    -p "$QDRANT_BIND:6333" \
    -v "$QDRANT_VOLUME:/qdrant/storage" \
    --health-cmd 'bash -c "exec 3<>/dev/tcp/127.0.0.1/6333 && printf \"GET /healthz HTTP/1.0\r\n\r\n\" >&3 && grep -q 200 <&3"' \
    --health-interval 10s \
    --health-timeout 5s \
    --health-retries 3 \
    --health-start-period 10s \
    "$QDRANT_IMAGE" >/dev/null
}

if [ "$mode" = "verify" ]; then
  docker network inspect "$NETWORK" >/dev/null
  # A real embed rather than /api/tags: it proves the model loads and not just
  # that it is listed, and it doubles as the warm-up that pays the cold start
  # here instead of in the first user search.
  docker run --rm --network "$NETWORK" curlimages/curl:latest \
    -fsS --max-time 180 "http://$OLLAMA_NAME:11434/api/embed" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$MODEL\",\"input\":\"sidecar check\"}" | grep -q '"embeddings"'
  say "ollama embeds with $MODEL over the network, and the model is now warm"
  docker run --rm --network "$NETWORK" curlimages/curl:latest \
    -fsS --max-time 10 "http://$QDRANT_NAME:6333/healthz" >/dev/null
  say "qdrant answers on the network"
  exit 0
fi

docker network inspect "$NETWORK" >/dev/null 2>&1 || {
  docker network create "$NETWORK" >/dev/null
  say "created network $NETWORK"
}

if [ "$mode" = "recreate" ]; then
  # The volumes are named and declared outside any compose project, so removing
  # the containers keeps the model and the vector index.
  docker rm -f "$OLLAMA_NAME" "$QDRANT_NAME" >/dev/null 2>&1 || true
  say "removed the old containers, kept $OLLAMA_VOLUME and $QDRANT_VOLUME"
fi

if exists "$OLLAMA_NAME"; then
  join "$OLLAMA_NAME"
else
  run_ollama
  say "started $OLLAMA_NAME from $OLLAMA_IMAGE"
fi

if exists "$QDRANT_NAME"; then
  join "$QDRANT_NAME"
else
  run_qdrant
  say "started $QDRANT_NAME from $QDRANT_IMAGE"
fi

# Pulling a model that is already there is a no-op that costs one API call, so
# this stays in the default path: the model is the slow half of the setup and a
# missing one only shows up much later, as a search that silently stays lexical.
if docker exec "$OLLAMA_NAME" ollama list | grep -q "^$MODEL"; then
  say "$MODEL already pulled"
else
  say "pulling $MODEL — around 1.2 GB, several minutes"
  docker exec "$OLLAMA_NAME" ollama pull "$MODEL"
fi

sh "$0" --verify
