// SPDX-License-Identifier: GPL-2.0-only

// Package sanitize makes text that came from a client safe to store, log and draw
// (docs/SPEC.md §8.3). A login name or a client version is whatever bytes the other side chose
// to send: an escape sequence in it would otherwise reach the user's terminal.
package sanitize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// replacement stands for every character that Display does not let through.
const replacement = '\uFFFD'

// ellipsis ends a string that Display cut.
const ellipsis = '…'

// Display returns s with every unsafe character replaced by U+FFFD, cut to at most max
// characters; a cut string ends with "…". Unsafe are invalid UTF-8 (each bad byte), control
// characters (C0, DEL, C1), Unicode format characters (bidirectional overrides and isolates,
// zero-width characters, U+FEFF and the like) and the line and paragraph separators. The result
// is valid UTF-8 on one line, and drawing it moves no cursor and changes no direction.
func Display(s string, max int) string {
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	count := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if count == max-1 && i < len(s) {
			// One place is left and more than one character remains: the cut.
			b.WriteRune(ellipsis)
			return b.String()
		}
		if (r == utf8.RuneError && size == 1) || unsafe(r) {
			r = replacement
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

// unsafe says whether a character must not reach a terminal, a log line or a text field.
func unsafe(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
}
