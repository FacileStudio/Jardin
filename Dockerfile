FROM golang:alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o ruche .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /app/ruche /usr/local/bin/ruche
EXPOSE 8420
CMD ["ruche", "serve", "--port", "8420"]
