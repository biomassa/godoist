#!/bin/sh
# release.sh makes a godoist release: it runs the tests, builds the binaries for Linux
# and macOS (amd64 and arm64), makes the Git tag, and publishes a GitHub release with the
# notes of the version from CHANGELOG.md.
#
#   scripts/release.sh v0.1.0            make and publish the release
#   scripts/release.sh --dry-run v0.1.0  build and show the notes, but do not tag or publish
#
# Before a release, move the [Unreleased] notes of CHANGELOG.md under "## [0.1.0] - date"
# and commit.
set -eu

dry=false
if [ "${1:-}" = "--dry-run" ]; then
	dry=true
	shift
fi
tag=${1:?"usage: scripts/release.sh [--dry-run] vX.Y.Z"}
case "$tag" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "the version must look like v0.1.0" >&2
	exit 1
	;;
esac
ver=${tag#v}
cd "$(dirname "$0")/.."

fail() {
	echo "$1" >&2
	exit 1
}
if ! $dry; then
	[ -z "$(git status --porcelain)" ] || fail "commit or stash the changes first"
	[ "$(git branch --show-current)" = main ] || fail "make a release from the main branch"
	! git rev-parse -q --verify "refs/tags/$tag" >/dev/null || fail "the tag $tag exists already"
fi
grep -q "^## \[$ver\] - " CHANGELOG.md || fail "CHANGELOG.md has no section \"## [$ver] - date\""

go vet ./...
go test ./...

rm -rf dist
mkdir dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
	os=${target%/*}
	arch=${target#*/}
	name=godoist_${ver}_${os}_${arch}
	mkdir "dist/$name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X github.com/biomassa/godoist/internal/version.Version=$ver" \
		-o "dist/$name/godoist" ./cmd/godoist
	cp README.md CHANGELOG.md "dist/$name/"
	[ ! -f LICENSE ] || cp LICENSE "dist/$name/"
	tar -C dist -czf "dist/$name.tar.gz" "$name"
	rm -r "dist/$name"
done
if command -v sha256sum >/dev/null; then
	(cd dist && sha256sum ./*.tar.gz >checksums.txt)
else
	(cd dist && shasum -a 256 ./*.tar.gz >checksums.txt)
fi

# The release notes are the section of this version in CHANGELOG.md, without its heading.
awk -v v="$ver" '
	index($0, "## [" v "]") == 1 { on = 1; next }
	on && /^## \[/ { exit }
	on && /^\[[^]]+\]: / { exit }
	on { print }
' CHANGELOG.md >dist/notes.md

ls -l dist
if $dry; then
	echo "dry run: the release notes are in dist/notes.md. No tag, no release."
	exit 0
fi
git tag -a "$tag" -m "godoist $ver"
git push origin main "$tag"
gh release create "$tag" dist/*.tar.gz dist/checksums.txt --title "godoist $ver" --notes-file dist/notes.md
echo "released $tag"
