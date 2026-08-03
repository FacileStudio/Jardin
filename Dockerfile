FROM golang:alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o mycelium .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /app/mycelium /usr/local/bin/mycelium
EXPOSE 8420
CMD ["mycelium", "serve", "--port", "8420"]
