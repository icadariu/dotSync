# Managed by github-utils templates from ai-setup (config/claude/user_scripts/go_version_template).
# Edit it there, not in this repo: 'github-utils templates --apply' overwrites local edits.
#
# Version injection for a Go app. In the project Makefile set MAIN (main
# package) and BIN (output binary) before the include:
#
#   MAIN := ./cmd/myapp
#   BIN  := myapp
#   include .make/version.mk
#
# VERSION is branch-aware: on main it is `git describe --tags --always --dirty`;
# on any other branch (or detached HEAD) --dirty is dropped and -dev appended,
# so --version prints e.g. "v0.0.1-3-gabc1234-dev (built 23-May-2026_12:00, commit abc1234)".
# LDFLAGS is ignored from the environment but can be overridden on the make
# line; GOFLAGS on the make line reaches go through the environment:
#   make install GOFLAGS=-trimpath

MAIN ?= .
BIN  ?= $(notdir $(CURDIR))

BRANCH    := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo dev)
ifeq ($(BRANCH),main)
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
else
VERSION   := $(shell git describe --tags --always 2>/dev/null || echo dev)-dev
endif
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILDTIME := $(shell LC_ALL=C date -u +%d-%b-%Y_%H:%M)
LDFLAGS   := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILDTIME)"

.PHONY: build install version

build: ## Build ./$(BIN) with version metadata
	go build $(LDFLAGS) -o $(BIN) $(MAIN)

install: ## Build and install into $$GOPATH/bin with version metadata
	go install $(LDFLAGS) $(MAIN)

version: ## Print the version that would be embedded
	@echo "$(BIN) $(VERSION) (built $(BUILDTIME), commit $(COMMIT))"
