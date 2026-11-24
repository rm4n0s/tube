#!/usr/bin/env bash
# isolated.sh runs a command away from the user's own files (docs/SPEC.md §16.1): Tube's
# folders, the home folder, the XDG folders and the runtime folder are in a new temporary
# folder, which is removed afterwards.
#
#   scripts/isolated.sh [--config <fixture dir>] [--keep] -- <command> [args...]
#
#   --config <dir>   copy this folder in as Tube's configuration folder (profiles/...) before the
#                    command starts; the original is never changed
#   --keep           do not remove the temporary folder; its path is printed on standard error
#
# Inside the command:
#   TUBE_CONFIG_DIR, TUBE_STATE_DIR                  Tube's folders
#   HOME, XDG_CONFIG_HOME, XDG_STATE_HOME,
#   XDG_CACHE_HOME, XDG_DATA_HOME, XDG_RUNTIME_DIR   folders inside the temporary folder
#   TUBE_ISOLATED_ROOT                               the temporary folder
#
# Exit status: the command's own; 64 for a mistake in the arguments of this script.
set -euo pipefail

usage() {
	echo "usage: scripts/isolated.sh [--config <fixture dir>] [--keep] -- <command> [args...]" >&2
	exit 64
}

config=""
keep=0
while [[ $# -gt 0 ]]; do
	case "$1" in
	--config)
		[[ $# -ge 2 ]] || usage
		config="$2"
		shift 2
		;;
	--keep)
		keep=1
		shift
		;;
	--)
		shift
		break
		;;
	*) usage ;;
	esac
done
[[ $# -gt 0 ]] || usage
if [[ -n "$config" && ! -d "$config" ]]; then
	echo "isolated.sh: the fixture folder '$config' does not exist" >&2
	exit 64
fi

root="$(mktemp -d "${TMPDIR:-/tmp}/tube-iso.XXXXXX")"
chmod 700 "$root"
cleanup() {
	if [[ $keep -eq 1 ]]; then
		echo "isolated.sh: kept $root" >&2
	else
		rm -rf "$root"
	fi
}
trap cleanup EXIT

for sub in home config state xdg-config xdg-state xdg-cache xdg-data run; do
	mkdir -m 700 "$root/$sub"
done
if [[ -n "$config" ]]; then
	cp -R "$config"/. "$root/config"/
	chmod -R go-rwx "$root/config"
fi

export TUBE_ISOLATED_ROOT="$root"
export TUBE_CONFIG_DIR="$root/config"
export TUBE_STATE_DIR="$root/state"
export HOME="$root/home"
export XDG_CONFIG_HOME="$root/xdg-config"
export XDG_STATE_HOME="$root/xdg-state"
export XDG_CACHE_HOME="$root/xdg-cache"
export XDG_DATA_HOME="$root/xdg-data"
export XDG_RUNTIME_DIR="$root/run"

# Run the command and keep its exit status; the trap removes the folder.
set +e
"$@"
status=$?
exit "$status"
