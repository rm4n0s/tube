#!/usr/bin/env bash
# Lists the error Tags of Tube (the first argument of errors.New, errors.NewErr and errors.NewErrf
# in the non-test code of cmd and internal, for every platform) and, with --untested, the Tags
# that no test asserts (CLAUDE.md "Definition of done", docs/SPEC.md §4.2 rule 7).
#
#   scripts/error-tags.sh                    every Tag: Tag, file:line, function
#   scripts/error-tags.sh --untested         the Tags no test asserts; prints nothing when all are
#                                            tested, except those docs/ERROR-TAGS-UNTESTED.md lists
#   scripts/error-tags.sh --untested --all   the same, with the listed ones kept in
#
# internal/archtest/errortags.go says exactly when a Tag counts as tested. A Tag that is not a
# string literal or a Tag constant is an error. The exit status is 1 when --untested printed
# something or a Tag is malformed. It needs no network and takes a second.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
# Tube is cgo-free; building the tool without cgo is faster and needs no C compiler.
export CGO_ENABLED=0
exec go run ./internal/archtest/cmd/errortags "$@" "$root"
