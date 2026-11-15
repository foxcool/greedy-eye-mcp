.PHONY: help bootstrap tidy build test run snapshot release clean

BIN := bin/server

# Stamp the build so the running binary can say what it is (see internal/buildinfo).
PKG := github.com/foxcool/greedy-eye-mcp/internal/buildinfo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

bootstrap: tidy build ## First-run setup: deps, build

tidy: ## Resolve and pin module dependencies
	go mod tidy

build: ## Build the server binary into bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

test: ## Run unit tests
	go test ./...

run: ## Run the server locally
	go run ./cmd/server

snapshot: ## Local goreleaser build without publishing
	goreleaser release --snapshot --clean

release: ## Build and publish release archives with goreleaser
	goreleaser release --clean

clean: ## Remove build output
	rm -rf bin dist
