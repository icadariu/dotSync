# dotsync

Manage dotfile symlinks from a dotfiles repository to your home directory.

## Install

```sh
go install github.com/icadariu/dotsync/cmd/dotsync@latest
```

## Config file

dotsync resolves the config path in this order:

1. `--config <path>` flag (persistent, works with any subcommand)
2. `DOTSYNC_CONFIG` environment variable
3. `~/.dotsync.yaml` (default)

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
- `id`: assigned automatically; gaps from deletions are reused.
- `backup_suffix`: suffix appended to files displaced by a symlink (default `.bk`).

## Commands

```bash
dotsync add <src> <dst>
dotsync add --src <path> --dest <path>
dotsync list
dotsync ls
dotsync delete [<id>] [--force]
dotsync edit <id>
dotsync plan [--verbose|-v]
dotsync apply [--force|-f] [--no-backup]
dotsync completion bash|zsh|fish|powershell
dotsync version
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

`src` accepts an absolute path, `~/...`, or a path relative to your current
working directory — the value is always resolved to an absolute path before being
stored. `dst` resolves relative to `$HOME`, so `.zshrc` becomes `$HOME/.zshrc`
regardless of your current working directory; absolute paths (e.g.
`/etc/hosts.local`) are stored as-is.

### list / ls

Prints all entries as an aligned table of `ID`, `SRC`, `DST`.

### delete

Removes an entry from the config and unlinks the symlink at `dst` (uses
`os.Remove` — never recursive). If `dst` is a real file or directory it is
left untouched. If the id is omitted or not found, shows a list and prompts
for one. Requires confirmation unless `--force` is given.

### edit

Opens the entry in `$EDITOR` (default: `vi`) as YAML. Validates src and dst
on save before writing.

### plan

Dry-run: shows what `apply` would do without making any changes. Unchanged entries are hidden by default; pass `--verbose` / `-v` to show them.

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
skips prompts. On conflict, the existing destination is backed up with
`backup_suffix` (collision-safe: `.bk`, `.bk.1`, `.bk.2`, …) before the
symlink is created. Pass `--no-backup` to delete the conflicting destination
instead of backing it up.

### version

Prints the binary version, git commit, and build date.

```sh
dotsync version
# dotsync v0.1.0 (commit abc1234, built 2026-05-04)
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

## Shell completion

```sh
# zsh
dotsync completion zsh > ~/.zfunc/_dotsync
echo 'fpath=(~/.zfunc $fpath)' >> ~/.zshrc
autoload -Uz compinit && compinit

# bash
dotsync completion bash > /etc/bash_completion.d/dotsync
```

`--src` and `--dest` flags on `add` complete filesystem paths natively.
`delete <id>` and `edit <id>` complete entry IDs from your config, with
`src -> dst` shown alongside each suggestion.
