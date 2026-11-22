#!/bin/sh
# Prepares a release: turns the "Unreleased" section of CHANGELOG.md into the
# given version with today's date, starts a new empty "Unreleased" section,
# pins the README's quick start to it, commits both and tags the commit
# vVERSION. It pushes nothing. Pushing the tag makes CI publish the image and
# the GitHub release.
#
# Usage: scripts/release.sh 0.1.0
set -eu

version=${1:-}
tag="v$version"
changelog=CHANGELOG.md

fail() {
	echo "release: $*" >&2
	exit 1
}

echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$' ||
	fail "give the version as X.Y.Z or X.Y.Z-suffix, for example VERSION=0.1.0"

cd "$(git rev-parse --show-toplevel)"

[ "$(git branch --show-current)" = master ] || fail "switch to master first"
[ -z "$(git status --porcelain)" ] || fail "commit or stash the changes first"
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && fail "tag $tag already exists"
grep -q "^## $version " "$changelog" && fail "$changelog already has a section for $version"
grep -q '^## Unreleased$' "$changelog" || fail "$changelog has no \"## Unreleased\" section"

notes=$(awk '/^## / { if (found) exit; found = ($0 == "## Unreleased"); next } found' "$changelog")
echo "$notes" | grep -q '[^[:space:]]' || fail "the \"Unreleased\" section of $changelog is empty"

tmp=$(mktemp)
awk -v heading="## $version - $(date +%Y-%m-%d)" '
	$0 == "## Unreleased" { print; print ""; print heading; next }
	{ print }
' "$changelog" >"$tmp"
cat "$tmp" >"$changelog"
rm "$tmp"

# Pin the README's quick start to this release. A pre-release suffix is left alone.
if echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
	sed -E -i.bak \
		-e "s|(runs release \`)[0-9.]+(\`)|\1$version\2|" \
		-e "s|(magellanica-photos/)v[0-9.]+(/deploy/compose.image.yaml)|\1$tag\2|" \
		-e "s|^(IMAGE_TAG=)[0-9.]+\$|\1$version|" README.md
	rm -f README.md.bak
fi

git commit -q -m "Release $version" -- "$changelog" README.md
git tag -a "$tag" -m "$tag"

echo "Committed \"Release $version\" and tagged it $tag. To publish:"
echo
echo "  git push --follow-tags"
