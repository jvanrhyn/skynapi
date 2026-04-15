BINARY   := skynapi
CMD      := ./cmd/api
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILT    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -ldflags "-X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT) -X main.BuildTime=$(BUILT)"

.PHONY: build test lint clean migrate-up migrate-down

build:
	go build $(LDFLAGS) -o bin/$(BINARY) $(CMD)

test:
	go test ./... -race -count=1

lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run ./...

clean:
	rm -rf bin/

migrate-up:
	@command -v psql >/dev/null 2>&1 || (echo "psql not found" && exit 1)
	psql "$(DB_URL)" -f migrations/002_weather_cache.up.sql

migrate-down:
	@command -v psql >/dev/null 2>&1 || (echo "psql not found" && exit 1)
	psql "$(DB_URL)" -f migrations/002_weather_cache.down.sql
