BINARY   := skynapi
CMD      := ./cmd/api
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILT    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -ldflags "-X main.Version=$(VERSION) -X main.CommitHash=$(COMMIT) -X main.BuildTime=$(BUILT)"

.PHONY: build test test-web test-city-data lint clean migrate-up migrate-down

build:
	go build $(LDFLAGS) -o bin/$(BINARY) $(CMD)

test:
	go test ./... -race -count=1

test-web:
	node --test tests/*.test.mjs

lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run ./...

clean:
	rm -rf bin/

# Applies every migration in lexical order, matching what
# initdb/010-run-migrations.sh does on a fresh Docker volume.
migrate-up:
	@command -v psql >/dev/null 2>&1 || (echo "psql not found" && exit 1)
	@for f in migrations/*.up.sql; do \
		echo "Applying $$f"; \
		psql "$(DB_URL)" -v ON_ERROR_STOP=1 -f "$$f" || exit 1; \
	done

# Rolls back in reverse lexical order.
migrate-down:
	@command -v psql >/dev/null 2>&1 || (echo "psql not found" && exit 1)
	@for f in $$(/bin/ls -r migrations/*.down.sql); do \
		echo "Reverting $$f"; \
		psql "$(DB_URL)" -v ON_ERROR_STOP=1 -f "$$f" || exit 1; \
	done

# Offline parser/SQL generation tests; no database required.
test-city-data:
	python3 -m unittest discover -s scripts -p 'test_*.py'
