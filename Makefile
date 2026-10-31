BINARY    := dotsync
MAIN      := ./cmd/dotsync
MODULE    := $(shell go list -m)
BRANCH    := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo dev)
ifeq ($(BRANCH),main)
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
else
VERSION   := $(shell git describe --tags --always 2>/dev/null || echo dev)-dev
endif
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILDTIME := $(shell LC_ALL=C date -u +%d-%b-%Y_%H:%M)
LDFLAGS   := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILDTIME)"

# The zsh dir must be on fpath before compinit: fpath=(~/.zsh/completions $fpath)
ZSH_COMPLETIONS_DIR  ?= $(HOME)/.zsh/completions
BASH_COMPLETIONS_DIR ?= $(HOME)/.local/share/bash-completion/completions

.DEFAULT_GOAL := help

.PHONY: build install autocompletion test test-race test-cover verify vet tidy lint clean completions demo release _release help

build: ## Build the binary to ./$(BINARY)
	go build $(LDFLAGS) -o $(BINARY) $(MAIN)

install: ## Build and install the binary to $$GOPATH/bin
	go install $(LDFLAGS) $(MAIN)

autocompletion: build ## Install zsh and bash completion scripts
	mkdir -p $(ZSH_COMPLETIONS_DIR) $(BASH_COMPLETIONS_DIR)
	./$(BINARY) completion zsh > $(ZSH_COMPLETIONS_DIR)/_$(BINARY)
	./$(BINARY) completion bash > $(BASH_COMPLETIONS_DIR)/$(BINARY)

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

release: ## Tag and push a signed release: make release [TAG=vX.Y.Z] (default: next version, asks first)
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || { echo "release from main only"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "working tree is not clean"; exit 1; }
	@git fetch -q origin main --tags
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "main is not in sync with origin/main"; exit 1; }
	@# The default tag is computed here, after the fetch, so stale local tags cannot pick a taken version.
	@if [ -n "$(TAG)" ]; then $(MAKE) --no-print-directory _release TAG='$(TAG)'; else \
		tag=$$(bash .github/next-version.sh) || exit 1; \
		printf 'next version: %s (since %s). Tag and push? [y/N] ' "$$tag" "$$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || echo 'the first commit')"; \
		read -r ans || ans=; case "$$ans" in y|Y) ;; *) echo "aborted"; exit 1 ;; esac; \
		$(MAKE) --no-print-directory _release TAG=$$tag; \
	fi

# Internal: reached only through release, which runs the branch/tree/sync checks first.
_release:
	@echo "$(TAG)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$$' || { echo "usage: make release [TAG=vX.Y.Z]"; exit 1; }
	@! git rev-parse -q --verify "refs/tags/$(TAG)" >/dev/null || { echo "tag $(TAG) already exists"; exit 1; }
	go test -count=1 ./...
	git tag -s $(TAG) -m "$(TAG)"
	git push origin $(TAG)
	@echo "pushed $(TAG); follow the build with: gh run watch \$$(gh run list -w release -L1 --json databaseId -q '.[0].databaseId')"

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out

demo: install ## Record docs/demo.cast + docs/demo.gif (needs asciinema >= 3, agg)
	asciinema rec -q --overwrite --cols 110 --rows 32 -c ./docs/demo.sh docs/demo.cast
	agg --theme monokai docs/demo.cast docs/demo.gif

completions: ## Enable Makefile target autocompletion for zsh
	@echo 'Adding zsh completion for make...'
	@grep -qxF 'zstyle ":completion:*:make:*" tag-order "targets"' ~/.zshrc 2>/dev/null \
		|| echo 'zstyle ":completion:*:make:*" tag-order "targets"' >> ~/.zshrc
	@echo 'Done. Run "source ~/.zshrc" or restart your shell to activate.'

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
