// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// field is a one-line text input: the text and a cursor.
type field struct {
	text   []rune
	cursor int
	// limit is the largest number of characters; 0 is no limit.
	limit int
	// placeholder is drawn, dim, when the field is empty.
	placeholder string
	// secret draws nothing of the text, and no cursor that moves: the field looks empty.
	secret bool
}

// value is the field's text.
func (f field) value() string { return string(f.text) }

// set replaces the text, made safe for one line, and puts the cursor at the end.
func (f *field) set(s string) {
	f.text = nil
	f.cursor = 0
	f.insert(s)
}

// insert inserts text, made safe for one line, at the cursor, as far as the limit allows.
func (f *field) insert(s string) {
	for _, r := range clean(s) {
		if f.limit > 0 && len(f.text) >= f.limit {
			return
		}
		f.text = append(f.text[:f.cursor], append([]rune{r}, f.text[f.cursor:]...)...)
		f.cursor++
	}
}

// paste inserts pasted text.
func (f *field) paste(s string) { f.insert(s) }

// clear empties the field.
func (f *field) clear() {
	f.text, f.cursor = nil, 0
}

// key handles a key press and says whether the field used it.
func (f *field) key(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "left":
		f.cursor = max(f.cursor-1, 0)
	case "right":
		f.cursor = min(f.cursor+1, len(f.text))
	case "home", "ctrl+a":
		f.cursor = 0
	case "end", "ctrl+e":
		f.cursor = len(f.text)
	case "backspace":
		if f.cursor > 0 {
			f.text = append(f.text[:f.cursor-1], f.text[f.cursor:]...)
			f.cursor--
		}
	case "delete":
		if f.cursor < len(f.text) {
			f.text = append(f.text[:f.cursor], f.text[f.cursor+1:]...)
		}
	case "ctrl+u":
		f.clear()
	case "ctrl+w":
		start := f.cursor
		for start > 0 && unicode.IsSpace(f.text[start-1]) {
			start--
		}
		for start > 0 && !unicode.IsSpace(f.text[start-1]) && f.text[start-1] != '/' {
			start--
		}
		if start == f.cursor && start > 0 {
			start-- // a "/" alone
		}
		f.text = append(f.text[:start], f.text[f.cursor:]...)
		f.cursor = start
	default:
		if msg.Text == "" {
			return false
		}
		f.insert(msg.Text)
	}
	return true
}

// view draws the field in width cells: the part of the text around the cursor, with the cursor
// reversed when the field is focused. A secret field draws nothing of its text.
func (f field) view(width int, focused bool, st styles) string {
	cursor := st.selected.Render(" ")
	switch {
	case f.secret:
		if focused {
			return fit(cursor, width)
		}
		return fit("", width)
	case len(f.text) == 0:
		text := st.dim.Render(f.placeholder)
		if focused {
			text = cursor + text
		}
		return fit(text, width)
	}
	// The first character shown: the cursor, and as much before it as fits.
	room := max(width-1, 1)
	start, used := f.cursor, 0
	for start > 0 {
		w := lipgloss.Width(string(f.text[start-1]))
		if used+w > room {
			break
		}
		used += w
		start--
	}
	var b strings.Builder
	cells := 0
	for i := start; i <= len(f.text); i++ {
		ch := " "
		if i < len(f.text) {
			ch = string(f.text[i])
		}
		w := lipgloss.Width(ch)
		if cells+w > width {
			break
		}
		cells += w
		if i == f.cursor && focused {
			b.WriteString(st.selected.Render(ch))
		} else if i < len(f.text) {
			b.WriteString(ch)
		}
	}
	return fit(b.String(), width)
}
