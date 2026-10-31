#!/usr/bin/env bash
# Managed by github-utils templates from ai-setup (config/claude/user_scripts/go_release_template); edit it there.
# Prints the next SemVer tag from the Conventional Commits since the last v*
# tag: breaking (`type!:` or a BREAKING CHANGE footer) -> major, feat -> minor,
# anything else -> patch; while the major is 0, breaking -> minor.
# Usage: next-version.sh [auto|patch|minor|major]   (default auto)
# Exits 1 with a message when there is nothing to release.
set -euo pipefail

bump=${1:-auto}
last=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)
range=${last:+$last..}HEAD
if [ -z "$(git rev-list "$range")" ]; then
  echo "nothing to release: no commits since ${last:-the first commit}" >&2
  exit 1
fi

if [ "$bump" = auto ]; then
  bump=patch
  if git log --format=%s "$range" | grep -Eq '^feat(\(.+\))?:'; then bump=minor; fi
  if git log --format=%s "$range" | grep -Eq '^[a-z]+(\(.+\))?!:' \
     || git log --format=%B "$range" | grep -q '^BREAKING CHANGE'; then bump=major; fi
fi

IFS=. read -r major minor patch <<<"${last#v}"
major=${major:-0} minor=${minor:-0} patch=${patch:-0}
case $bump in
  major) if [ "$major" -eq 0 ]; then minor=$((minor + 1)); else major=$((major + 1)); minor=0; fi; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
  *) echo "bump must be auto, patch, minor or major, got: $bump" >&2; exit 2 ;;
esac
echo "v$major.$minor.$patch"
