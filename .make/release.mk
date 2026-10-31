# Managed by github-utils templates from ai-setup (config/claude/user_scripts/go_release_template).
# Edit it there, not in this repo: 'github-utils templates --apply' overwrites local edits.
#
# Signed-tag release from a terminal; pairs with .github/workflows/release.yml
# and .github/next-version.sh. Pushing the tag is the whole release: CI builds
# and publishes it. TAG (not VERSION) so it never collides with version.mk.

.PHONY: release _release

release: ## Tag and push a signed release: make release [TAG=vX.Y.Z | BUMP=patch|minor|major] (default: auto bump, asks first)
	@[ -z "$(TAG)" ] || [ -z "$(BUMP)" ] || { echo "use TAG or BUMP, not both"; exit 1; }
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || { echo "release from main only"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "working tree is not clean"; exit 1; }
	@git fetch -q origin main --tags
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "main is not in sync with origin/main"; exit 1; }
	@# The default tag is computed here, after the fetch, so stale local tags cannot pick a taken version.
	@if [ -n "$(TAG)" ]; then $(MAKE) --no-print-directory _release TAG='$(TAG)'; else \
		tag=$$(bash .github/next-version.sh '$(BUMP)') || exit 1; \
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
