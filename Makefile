BINARY  := dotsync
MAIN    := ./cmd/dotsync
MODULE  := $(shell go list -m)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")$(shell git diff --quiet HEAD 2>/dev/null || echo "-dirty")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)"

.DEFAULT_GOAL := help

.PHONY: build test test-race test-cover install lint vet tidy clean help completions sort

build: ## Build the binary to ./$(BINARY)
	go build $(LDFLAGS) -o $(BINARY) $(MAIN)

install: ## Install the binary via go install
	go install $(LDFLAGS) $(MAIN)

sort: build ## Sort .dotsync.yaml entries by src and renumber IDs
	./$(BINARY) sort

test: ## Run tests
	go test ./... -count=1

test-race: ## Run tests with race detector
	go test -race ./... -count=1

test-cover: ## Run tests with coverage report
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out

lint: vet ## Run go vet (add golangci-lint here if available)
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run || true

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy and verify go.mod / go.sum
	go mod tidy
	go mod verify

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out

completions: ## Enable Makefile target autocompletion for zsh
	@echo 'Adding zsh completion for make...'
	@grep -qxF 'zstyle ":completion:*:make:*" tag-order "targets"' ~/.zshrc 2>/dev/null \
		|| echo 'zstyle ":completion:*:make:*" tag-order "targets"' >> ~/.zshrc
	@echo 'Done. Run "source ~/.zshrc" or restart your shell to activate.'

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
