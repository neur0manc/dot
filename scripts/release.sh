#!/bin/sh
# Tag a release; pushing the tag makes GitHub Actions build and publish it.
#
#   scripts/release.sh v0.4.0
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

git tag -a "$tag" -m "dot $tag"

echo "Now push: git push origin HEAD $tag"
