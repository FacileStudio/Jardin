FROM golang:alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o jardin .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /app/jardin /usr/local/bin/jardin
EXPOSE 8420
CMD ["jardin", "serve", "--port", "8420"]
