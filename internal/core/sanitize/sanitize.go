// SPDX-License-Identifier: GPL-2.0-only

// Package sanitize makes hostile text safe to store, log and draw (docs/SPEC.md §6.5). A profile
// and everything a VPN server sends are whatever bytes their authors chose: an escape sequence
// in a profile's name or in a server's message would otherwise reach the user's terminal, and a
// new line in a search domain would add a line to resolv.conf.
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

// maxHostName is the longest name that DNS allows, in bytes, without the ending dot.
const maxHostName = 253

// maxLabel is the longest label of a name, in bytes.
const maxLabel = 63

// HostName says whether s can be written into resolv.conf as a search domain: 1 to 253
// bytes, labels of 1 to 63 letters, digits and hyphens (ASCII only) separated by single
// dots, no label starting or ending with a hyphen. A trailing dot is not accepted.
func HostName(s string) bool {
	if len(s) == 0 || len(s) > maxHostName {
		return false
	}
	for label := range strings.SplitSeq(s, ".") {
		if len(label) == 0 || len(label) > maxLabel || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if !hostNameByte(label[i]) {
				return false
			}
		}
	}
	return true
}

// hostNameByte says whether a byte may be in a label of a host name: an ASCII letter, a digit
// or a hyphen.
func hostNameByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
		return true
	}
	return false
}
