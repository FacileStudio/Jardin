FROM oven/bun:1 AS client-build
WORKDIR /client
COPY apps/client/package.json apps/client/bun.lock* ./
RUN bun install --frozen-lockfile
COPY apps/client/ .
RUN bun run build

FROM golang:1.26-alpine AS api-build

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /repo

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o bin/jardin .

# Not the :nonroot variant: the data directory is a named volume, which Docker
# creates owned by root, so a non-root process could not write to it.
FROM gcr.io/distroless/static-debian12

COPY --from=api-build /repo/bin/jardin /jardin
COPY --from=client-build /client/build /client

# Obligatory: a relative ./client would resolve against the image's working
# directory, and the SPA would silently not be served — the API answers, the
# healthcheck is green, and only a browser sees the failure.
ENV CLIENT_DIR=/client

EXPOSE 8420

ENTRYPOINT ["/jardin"]
CMD ["serve"]
