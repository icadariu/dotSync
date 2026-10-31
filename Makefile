MAIN      := ./cmd/dotsync
BIN       := dotsync
MODULE    := $(shell go list -m)

# The zsh dir must be on fpath before compinit: fpath=(~/.zsh/completions $fpath)
ZSH_COMPLETIONS_DIR  ?= $(HOME)/.zsh/completions
BASH_COMPLETIONS_DIR ?= $(HOME)/.local/share/bash-completion/completions

.DEFAULT_GOAL := help

include .make/version.mk .make/release.mk

.PHONY: autocompletion test test-race test-cover verify vet tidy lint clean completions demo help

autocompletion: build ## Install zsh and bash completion scripts
	mkdir -p $(ZSH_COMPLETIONS_DIR) $(BASH_COMPLETIONS_DIR)
	./$(BIN) completion zsh > $(ZSH_COMPLETIONS_DIR)/_$(BIN)
	./$(BIN) completion bash > $(BASH_COMPLETIONS_DIR)/$(BIN)

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
	@which golangci-lint > /dev/null 2>&1 || { echo "golangci-lint not found; install: https://golangci-lint.run/welcome/install/" >&2; exit 1; }
	golangci-lint run

clean: ## Remove build artifacts
	rm -f $(BIN) coverage.out

demo: install ## Record docs/demo.cast + docs/demo.gif (needs asciinema >= 3, agg)
	asciinema rec -q --overwrite --cols 110 --rows 32 -c ./docs/demo.sh docs/demo.cast
	agg --theme monokai docs/demo.cast docs/demo.gif

completions: ## Enable Makefile target autocompletion for zsh
	@echo 'Adding zsh completion for make...'
	@grep -qxF 'zstyle ":completion:*:make:*" tag-order "targets"' ~/.zshrc 2>/dev/null \
		|| echo 'zstyle ":completion:*:make:*" tag-order "targets"' >> ~/.zshrc
	@echo 'Done. Run "source ~/.zshrc" or restart your shell to activate.'

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
