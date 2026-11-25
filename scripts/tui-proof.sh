#!/usr/bin/env bash
# tui-proof.sh makes a proof of the TUI (method U of docs/IMPLEMENTATION.md): it builds tube, runs
# it isolated (scripts/isolated.sh) in a private tmux server, sends keys, and saves the screen as
# text. The user's own tmux, its server and its configuration are never touched.
#
#   scripts/tui-proof.sh <out.txt> [--config <fixture dir>] [--size 100x30] [--wait-first <regex>]
#                        [--keys '<tmux keys>' | --text '<text>' | --wait '<regex>']... -- [tube arguments]
#
#   --config <dir>       the configuration folder to copy in (scripts/testdata/three-profiles)
#   --size <cols>x<rows> the size of the terminal; 100x30 by default
#   --wait-first <re>    wait until a line of the screen matches; "Tube" by default
#   --keys '<keys>'      tmux key names (a Enter Escape C-c), separated by spaces; then 0.3 s
#   --text '<text>'      types the text as it is
#   --wait '<re>'        waits until a line of the screen matches (grep -E)
#
# Each wait lasts 15 seconds at most; then the script fails and prints the screen. The steps run
# in the order they are given. Exit status 64 for a mistake in these arguments.
set -euo pipefail

usage() {
	echo "usage: scripts/tui-proof.sh <out.txt> [--config <dir>] [--size 100x30] [--wait-first <regex>] [--keys '<keys>' | --text '<text>' | --wait '<regex>']... -- [tube arguments]" >&2
	exit 64
}

for tool in tmux go timeout; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "tui-proof.sh: $tool is not installed (on Fedora: sudo dnf install $tool)" >&2
		exit 1
	fi
done

[[ $# -ge 1 ]] || usage
out="$1"
shift
config=""
size="100x30"
wait_first="Tube"
steps=()
while [[ $# -gt 0 ]]; do
	case "$1" in
	--config | --size | --wait-first | --keys | --text | --wait)
		[[ $# -ge 2 ]] || usage
		case "$1" in
		--config) config="$2" ;;
		--size) size="$2" ;;
		--wait-first) wait_first="$2" ;;
		*) steps+=("$1" "$2") ;;
		esac
		shift 2
		;;
	--)
		shift
		break
		;;
	*) usage ;;
	esac
done
[[ "$size" =~ ^([0-9]+)x([0-9]+)$ ]] || usage
cols="${BASH_REMATCH[1]}"
rows="${BASH_REMATCH[2]}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/tube-proof.XXXXXX")"
socket="tube-proof-$$"
unset TMUX
export TMUX_TMPDIR="$tmp"
tm() { tmux -L "$socket" -f /dev/null "$@"; }
cleanup() {
	tm kill-server >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT

(cd "$root" && go build -o "$tmp/tube" ./cmd/tube)

isolated=("$root/scripts/isolated.sh")
if [[ -n "$config" ]]; then
	isolated+=(--config "$config")
fi
command="exec timeout -k 2 120 $(printf '%q ' "${isolated[@]}" -- "$tmp/tube" "$@")"
tm new-session -d -s proof -x "$cols" -y "$rows" "$command"
tm set-option -t proof remain-on-exit on >/dev/null

screen() { tm capture-pane -p -t proof; }

wait_for() {
	local regex="$1"
	for _ in $(seq 1 150); do
		if screen | grep -qE -- "$regex"; then
			return 0
		fi
		sleep 0.1
	done
	echo "tui-proof.sh: no line matched '$regex' within 15 seconds; the screen:" >&2
	screen >&2
	exit 1
}

wait_for "$wait_first"
i=0
while [[ $i -lt ${#steps[@]} ]]; do
	kind="${steps[$i]}"
	value="${steps[$((i + 1))]}"
	case "$kind" in
	--keys)
		read -r -a keys <<<"$value"
		tm send-keys -t proof "${keys[@]}"
		sleep 0.3
		;;
	--text) tm send-keys -t proof -l "$value" ;;
	--wait) wait_for "$value" ;;
	esac
	i=$((i + 2))
done
sleep 0.5
mkdir -p "$(dirname "$out")"
screen >"$out"
