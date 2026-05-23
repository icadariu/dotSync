# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`dotsync` is a per-user dotfile symlink manager (in the spirit of GNU Stow / chezmoi). It reads a YAML config of `(src, dst)` entries and reconciles `dst` paths as symlinks pointing to `src` in the user's dotfiles repository. The CLI is built on `spf13/cobra` and ships as a single static Go binary.

## Common commands

```sh
make build              # build ./dotsync with version ldflags
make install            # go install to $GOPATH/bin
make test               # full test suite
make test-race          # with -race
make test-cover         # coverage.out + go tool cover -func summary
make vet                # go vet ./...
make tidy               # go mod tidy + verify
make lint               # vet + golangci-lint if installed
make clean
make help               # list all annotated targets

# Run one test (no make wrapper):
go test ./internal/config -run TestConfig_NormalizeIDs -v
go test ./cmd/dotsync    -run TestE2E_AddAndList      -v
```

## Big picture

The codebase is two layers:

- **`cmd/dotsync/*.go`** — one file per cobra subcommand (`add`, `delete`, `edit`, `list`, `plan`, `apply`, `version`, `completion`, …). Each command auto-registers itself in an `init()` that calls `rootCmd.AddCommand(...)`. To add a subcommand, drop a new file in this directory following the same pattern; no central registry to edit.
- **`internal/`** — pure logic, no cobra:
  - `config` — `Config`/`Entry` structs, `Load`/`Save`, path resolution (`ResolveSrcPath`, `ResolveDstPath`, `expandHome`), `NormalizeIDs`. Stored `src`/`dst` are always absolute on disk; `$HOME` / `~/` are still expanded on read for back-compat.
  - `linker` — the reconciliation engine. Classifies each entry into a `Status` (`StatusOK`, `StatusLink`, `StatusRelink`, `StatusConflict`, `StatusError`). Both `plan` and `apply` are thin shells around this — change behavior here, not in the cobra files.
  - `diff` — unified diff used by `plan`'s output for `~ replace` conflicts.
  - `color` — ANSI color helpers; respects `NO_COLOR`.
  - `prompt` — interactive `y/n` confirmations; reads from `prompt.Stdin` so tests can inject.
  - `testenv` — **test-only** sandbox harness (see below).

### Config path resolution

In every command, use `cfgPath()` from `cmd/dotsync/main.go` — it resolves in this exact order: `--config` flag → `DOTSYNC_CONFIG` env → `~/.dotsync.yaml`. Never read `$HOME/.dotsync.yaml` directly.

### Version

`version`, `commit`, and `buildDate` are package-level `var`s in `cmd/dotsync/version.go`, injected by the Makefile via `-ldflags "-X main.version=... -X main.commit=... -X main.buildDate=..."`. The same string is printed by `dotsync version` and `dotsync --version`. If you add a new var, wire it through the Makefile's `LDFLAGS` too.

## Test sandboxing — read this before writing tests

`internal/testenv` is the only way tests should touch the filesystem. `testenv.New(t)` creates a fresh `/tmp/dotsync_tests/testN` directory and points `HOME` and `DOTSYNC_CONFIG` into it via `t.Setenv`. Helpers (`WriteRepo`, `WriteHome`, `WriteConfig`, `AssertSymlink`, `AssertBackup`, …) operate inside that sandbox.

- Never write to or read from the user's real `~/.dotsync.yaml`.
- Never `rm -rf /tmp/dotsync_tests` from a test — sandbox cleanup is the user's cron, not the test.
- For unit tests in `internal/config`, `testenv.NextTestDir()` returns just the sandbox path without env overrides.

E2E tests live in `cmd/dotsync/e2e_test.go` and drive `rootCmd.SetArgs(...) + rootCmd.Execute()`; use `captureStdout` to capture command output.

## Conventions worth knowing

- `Entry.ID` is assigned automatically and renumbered to `1..N` after `add`/`delete` via `config.NormalizeIDs` — don't rely on stable IDs.
- `apply` backs up displaced files with a collision-safe suffix (`.bk`, `.bk.1`, `.bk.2`, …) unless `--no-backup` is given.
- `delete` uses `os.Remove` only on `dst` if it's a symlink — real files/directories are left in place.
- `plan` output is git/chezmoi style; the prefixes (`= unchanged`, `+ create`, `~ relink`, `~ replace`, `! error`) come from `Status.String()` in `internal/linker`.
