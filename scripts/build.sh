#!/bin/sh
# Cross-compile release archives and their checksums into dist/.
#
#   scripts/build.sh [version]
#
# The version defaults to the tag on HEAD, or dev-<commit> without one.
set -eu

cd "$(dirname "$0")/.."

commit=$(git rev-parse HEAD)
date=$(git show -s --format=%cI HEAD)
version=${1:-$(git describe --tags --exact-match HEAD 2>/dev/null || echo "dev-$(git rev-parse --short=12 HEAD)")}

ldflags="-s -w -X main.version=$version -X main.commit=$commit -X main.date=$date"

rm -rf build dist
mkdir -p dist

for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	os=${platform%/*}
	arch=${platform#*/}
	out=build/$os-$arch
	binary=dot
	[ "$os" = windows ] && binary=dot.exe

	echo "Building $os/$arch"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
		go build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$out/$binary" ./cmd/dot

	if [ "$os" = windows ]; then
		(cd "$out" && zip -q -9 "../../dist/dot-$os-$arch.zip" "$binary")
	else
		tar -C "$out" -czf "dist/dot-$os-$arch.tar.gz" "$binary"
	fi
done

(cd dist && sha256sum dot-* >SHA256SUMS)

# The version must have made it into the binary.
native=build/$(go env GOOS)-$(go env GOARCH)/dot
if [ -x "$native" ] && ! "$native" version | grep -qx "dot $version"; then
	echo "version injection failed: $("$native" version | head -1)" >&2
	exit 1
fi

cat dist/SHA256SUMS
