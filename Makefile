.PHONY: help bootstrap tidy build run snapshot release clean

BIN := bin/server

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

bootstrap: tidy build ## First-run setup: deps, build

tidy: ## Resolve and pin module dependencies
	go mod tidy

build: ## Build the server binary into bin/
	go build -trimpath -o $(BIN) ./cmd/server

run: ## Run the server locally
	go run ./cmd/server

snapshot: ## Local goreleaser build without publishing
	goreleaser release --snapshot --clean

release: ## Build and publish release archives with goreleaser
	goreleaser release --clean

clean: ## Remove build output
	rm -rf bin dist
