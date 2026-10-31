# dotSync

Manage dotfile symlinks from a dotfiles repository to your home directory.

![dotsync demo: add, list, plan, apply with backup of a conflicting file](docs/demo.gif)

Terminal replay of the same demo: `asciinema play docs/demo.cast`
(re-record with `make demo`).

How it compares with Stow, chezmoi, yadm and others: [docs/comparison.md](docs/comparison.md).

## Install

Download the archive for your OS and architecture (linux or darwin, amd64 or
arm64) from the [latest release](https://github.com/icadariu/dotSync/releases/latest),
check it against `checksums.txt`, and put `dotsync` on your `PATH`. Or build it
with Go:

```sh
go install github.com/icadariu/dotSync/cmd/dotsync@latest
```

## Config file

dotsync resolves the config path in this order:

1. `--config <path>` flag (persistent, works with any subcommand)
2. `DOTSYNC_CONFIG` environment variable
3. `~/.dotsync.yaml` (default)

Every command except `add` fails if the config file does not exist; the first `dotsync add` creates it.

Unknown keys are rejected, and comments are not preserved when dotsync rewrites the file (`add`/`delete`/`edit`/`sort`).

```yaml
version: 1
backup_suffix: .bk

entries:
  - id: 1
    src: /home/me/dotfiles/linux/dirs/.config/Code/User/settings-ubuntu.json
    dst: /home/me/.config/Code/User/settings-ubuntu.json

  - id: 2
    src: /home/me/dotfiles/linux/dirs/.zshrc
    dst: /home/me/.zshrc

  - id: 3
    src: /etc/hosts
    dst: /etc/hosts.local
```

- `src` / `dst`: always stored as absolute paths. `$HOME` and `~/` in stored
  values are still expanded on read, so old configs keep working.
- `id`: assigned automatically and renumbered `1..N` in file order on every load (so duplicate or missing IDs in a hand-edited file become addressable) and persisted on the next save.
- `backup_suffix`: suffix appended to files displaced by a symlink (default `.bk`).

## Commands

```bash
dotsync add <src> <dst>
dotsync add --src <path> --dest <path>
dotsync add --adopt <src> <dst>
dotsync list
dotsync ls
dotsync delete [<id>] [--force]
dotsync edit <id>
dotsync plan [<id>...] [--verbose|-v] [--no-pager]
dotsync apply [<id>...] [--force|-f] [--no-backup]
dotsync unlink [<id>...] [--force] [--restore]
dotsync completion bash|zsh|fish|powershell
dotsync --version
```

### add

Adds an entry mapping `src` (file or directory in your dotfiles repo) to `dst`
in your home directory. Rejects duplicate destinations and rejects sources that
do not exist on disk.

Positional arguments are accepted as a shorthand:

```sh
dotsync add ~/dotfiles/.zshrc ~/.zshrc
```

The `--src` and `--dest` flags remain available and take precedence when given:

```sh
dotsync add --src ~/dotfiles/.zshrc --dest ~/.zshrc
```

Flags and positional arguments can be mixed: `dotsync add --src ~/dotfiles/.zshrc ~/.zshrc`.

`src` and `dst` accept an absolute path, `~/...` or `$HOME/...`, or a
path relative to your current working directory. Both are resolved to absolute
paths before being stored, so the symlink `apply` creates always points at an
absolute path. `add` prints the stored entry, e.g.
`added 3: /home/me/dotfiles/.zshrc -> /home/me/.zshrc`. To target your home
directory from elsewhere, write `~/.zshrc`, not `.zshrc`.

As with `ln -s`, a `dst` ending in `/` names the directory to link into, keeping
the basename of `src`: `dotsync add dir1/dir2/file1 ~/some_dir/` stores
`~/some_dir/file1`, and `apply` creates `~/some_dir` if it is missing. Mind shell
tab completion: `dotsync add ~/dotfiles/nvim ~/.config/nvim/` stores
`~/.config/nvim/nvim`; drop the slash to replace `~/.config/nvim` itself.

Without the slash, a file `src` over an existing directory is ambiguous, so `add`
asks whether to link **i**nto it (the default), **r**eplace it on `apply`, or
**c**ancel; with no terminal to answer it fails and suggests the trailing slash.
Any other existing `dst` is saved with a `warning:` on stderr that `apply` will move it to a backup.

A `dst` that is `/`, your home directory or one of its parents would move your whole home aside, so `add`, `edit` and `apply` ask first (default no). `add --force` and `apply --force` skip the question but still print a `warning:` to stderr.

Pass `--adopt` to bring an existing file or directory under management:
dotsync moves `dst` to `src` (creating parent directories), records the entry,
and replaces `dst` with a symlink to it. It refuses if `src` already exists, if
`dst` is missing or already a symlink, or if the paths overlap. `src` and `dst`
must be on the same filesystem.

### list / ls

Prints all entries as an aligned table of `ID`, `SRC`, `DST`.

### delete / del

Removes an entry from the config and unlinks the symlink at `dst` (uses
`os.Remove` — never recursive). The link is removed only if it points at the
entry's `src`; a real file or directory, or a symlink pointing elsewhere, is
left untouched. If unlinking fails the entry is still removed and the command
exits 1. If the id is omitted or not found, shows a list and prompts for one;
with `--force` an unknown id is an error instead, and no confirmation is
asked.

### edit

Opens the entry in `$EDITOR` (default: `vi`) as YAML. Validates src and dst
on save before writing. If dst changed, the old dst is unlinked when it is a
symlink to the old src; anything else there is left in place.

### plan

Dry-run: shows what `apply` would do without making any changes. Unchanged entries are hidden by default; pass `--verbose` / `-v` to show them. Exits 1 if any entry has an error. Prints `no changes` when nothing would change.

Pass one or more entry IDs to limit the plan to those entries.

On a terminal, plans with changes open in a pager (`$DOTSYNC_PAGER`, then `$PAGER`, then `less`). Plain `less` runs as `less -FRX`, so output that fits one screen just prints. `DOTSYNC_PAGER=cat` or `--no-pager` disables it.

Two entries with the same `dst` are an error: the first one is applied, the later one is reported and skipped.

| Prefix | Meaning |
|--------|---------|
| `= unchanged` | Symlink already correct |
| `+ create` | Symlink will be created |
| `~ relink` | Existing dst is a symlink pointing to the wrong target — shows old → new target, then repoints to src |
| `~ replace` | Existing dst is a regular file or directory — backed up and replaced with a symlink |
| `! error` | Source missing or unreadable |

`~ relink` output includes the current and new symlink targets so you can see exactly what changes:

```
~ relink     /home/me/.zshrc
  /home/me/dotfiles/old/.zshrc → /home/me/dotfiles/new/.zshrc
old mode 120000
new mode 120000
```

Output is git/chezmoi-style and colored when stdout is a terminal: `+ create`
green, `~ relink`/`~ replace` yellow, `! error` red, and the unified diff body
follows git colors (additions green, removals red, hunks cyan, file headers
bold). Set `NO_COLOR=1` to disable.

Each `+ create` includes `new file mode 120000`
and the source content as additions; each `~ replace` includes `old mode` /
`new mode 120000` and a unified diff between the current destination and the
source — including per-file diffs when both are directories.

### apply

Creates or updates symlinks. Prompts per entry by default; `--force` / `-f`
skips prompts. On conflict, the existing destination is backed up to
`<dst><backup_suffix>` before the symlink is created. Only one backup is kept:
if it already exists, plan and the apply prompt say `backup <path> will be
overwritten`, and it is replaced on yes. With `--force` it is replaced
immediately and apply prints `warning: overwrote backup <path>` to stderr. A
backup owned by another user is never replaced; that entry fails. A symlink
pointing elsewhere is simply re-pointed, with no backup. Pass `--no-backup` to delete the conflicting destination
instead of backing it up; with `--no-backup` the prompt for a conflicting real
file defaults to `n`, a non-empty directory is refused rather than deleted, and
each deletion prints `warning: deleted <dst> (no backup)` to stderr, even with `--force`.
Exits 1 if any entry failed; the remaining entries are still processed.

Pass one or more entry IDs to apply only those entries. The prompt accepts `y`
(yes, the default), `n` (skip), `a` (yes to this and all remaining entries) and
`q` (stop; the remaining entries are left unchanged). With no input at all
(cron, CI, a closed pipe) apply stops with an error instead of assuming `y`;
use `--force` for unattended runs.

Each created symlink is reported as `linked <dst> -> <src>` (with `(backup: <path>)`
when a displaced file was backed up, or `(was -> <old target>)` when a symlink was re-pointed), followed by a summary such as
`2 linked, 1 unchanged` (`, N failed` and `, N skipped` appear only when non-zero).

### unlink

Removes the symlinks at `dst` for the given entry IDs (all entries if none)
without touching the config or `src`. A link is removed only if it points at
the entry's `src`; real files and foreign links are left in place with a note.
Asks for confirmation unless `--force`. With `--restore`, `<dst><backup_suffix>` is moved
back to `dst`; it is refused if owned by another user. Exits 1 if any entry failed; the rest are still processed.

### --version

Prints the binary version, build time (UTC) and git commit. There is no
`version` subcommand.

```sh
dotsync --version
# v0.1.0 (built 09-Sep-2026_14:30, commit abc1234)
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOTSYNC_CONFIG` | `~/.dotsync.yaml` | Config file path; overridden by `--config` |
| `EDITOR` | `vi` | Editor opened by `dotsync edit` |
| `NO_COLOR` | unset | Set to any value to disable ANSI color output |

## Security model

dotsync, like any tool that mutates the filesystem from a config file, should
not be run as root against an untrusted config. The threat model assumes the
user owns both the binary invocation and the YAML — this is the standard
expectation for a per-user dotfile manager (chezmoi, GNU Stow, and similar).
As a guard, dotsync refuses to run as root when the config file is owned by
another user, even with `--force`; completion commands are exempt.

Entries may point anywhere you can write, by design. Before applying a config
you did not write yourself (a cloned or shared dotfiles repo), run
`dotsync plan` and read every line: `apply` can create or replace any file in
those locations.

## Shell completion

```sh
# writes ~/.zsh/completions/_dotsync and
# ~/.local/share/bash-completion/completions/dotsync
make autocompletion
```

Override the targets with `ZSH_COMPLETIONS_DIR=...` /
`BASH_COMPLETIONS_DIR=...`. For zsh, the directory must be on `fpath` before
`compinit`: `fpath=(~/.zsh/completions $fpath)`.
`dotsync completion bash|zsh|fish|powershell` prints a script to stdout if you
prefer to install it by hand.

`--src` and `--dest` flags on `add` complete filesystem paths natively.
`delete <id>` and `edit <id>` complete an entry ID from your config, and
`plan`/`apply` complete any number of IDs (skipping ones already typed), with
`src -> dst` shown alongside each suggestion.
