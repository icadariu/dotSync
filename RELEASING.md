# Releasing

A release is a signed `vX.Y.Z` tag pushed from `main`. The `release` workflow
then builds the binaries with GoReleaser and publishes the GitHub release:
linux and darwin archives for amd64 and arm64, a `checksums.txt`, and (on
public repos) a build provenance attestation.

## Steps

1. Pick the version with [SemVer](https://semver.org): `fix` commits bump the
   patch, `feat` commits the minor, breaking changes (`!`) the major. While the
   major version is `0`, a breaking change bumps the minor.
2. On an up-to-date `main` with a clean tree, run:

   ```sh
   make release TAG=vX.Y.Z
   ```

   It refuses unless the tag is SemVer, you are on `main` in sync with
   `origin/main`, the tree is clean and the tag is new; then it runs the tests,
   creates a signed annotated tag and pushes it.
3. Watch the build: `gh run watch $(gh run list -w release -L1 --json databaseId -q '.[0].databaseId')`.
4. Check the result: `gh release view vX.Y.Z`. On a public repo, verify an
   archive with `gh attestation verify <archive> -R <owner>/<repo>`.

## Changelog

The GitHub release notes are the changelog. They are generated from the
Conventional Commit subjects since the previous tag, grouped into Breaking
changes, Features, Bug fixes and Other; `docs`, `test`, `ci`, `chore`, `build`
and `style` commits are left out. Write commit subjects with that in mind.

## When a release fails

Tags matching `v*` are protected (signed, no deletion, no force-push), so a bad
tag is never moved or reused. Fix the problem on `main` and release the next
patch version.
