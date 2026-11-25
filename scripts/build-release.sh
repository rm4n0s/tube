#!/usr/bin/env bash
# build-release.sh builds the files of a release: for linux/amd64 and linux/arm64, an archive
# tube-<version>-linux-<arch>.tar.gz with tube, LICENSE and README.md, and SHA256SUMS, in
# <dist>/<version>/. It uploads nothing and makes no tag.
#
#   scripts/build-release.sh [--version <v>] [--dist <dir>]
#
#   --version <v>   the version; devVersion of internal/app/version.go without "-dev" by default
#   --dist <dir>    the folder of the releases; "dist" by default
#
# Exit status 64 for a mistake in these arguments.
set -euo pipefail

usage() {
	echo "usage: scripts/build-release.sh [--version <v>] [--dist <dir>]" >&2
	exit 64
}

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version=""
dist="dist"
while [[ $# -gt 0 ]]; do
	case "$1" in
	--version | --dist)
		[[ $# -ge 2 && -n "$2" ]] || usage
		case "$1" in
		--version) version="$2" ;;
		--dist) dist="$2" ;;
		esac
		shift 2
		;;
	*) usage ;;
	esac
done

if [[ -z "$version" ]]; then
	version="$(sed -n 's/^const devVersion = "\(.*\)"$/\1/p' "$root/internal/app/version.go")"
	version="${version%-dev}"
fi
# A version is digits, letters, dots, hyphens and plus signs: it becomes part of file names.
[[ "$version" =~ ^[0-9A-Za-z][0-9A-Za-z.+-]*$ ]] || {
	echo "build-release.sh: the version '$version' is not valid" >&2
	exit 64
}

for tool in go tar gzip sha256sum; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "build-release.sh: $tool is not installed" >&2
		exit 1
	fi
done

mkdir -p "$dist"
out="$(cd "$dist" && pwd)/$version"
mkdir -p "$out"
work="$(mktemp -d "${TMPDIR:-/tmp}/tube-release.XXXXXX")"
trap 'rm -rf "$work"' EXIT

for arch in amd64 arm64; do
	dir="$work/$arch"
	mkdir -p "$dir"
	(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
		-ldflags "-s -w -X github.com/rm4n0s/tube/internal/app.buildVersion=$version" \
		-o "$dir/tube" ./cmd/tube)
	cp "$root/LICENSE" "$root/README.md" "$dir/"
	chmod 0755 "$dir/tube"
	chmod 0644 "$dir/LICENSE" "$dir/README.md"
	archive="$out/tube-$version-linux-$arch.tar.gz"
	tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C "$dir" -cf - LICENSE README.md tube |
		gzip -n -9 >"$archive"
	echo "built $archive"
done

(cd "$out" && sha256sum tube-"$version"-linux-*.tar.gz >SHA256SUMS)
echo "wrote $out/SHA256SUMS"
