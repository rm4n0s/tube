// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ellipsis ends a text that was cut.
const ellipsis = "…"

// fit cuts s to width cells, with "…" as the last cell when it was longer, and fills it with
// spaces to exactly width cells. s may hold styles.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) > width {
		s = ansi.Truncate(s, width, ellipsis)
	}
	if pad := width - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// clean makes pasted or typed text safe for one line: every control character (C0, DEL, C1),
// new line and tab becomes a space, and invalid UTF-8 becomes U+FFFD.
func clean(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == utf8.RuneError && size == 1:
			b.WriteRune(utf8.RuneError)
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ago says how long ago then was: "never" for the zero time, "just now", "5 min ago",
// "3 hours ago", "2 days ago", and the date after 30 days.
func ago(now, then time.Time) string {
	if then.IsZero() {
		return "never"
	}
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	case d < 2*time.Hour:
		return "1 hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d/time.Hour))
	case d < 48*time.Hour:
		return "1 day ago"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d/(24*time.Hour)))
	}
	return then.Format(time.DateOnly)
}

// wrap breaks text into lines of at most width cells, at spaces; a word longer than the width is
// cut by fit when it is drawn.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
