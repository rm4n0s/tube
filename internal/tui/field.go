// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// field is a one-line text input: the text and a cursor.
type field struct {
	text   []rune
	cursor int
	// limit is the largest number of characters; 0 is no limit.
	limit int
	// placeholder is drawn, dim, when the field is empty.
	placeholder string
	// secret draws a "*" in place of each character of the text, never the text.
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

// secretMark is what a secret field draws in place of each of its characters.
const secretMark = '*'

// shown is the text as the field draws it: for a secret field a "*" in place of each character,
// so that the user sees that the keys arrive, and how many, and never what they are.
func (f field) shown() []rune {
	if !f.secret {
		return f.text
	}
	out := make([]rune, len(f.text))
	for i := range out {
		out[i] = secretMark
	}
	return out
}

// first is the first character that the field shows in width cells: the cursor is shown, and
// as much before it as fits.
func (f field) first(width int) int {
	shown := f.shown()
	room := max(width-1, 1)
	start, used := f.cursor, 0
	for start > 0 {
		w := lipgloss.Width(string(shown[start-1]))
		if used+w > room {
			break
		}
		used += w
		start--
	}
	return start
}

// click puts the cursor on the character that is drawn on the cell x of a field of width cells.
func (f *field) click(x, width int) {
	shown := f.shown()
	cells := 0
	for i := f.first(width); i < len(shown); i++ {
		w := lipgloss.Width(string(shown[i]))
		if x < cells+w {
			f.cursor = i
			return
		}
		cells += w
	}
	f.cursor = len(shown)
}

// view draws the field in width cells: the part of the text around the cursor, with the cursor
// drawn as selected when the field is focused. A secret field draws a "*" for each character.
func (f field) view(width int, focused bool, st styles) string {
	if width <= 0 {
		return ""
	}
	cursor := st.selected.Render(" ")
	blank := func(n int) string { return st.field.Render(strings.Repeat(" ", max(n, 0))) }
	shown := f.shown()
	if len(shown) == 0 {
		placeholder := f.placeholder
		room := width
		if focused {
			room--
		}
		if lipgloss.Width(placeholder) > room {
			placeholder = ansi.Truncate(placeholder, max(room, 0), ellipsis)
		}
		out := st.field.Inherit(st.dim).Render(placeholder) + blank(room-lipgloss.Width(placeholder))
		if focused {
			out = cursor + out
		}
		return out
	}
	var b strings.Builder
	cells := 0
	for i := f.first(width); i <= len(shown); i++ {
		ch := " "
		if i < len(shown) {
			ch = string(shown[i])
		}
		w := lipgloss.Width(ch)
		if cells+w > width {
			break
		}
		switch {
		case i == f.cursor && focused:
			b.WriteString(st.selected.Render(ch))
		case i < len(shown):
			b.WriteString(st.field.Render(ch))
		default:
			continue
		}
		cells += w
	}
	return b.String() + blank(width-cells)
}
