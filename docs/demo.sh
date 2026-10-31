#!/usr/bin/env bash
# Scripted dotsync demo. Record with `make demo`.
# Runs in a throwaway HOME so the real ~/.dotsync.yaml is never touched.
set -euo pipefail

BIN=$(command -v dotsync)
SANDBOX=$(mktemp -d /tmp/demo.XXX)
export HOME=$SANDBOX DOTSYNC_CONFIG=$SANDBOX/.dotsync.yaml EDITOR=true
cd "$HOME"

mkdir -p dotfiles/nvim
printf 'set -g mouse on\n' >dotfiles/tmux.conf
printf 'vim.o.number = true\n' >dotfiles/nvim/init.lua
printf 'syntax on\nset number\n' >dotfiles/vimrc
printf 'syntax on\n' >.vimrc # pre-existing file -> conflict

# type CMD: echo a prompt with simulated typing, then run it.
type() {
  printf '\033[1;32m$\033[0m '
  local i
  for ((i = 0; i < ${#1}; i++)); do printf '%s' "${1:i:1}"; sleep 0.03; done
  sleep 0.4; echo
  eval "${1/#dotsync/$BIN}"
  sleep 1.5
}

type "ls -R dotfiles"
type "dotsync add dotfiles/tmux.conf ~/.tmux.conf"
type "dotsync add dotfiles/nvim/init.lua ~/.config/nvim/" # trailing / links into the dir
type "dotsync add dotfiles/vimrc ~/.vimrc"
type "dotsync list"
type "dotsync plan"
type "dotsync apply --force"
type "ls -go --time-style=+ ~/.tmux.conf ~/.config/nvim/init.lua ~/.vimrc*"
type "dotsync plan -v"
sleep 1
rm -rf "$SANDBOX"
