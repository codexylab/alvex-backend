# syntax=docker/dockerfile:1

FROM golang:1.26.8-alpine3.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/alvex-api \
    ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/alvex-migrate \
    ./cmd/migrate

FROM alpine:3.24

RUN apk --no-cache add ca-certificates tzdata
RUN addgroup -S -g 10001 alvex \
    && adduser -S -D -H -u 10001 -G alvex alvex

WORKDIR /app

COPY --from=builder --chown=alvex:alvex /out/alvex-api /app/alvex-api
COPY --from=builder --chown=alvex:alvex /out/alvex-migrate /app/alvex-migrate

EXPOSE 8080

USER alvex

ENTRYPOINT ["/app/alvex-api"]
