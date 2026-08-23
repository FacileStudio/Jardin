FROM oven/bun:1 AS client-build
WORKDIR /client
COPY apps/client/package.json apps/client/bun.lock* ./
RUN bun install --frozen-lockfile
COPY apps/client/ .
RUN bun run build

FROM golang:1.26-alpine AS api-build

ARG TARGETOS=linux
ARG TARGETARCH

# Not in the base image, and the build needs it to name what it built.
RUN apk add --no-cache git

WORKDIR /repo

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# The version comes out of the checkout rather than a literal someone has to
# remember to bump, which is the copy that drifts on the first release nobody
# thinks about. A build sitting on a tag reports the tag, one past it reports
# the tag plus the commit, and a checkout with no tags at all still reports the
# short sha. Goreleaser stamps this same variable without the leading v, so
# stripping it keeps `mycelium --version` printing a bare semver either way.
#
# Without this the container answers "mycelium dev" and nothing running in
# production can tell you which commit it came from.
RUN VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)" && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath \
    -ldflags="-s -w -X github.com/FacileStudio/Mycelium/cmd.version=${VERSION#v}" \
    -o bin/mycelium .

# Not the :nonroot variant: the data directory is a named volume, which Docker
# creates owned by root, so a non-root process could not write to it.
FROM gcr.io/distroless/static-debian12

COPY --from=api-build /repo/bin/mycelium /mycelium
COPY --from=client-build /client/build /client

# Obligatory: a relative ./client would resolve against the image's working
# directory, and the SPA would silently not be served — the API answers, the
# healthcheck is green, and only a browser sees the failure.
ENV CLIENT_DIR=/client

EXPOSE 8420

ENTRYPOINT ["/mycelium"]
CMD ["serve"]
