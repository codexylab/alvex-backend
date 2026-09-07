# ============================================================
# ALVEX Backend — Makefile
# ============================================================

.PHONY: run build seed migrate-up test test-short fmt fmt-check lint vet verify clean tidy

# --- Development ---
run:
	go run ./cmd/server/

# --- Build ---
build:
	go build -trimpath -ldflags="-s -w" -o bin/alvex ./cmd/server/
	go build -trimpath -ldflags="-s -w" -o bin/alvex-migrate ./cmd/migrate/

# --- Database ---
seed:
	go run ./cmd/seed/

migrate-up:
	go run ./cmd/migrate/

# --- Testing ---
test:
	go test ./... -v -cover

test-short:
	go test ./... -short

fmt:
	gofmt -w api cmd pkg scratch

fmt-check:
	@test -z "$$(gofmt -l api cmd pkg scratch)" || \
		(echo "Go files require formatting:"; gofmt -l api cmd pkg scratch; exit 1)

# --- Code quality ---
lint:
	golangci-lint run ./...

vet:
	go vet ./...

verify: fmt-check vet test
	go build ./cmd/server
	go build ./cmd/migrate

# --- Cleanup ---
clean:
	rm -rf bin/

# --- Dependencies ---
tidy:
	go mod tidy
	go mod download
