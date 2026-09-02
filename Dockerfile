# Multi-stage build for optimal image size and security
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile static Linux binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server ./cmd/server

# Minimal production image
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server /app/server

# Expose default port
EXPOSE 8080

CMD ["/app/server"]
