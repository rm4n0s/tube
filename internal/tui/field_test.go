// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// typed types text into the field, one key press for each character.
func typed(f *field, text string) {
	for _, r := range text {
		f.key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// press presses keys on the field, by their names.
func press(f *field, names ...string) {
	for _, n := range names {
		f.key(keyMsg(n))
	}
}

func TestFieldEditing(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		want   string
		cursor int
	}{
		{"typing", nil, "hello world", 11},
		{"left and insert", []string{"left", "left", "X"}, "hello worXld", 10},
		{"home and insert", []string{"home", "X"}, "Xhello world", 1},
		{"ctrl+a and ctrl+e", []string{"ctrl+a", "ctrl+e", "!"}, "hello world!", 12},
		{"end", []string{"home", "end", "!"}, "hello world!", 12},
		{"right at the end", []string{"right", "!"}, "hello world!", 12},
		{"backspace", []string{"backspace", "backspace"}, "hello wor", 9},
		{"backspace at the start", []string{"home", "backspace"}, "hello world", 0},
		{"delete", []string{"home", "delete"}, "ello world", 0},
		{"delete at the end", []string{"delete"}, "hello world", 11},
		{"ctrl+u", []string{"ctrl+u"}, "", 0},
		{"ctrl+w", []string{"ctrl+w"}, "hello ", 6},
		{"ctrl+w twice", []string{"ctrl+w", "ctrl+w"}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := field{}
			typed(&f, "hello world")
			for _, k := range tt.keys {
				if len(k) == 1 {
					typed(&f, k)
				} else {
					press(&f, k)
				}
			}
			if f.value() != tt.want || f.cursor != tt.cursor {
				t.Errorf("the field is %q at %d, want %q at %d", f.value(), f.cursor, tt.want, tt.cursor)
			}
		})
	}
	t.Run("ctrl+w in a path", func(t *testing.T) {
		f := field{}
		typed(&f, "/home/ada/x.ovpn")
		press(&f, "ctrl+w")
		if f.value() != "/home/ada/" {
			t.Errorf("the field is %q", f.value())
		}
	})
}

func TestFieldLimitAndPaste(t *testing.T) {
	f := field{limit: 5}
	typed(&f, "abcdefgh")
	if f.value() != "abcde" {
		t.Errorf("the limit let %q in", f.value())
	}
	g := field{}
	g.paste("line one\nline\ttwo\x1b[31m")
	if g.value() != "line one line two [31m" {
		t.Errorf("the paste is %q", g.value())
	}
	g.set("new\ntext")
	if g.value() != "new text" || g.cursor != 8 {
		t.Errorf("set: %q at %d", g.value(), g.cursor)
	}
	if used := g.key(keyMsg("enter")); used {
		t.Error("the field used enter")
	}
}

func TestFieldView(t *testing.T) {
	st := newStyles()
	f := field{}
	typed(&f, strings.Repeat("abcdefghij", 5))
	for _, keys := range [][]string{nil, {"home"}, {"right", "right", "right"}} {
		press(&f, keys...)
		view := f.view(12, true, st)
		if lipgloss.Width(view) != 12 {
			t.Errorf("the view is %d cells wide", lipgloss.Width(view))
		}
		cursorChar := " "
		if f.cursor < len(f.text) {
			cursorChar = string(f.text[f.cursor])
		}
		if !strings.Contains(view, st.selected.Render(cursorChar)) {
			t.Errorf("the cursor at %d is not visible: %q", f.cursor, ansi.Strip(view))
		}
	}
	empty := field{placeholder: "the file's name"}
	if got := ansi.Strip(empty.view(20, false, st)); !strings.Contains(got, "the file's name") {
		t.Errorf("the placeholder is not drawn: %q", got)
	}
}

func TestFieldSecret(t *testing.T) {
	st := newStyles()
	one, ten := field{secret: true}, field{secret: true}
	typed(&one, "x")
	typed(&ten, "hunter2!!!")
	if one.view(20, true, st) != ten.view(20, true, st) {
		t.Error("a secret field shows its length")
	}
	if got := ansi.Strip(ten.view(20, true, st)); strings.TrimSpace(got) != "" {
		t.Errorf("a secret field shows %q", got)
	}
	if ten.value() != "hunter2!!!" {
		t.Error("the secret was not kept")
	}
}
