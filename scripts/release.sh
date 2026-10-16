#!/bin/sh
# Record and tag a release; the build on builds.sr.ht publishes the binaries.
#
#   scripts/release.sh v0.2.0
set -eu

cd "$(dirname "$0")/.."

tag=${1:?usage: scripts/release.sh vX.Y.Z}

case $tag in
v[0-9]*.[0-9]*.[0-9]*) ;;
*) echo "tag must look like v1.2.3" >&2; exit 2 ;;
esac

if [ -n "$(git status --porcelain)" ]; then
	echo "working tree is not clean" >&2
	exit 1
fi

go test ./...

# install.sh and install.ps1 read this file to find the latest release.
echo "$tag" >RELEASE
git add RELEASE
git commit -m "Release $tag"
git tag -a "$tag" -m "dotmgr $tag"

echo "Now push: git push origin HEAD $tag"
