# dotsync compared with other dotfile managers

dotsync is a small CLI that keeps a list of explicit `src -> dst` entries in a YAML config and reconciles each `dst` as a symlink to its `src`, using `plan` (dry run) and `apply`. This page says where that design fits, where it does not, and which other tool to reach for in those cases.

## At a glance

| Tool | Language/runtime | How files land | Mapping | Dry run with diffs | Per-host differences | Templates | Secrets/encryption |
| --- | --- | --- | --- | --- | --- | --- | --- |
| dotsync | Go, single static binary | Symlink only | Explicit entries (any src to any dst) | Yes (`plan`, unified diffs) | No (use one config per machine) | No | No |
| GNU Stow | Perl | Symlink | Mirrored tree (layout is the mapping) | Dry run (`-n`), no diffs | No | No | No |
| chezmoi | Go, single binary | Copy (symlink mode optional) | Naming conventions (`dot_`, `private_`, `executable_`) | Yes (`chezmoi diff`) | Templates | Yes (Go `text/template`) | Yes (password managers, age, gpg) |
| yadm | Shell script wrapping git | Git work tree in `$HOME` (no symlinks) | Naming conventions (alternate files by suffix) | - | Alternate files (`##os`, `##hostname`, `##class`) | Yes | Yes |
| Dotbot | Python | Symlink | Explicit entries (YAML/JSON directives) | No diff-style dry run | - | - | - |
| rcm | Shell tools (`rcup`, `lsrc`, `mkrc`) | Symlink (copy optional) | Mirrored tree | - | Host-specific and tag directories | - | - |
| home-manager | Nix | Symlinks into the Nix store | Nix expressions | - | Nix expressions | - | - |
| Bare git repo | git only | Git work tree in `$HOME` (no symlinks) | Tracked paths in `$HOME` | - | No | No | No |

A "-" means the fact is not established here; check the tool's documentation.

## Where dotsync is strong

- Explicit mapping: any file or whole directory can be linked to any destination, with no required mirrored directory layout.
- Reviewable changes: `plan` prints git/chezmoi-style output with unified diffs for conflicts, paged when there are changes, and `apply` shows the diff and asks per entry (y/n/a/q).
- Non-destructive by default: displaced files are backed up as `.bk`, `.bk.1`, ... and `unlink --restore` moves the newest backup back. `--no-backup` deletes instead but refuses non-empty directories.
- Safety checks: it rejects src/dst overlap (same path, nested, same file) and duplicate dst. Prompts never auto-confirm on EOF, so cron/CI must pass `--force`.
- Adopting existing files: `add --adopt` moves an existing dst into the repo and links it.
- Small footprint: a single static Go binary with four direct dependencies (cobra, pflag, yaml.v3, go-difflib), plus shell completion.
- Stays out of your repo: the config lives outside it (or can be a symlink into it), and dotsync never runs git.

## Where dotsync is weak

- No templating, so per-machine differences need one config per machine.
- No secrets handling or encryption. Anything in the repo is stored as-is.
- Symlinks only, with no copy mode. Apps that replace a symlink with a regular file on save silently break the link.
- No file permission or ownership management.
- No scripts or hooks, and no package installation. It only manages links.
- No git integration: you manage the repo with git yourself.
- Unix-like systems only. The editor and pager run through `sh`, and Windows is not supported or tested.
- `add`, `delete`, `edit` and `sort` rewrite the whole config with absolute paths. A hand-written `~/` entry becomes machine-specific after any write, so sharing one config across machines with different home directories (for example `/home/u` and `/Users/u`) needs care.
- Young project with a small feature set and a small user base compared to the others.

## When to pick something else

- You want a mature, ubiquitous, dependency-light tool and your repo already mirrors `$HOME`: GNU Stow.
- You need templates, secrets or encryption, run scripts, or machines that differ a lot: chezmoi.
- You want git itself to be the interface, with alternate files per host or class and encryption: yadm.
- You want one idempotent install script that also creates directories, cleans dead links and runs shell commands: Dotbot.
- You want host-specific and tag directories plus hooks, with symlinks: rcm.
- You want packages, config and rollbacks managed together as one declarative environment, and accept learning Nix: home-manager.
- You want no tool at all, only git: a bare git repo.

