#!/usr/bin/env bash
# checks.sh runs the standard checks of docs/IMPLEMENTATION.md rule 9 (CLAUDE.md "Definition of
# done"), in order, and stops at the first that fails. The heavy ones go through scripts/heavy.sh,
# so that at most two heavy commands run on this machine at once (CLAUDE.md "Memory (OOM)").
#
#   scripts/checks.sh               every check
#   scripts/checks.sh --no-server   every check but the tests against the real OpenVPN server
#                                   (while working; never to finish a step)
#
# A check whose script does not exist yet (scripts/error-tags.sh comes with step 0.2,
# scripts/vpn-ns.sh with step 1.4) is skipped with a line that says so. Exit status: 0 when every
# check passed, else 1.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

server=1
for arg in "$@"; do
	case "$arg" in
	--no-server) server=0 ;;
	*)
		echo "usage: scripts/checks.sh [--no-server]" >&2
		exit 2
		;;
	esac
done

heavy="$root/scripts/heavy.sh"
failed=""

# check <name> <command...> runs one check and remembers the first that failed.
check() {
	local name="$1"
	shift
	[[ -n "$failed" ]] && return
	echo "==> $name"
	if ! "$@"; then
		failed="$name"
	fi
}

# gofmt_clean fails, listing them, when a file of cmd or internal is not formatted.
gofmt_clean() {
	local files
	files="$(gofmt -l cmd internal)" || return 1
	if [[ -n "$files" ]]; then
		echo "not formatted (gofmt -w cmd internal):" >&2
		echo "$files" >&2
		return 1
	fi
}

# script_check <name> <script> <command...> runs a check that needs a script of this folder, or
# says that the script does not exist yet.
script_check() {
	local name="$1" script="$2"
	shift 2
	[[ -n "$failed" ]] && return
	if [[ ! -x "$root/scripts/$script" ]]; then
		echo "==> $name: skipped, scripts/$script does not exist yet"
		return
	fi
	check "$name" "$@"
}

check "gofmt" gofmt_clean
check "go vet" go vet ./...
check "golangci-lint" "$heavy" -- golangci-lint run
check "tests, race detector" "$heavy" -- GOMEMLIMIT=3GiB go test -race -p 2 ./...
check "build without cgo" env CGO_ENABLED=0 go build ./...
check "tests without cgo" "$heavy" -- CGO_ENABLED=0 go test ./...
script_check "error tags" error-tags.sh "$root/scripts/error-tags.sh" --untested
if [[ $server -eq 1 ]]; then
	script_check "tests against the real OpenVPN server" vpn-ns.sh \
		"$heavy" --alone -- "$root/scripts/vpn-ns.sh" -- go test -race -tags tube_server ./...
else
	echo "==> tests against the real OpenVPN server: skipped, --no-server"
fi

if [[ -n "$failed" ]]; then
	echo "FAILED: $failed" >&2
	exit 1
fi
echo "all checks passed"
