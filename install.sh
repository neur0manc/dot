#!/bin/sh
# Install dotmgr on Linux or macOS.
#
#   curl -fsSL https://git.sr.ht/~stephan/dotmgr/blob/main/install.sh | sh
#
# Environment:
#   DOTMGR_VERSION     release tag to install (default: the latest release)
#   DOTMGR_INSTALL_DIR target directory (default: ~/.local/bin)
#   DOTMGR_BASE_URL    where release files are served (for mirrors and tests)
set -eu

repo=https://git.sr.ht/~stephan/dotmgr

fail() {
	echo "install: $*" >&2
	exit 1
}

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		fail "curl or wget is required"
	fi
}

case $(uname -s) in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported operating system $(uname -s); on Windows use install.ps1" ;;
esac

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "unsupported architecture $(uname -m)" ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

version=${DOTMGR_VERSION:-}
if [ -z "$version" ]; then
	download "$repo/blob/main/RELEASE" "$tmp/RELEASE" || fail "cannot determine the latest release"
	version=$(tr -d ' \r\n' <"$tmp/RELEASE")
fi

base=${DOTMGR_BASE_URL:-$repo/refs/download/$version}
name=dotmgr-$os-$arch.tar.gz

echo "Installing dotmgr $version ($os/$arch)"

download "$base/$name" "$tmp/$name" || fail "cannot download $base/$name"
download "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail "cannot download checksums"

expected=$(awk -v n="$name" '$2 == n { print $1 }' "$tmp/SHA256SUMS")
[ -n "$expected" ] || fail "no checksum listed for $name"

if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$name" | awk '{ print $1 }')
else
	actual=$(shasum -a 256 "$tmp/$name" | awk '{ print $1 }')
fi

[ "$actual" = "$expected" ] || fail "checksum mismatch for $name"

dir=${DOTMGR_INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$dir"
command -v tar >/dev/null 2>&1 || fail "tar is required"
tar -xzf "$tmp/$name" -C "$tmp" dotmgr
chmod 755 "$tmp/dotmgr"
mv "$tmp/dotmgr" "$dir/dotmgr"

echo "Installed $dir/dotmgr"

case ":$PATH:" in
*":$dir:"*) ;;
*) echo "Note: $dir is not in your PATH." ;;
esac
