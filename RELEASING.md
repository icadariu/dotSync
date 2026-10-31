# Releasing

A release is a `vX.Y.Z` tag on `main`. Every PR merged since the previous tag
goes into that one release. The `release` workflow builds the binaries with
GoReleaser and publishes the GitHub release: linux and darwin archives for
amd64 and arm64, a `checksums.txt`, and (on public repos) a build provenance
attestation.

Versions follow [SemVer](https://semver.org) from the Conventional Commit
subjects since the previous tag: `fix` (and anything else) bumps the patch,
`feat` the minor, a breaking change (`type!:` or a `BREAKING CHANGE` footer)
the major. While the major version is `0`, a breaking change bumps the minor.

## Release from GitHub (the usual way)

1. Actions -> **release** -> **Run workflow** on `main`, with `dry_run` ticked
   (the default). The run summary shows the next version and every commit that
   goes into it. `bump` overrides the computed bump if needed.
2. Run it again with `dry_run` unticked. It tests `main`, tags it with the
   computed version and publishes the release in the same run.

From a terminal: `gh workflow run release -f dry_run=false` (add `-f bump=minor`
to override), then `gh run watch`.

## Release from a terminal (signed tag)

On an up-to-date `main` with a clean tree:

```sh
make release               # tags the next version, after asking [y/N]
make release BUMP=patch    # force the bump (patch, minor or major), still asks
make release TAG=vX.Y.Z    # or pick the version yourself, no prompt
```

Without `TAG`, `make release` fetches the tags and computes the next version
with `.github/next-version.sh` (auto bump from the commits, unless `BUMP` is
given), then asks before tagging. It refuses unless the
tag is SemVer, you are on `main` in sync with `origin/main`, the tree is clean
and the tag is new; then it runs the tests, creates a signed annotated tag and
pushes it, which starts the same workflow.

## Check the result

`gh release view vX.Y.Z`. On a public repo, verify an archive with
`gh attestation verify <archive> -R <owner>/<repo>`.

## Changelog

The GitHub release notes are the changelog. They are generated from the
Conventional Commit subjects since the previous tag, grouped into Breaking
changes, Features, Bug fixes and Other; `docs`, `test`, `ci`, `chore`, `build`
and `style` commits are left out. Write commit subjects with that in mind.

## When a release fails

Tags matching `v*` are protected (signed, no deletion, no force-push), so a bad
tag is never moved or reused. Fix the problem on `main` and release the next
patch version.

Releases are immutable: once a release is published, GitHub reserves its tag
name permanently, even if the release is deleted or the repository is deleted
and recreated under the same name. A version number is never reused; pick the
next one instead.
