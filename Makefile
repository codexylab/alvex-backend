# ============================================================
# ALVEX Backend — Makefile
# ============================================================

.PHONY: run build seed migrate-up migrate-down test lint clean

# --- Development ---
run:
	go run ./cmd/server/

# --- Build ---
build:
	go build -ldflags="-s -w" -o bin/alvex.exe ./cmd/server/

# --- Database ---
seed:
	go run ./cmd/seed/

migrate-up:
	go run -mod=mod github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path internal/database/migrations \
		-database "$(DATABASE_URL)" up

migrate-down:
	go run -mod=mod github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path internal/database/migrations \
		-database "$(DATABASE_URL)" down 1

# --- Testing ---
test:
	go test ./... -v -cover

test-short:
	go test ./... -short

# --- Code quality ---
lint:
	golangci-lint run ./...

vet:
	go vet ./...

# --- Cleanup ---
clean:
	rm -rf bin/

# --- Dependencies ---
tidy:
	go mod tidy
	go mod download