## Tool by tool

### GNU Stow

Homepage: <https://www.gnu.org/software/stow/>

Symlinks a "package" directory tree into a target directory, mirroring its layout (tree folding). There is no per-entry config file; the layout is the mapping.

Advantages over dotsync:

- Very mature and packaged almost everywhere.
- No config file to maintain: the directory layout is the configuration.

Disadvantages vs dotsync:

- No backups: conflicts abort.
- Dry run (`-n`) does not show diffs.
- No host logic built in.
- The source tree must mirror the target layout.

### chezmoi

Homepage: <https://www.chezmoi.io/>

Go single binary. Copies files by default (symlink mode is optional). Source state uses filename attributes such as `dot_`, `private_` and `executable_`.

Advantages over dotsync:

- Go `text/template` templates for per-machine differences.
- Password-manager integration and age/gpg encryption.
- Can run scripts, and has git integration.
- `chezmoi diff` and `chezmoi apply` cover the plan/apply workflow.

Disadvantages vs dotsync:

- Larger learning curve and more concepts to learn.
- Source files are renamed with attribute prefixes rather than kept as plain files.
- Copy mode means edits to the live file must be brought back into the source state.

### yadm

Homepage: <https://yadm.io/>

A shell script wrapping git with `$HOME` as the work tree. There are no symlinks, and you use git commands directly.

Advantages over dotsync:

- Alternate files by suffix (`##os`, `##hostname`, `##class`) and templates.
- Encryption and a bootstrap script.
- Files are real files, so apps that rewrite them cannot break a link.

Disadvantages vs dotsync:

- No separate plan/apply step: a checkout or pull changes the live files at once, so review happens with git (`yadm diff`, `yadm log`).
- The repo is your `$HOME`, so layout in the repo is tied to layout on disk.
- Requires working with git for everything.

### Dotbot

Homepage: <https://github.com/anishathalye/dotbot>

Python, usually vendored as a git submodule in the dotfiles repo. YAML/JSON config with `link`, `create`, `clean` and `shell` directives, plus plugins.

Advantages over dotsync:

- `create`, `clean` and `shell` directives and plugins go beyond linking.
- An idempotent install script that can be run on a fresh machine from the repo alone.
- Config lives in the repo, so it travels with the dotfiles.

Disadvantages vs dotsync:

- No diff-style dry run.
- Needs Python and usually a git submodule.
- No built-in per-entry add/delete/edit commands: the config is edited by hand.

### rcm

Homepage: <https://github.com/thoughtbot/rcm>

Shell tools (`rcup`, `lsrc`, `mkrc`). Symlinks by default, with copy optional.

Advantages over dotsync:

- Host-specific and tag directories.
- Hooks.
- Optional copy mode.

Disadvantages vs dotsync:

- Layout-based mapping rather than explicit entries.
- `lsrc` lists what would be linked, but there is no diff of conflicting content.

### home-manager

Homepage: <https://github.com/nix-community/home-manager>

Nix-based declarative user environment: packages and config together, with rollbacks and generations.

Advantages over dotsync:

- Manages packages as well as config files.
- Rollbacks and generations.
- Fully declarative, expressed in Nix.

Disadvantages vs dotsync:

- Requires Nix.
- Steep learning curve.
- Far more machinery than needed if you only want symlinks.

### Bare git repo

No tool: plain git with `--git-dir` and `--work-tree=$HOME`.

Advantages over dotsync:

- Zero dependencies beyond git.
- Files are real files, so no link can break.

Disadvantages vs dotsync:

- No symlinks, no dry run, no backup of displaced files.
- No per-host logic.
- Easy to accidentally show untracked `$HOME` files in `git status`.
