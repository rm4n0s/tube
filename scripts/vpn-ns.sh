#!/usr/bin/env bash
# vpn-ns.sh runs a command inside a private user and network namespace, where the process is
# root of the namespace, may make TUN devices, and has its own loopback and no route to anything
# else. Nothing on the machine changes and no real root is needed. The tests against the real
# OpenVPN server run here (docs/SPEC.md §16.3).
#
#   scripts/vpn-ns.sh -- <command> [args...]
#
# The namespace has no internet, so the Go modules are downloaded first. TUBE_NS=1 tells the
# tests that they are inside it. Run the server tests one package at a time (go test -p 1): two
# servers of two packages at once would share the namespace's network.
#
# Exit status: the command's own; 2 for a mistake in the arguments; 1 when the machine does not
# allow the namespace.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

usage() {
	echo "usage: scripts/vpn-ns.sh -- <command> [args...]" >&2
	exit 2
}

[[ "${1:-}" == "--" ]] || usage
shift
[[ $# -gt 0 ]] || usage

if ! why="$(unshare -Urn true 2>&1)"; then
	echo "vpn-ns.sh: cannot make a user and network namespace: $why" >&2
	echo "vpn-ns.sh: unprivileged user namespaces must be allowed (sysctl user.max_user_namespaces;" >&2
	echo "vpn-ns.sh: on Ubuntu 24.04 and later also kernel.apparmor_restrict_unprivileged_userns=0)" >&2
	exit 1
fi

go mod download

export TUBE_NS=1
exec unshare -Urn sh -c 'ip link set lo up && exec "$@"' sh "$@"
