BINARY    := dotsync
MAIN      := ./cmd/dotsync
MODULE    := $(shell go list -m)
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILDTIME := $(shell date +%y-%m-%d_%H:%M)
LDFLAGS   := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILDTIME)"

.DEFAULT_GOAL := help

.PHONY: build install test test-race test-cover verify vet tidy lint version clean completions help

build: ## Build the binary to ./$(BINARY)
	go build $(LDFLAGS) -o $(BINARY) $(MAIN)

install: ## Build and install the binary to $$GOPATH/bin
	go install $(LDFLAGS) $(MAIN)

# Single test entry point. Opt in via env:
#   make test RACE=1   -> race detector
#   make test COVER=1  -> coverage report
test: ## Run tests (set RACE=1 or COVER=1 to opt in)
	@flags="-count=1"; \
	if [ "$(RACE)" = "1" ]; then flags="$$flags -race"; fi; \
	if [ "$(COVER)" = "1" ]; then flags="$$flags -coverprofile=coverage.out"; fi; \
	go test ./... $$flags
	@if [ "$(COVER)" = "1" ]; then go tool cover -func=coverage.out; fi

# Hidden compatibility aliases (kept but not advertised in `make help`).
test-race:
	@$(MAKE) --no-print-directory test RACE=1

test-cover:
	@$(MAKE) --no-print-directory test COVER=1

verify: vet tidy lint ## Run vet, tidy and lint (full static-check sweep)

# Hidden individual checks (callable but not shown in help).
vet:
	go vet ./...

tidy:
	go mod tidy
	go mod verify

lint: vet
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run || true

version: ## Print the version string that will be baked into the binary
	@echo "$(BINARY) $(VERSION) (built $(BUILDTIME), commit $(COMMIT))"

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
