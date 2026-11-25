// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Selecting and copying (docs/SPEC.md §13.14): the left button, held and moved, selects the text
// of the screen from the cell where it was pressed to the cell under the mouse, as a terminal
// does; when it is let go, that text is copied to the clipboard.
const (
	// maxCopy is the number of bytes that are copied, at most: a screen has far less.
	maxCopy = 256 << 10
	// copiedByTerminal ends the sentence when the terminal was asked to copy, which it may not
	// do; copyFailed is the sentence after the name of a program that failed.
	copiedByTerminal = ", if your terminal allows it; else hold shift and select with the terminal."
	copyFailed       = " could not copy the text. Hold shift and select with your terminal."
)

// copyPrograms are the programs that take the clipboard from their standard input, each with
// the variable of the display that it needs: the first one that is found is run, without a
// shell. They are the other halves of clipboardPrograms.
var copyPrograms = []clipboardProgram{
	{"WAYLAND_DISPLAY", []string{"wl-copy", "--type", "text/plain"}},
	{"DISPLAY", []string{"xclip", "-selection", "clipboard", "-in"}},
	{"DISPLAY", []string{"xsel", "--clipboard", "--input"}},
}

type (
	// cell is a cell of the screen.
	cell struct{ x, y int }
	// selection is the text that the mouse is selecting: from the cell where the button was
	// pressed to the cell under the mouse, both among it, in the order of reading.
	selection struct{ from, to cell }
	// copiedMsg says how the program that takes the clipboard ended: it took n characters, or
	// it failed.
	copiedMsg struct {
		program string
		n       int
		ok      bool
	}
)

// ordered is the selection with its first cell, in the order of reading, in from.
func (s selection) ordered() selection {
	if s.to.y < s.from.y || (s.to.y == s.from.y && s.to.x < s.from.x) {
		s.from, s.to = s.to, s.from
	}
	return s
}

// span is the part of the line y that is selected: the cells from left to right, right not
// among them; ok is false for a line outside the selection.
func (s selection) span(y, width int) (left, right int, ok bool) {
	s = s.ordered()
	if y < s.from.y || y > s.to.y {
		return 0, 0, false
	}
	left, right = 0, width
	if y == s.from.y {
		left = s.from.x
	}
	if y == s.to.y {
		right = min(s.to.x+1, width)
	}
	return left, right, left < right
}

// onScreen is the cell of the screen that is nearest to x, y.
func (m Model) onScreen(x, y int) cell {
	return cell{x: min(max(x, 0), max(m.width-1, 0)), y: min(max(y, 0), max(m.height-1, 0))}
}

// moved is the mouse moved with a button held: after a press of the left button, the text from
// there to here is selected.
func (m Model) moved(msg tea.MouseMotionMsg) (Model, tea.Cmd) {
	if m.press == nil || m.width < minWidth || m.height < minHeight {
		return m, nil
	}
	to := m.onScreen(msg.X, msg.Y)
	if to == *m.press {
		// Back where it began: nothing is selected, and letting go is the end of a click.
		m.sel = nil
		return m, nil
	}
	m.menu, m.clip = nil, nil
	m.sel = &selection{from: *m.press, to: to}
	return m, nil
}

// released is a button let go: what was selected is copied, and is selected no more.
func (m Model) released(tea.MouseReleaseMsg) (Model, tea.Cmd) {
	sel := m.sel
	m.press, m.sel = nil, nil
	if sel == nil {
		return m, nil
	}
	text := m.selectedText(*sel)
	if strings.TrimSpace(text) == "" {
		return m, nil
	}
	return m.copyText(text)
}

// selectedText is the text of the selection: of each of its lines the cells that are selected,
// as plain text and without the spaces at its end, with a new line between two lines. It is
// what the screen draws there, so it holds nothing that the screen does not show.
func (m Model) selectedText(sel selection) string {
	lines := m.draw()
	var out []string
	for y, l := range lines {
		left, right, ok := sel.span(y, m.width)
		if !ok {
			continue
		}
		out = append(out, strings.TrimRight(ansi.Strip(ansi.Cut(fit(l.s, m.width), left, right)), " "))
	}
	text := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if len(text) > maxCopy {
		text = strings.ToValidUTF8(text[:maxCopy], "")
	}
	return text
}

// withSelection draws the selection over a line of the screen, which is as wide as the screen.
func (m Model) withSelection(s string, y int) string {
	if m.sel == nil {
		return s
	}
	left, right, ok := m.sel.span(y, m.width)
	if !ok {
		return s
	}
	return ansi.Truncate(s, left, "") + m.styles.selection.Render(ansi.Strip(ansi.Cut(s, left, right))) + ansi.TruncateLeft(s, right, "")
}

// copyText copies text to the clipboard: with the program that takes it, when there is one, and
// else by asking the terminal (OSC 52), which does not say whether it did.
func (m Model) copyText(text string) (Model, tea.Cmd) {
	n := len([]rune(text))
	argv := programOf(copyPrograms, m.getenv, m.lookPath)
	if argv == nil {
		m.setNotice(copied(n) + copiedByTerminal)
		return m, tea.SetClipboard(text)
	}
	ctx, write := m.ctx, m.writeClip
	return m, func() tea.Msg { return copiedMsg{program: argv[0], n: n, ok: write(ctx, argv, text)} }
}

// copied is the sentence about n characters that were copied, without its end.
func copied(n int) string {
	if n == 1 {
		return "Copied 1 character"
	}
	return "Copied " + strconv.Itoa(n) + " characters"
}

// copyDone shows how the copy ended.
func (m Model) copyDone(msg copiedMsg) Model {
	if !msg.ok {
		m.message, m.isError = msg.program+copyFailed, true
		return m
	}
	m.setNotice(copied(msg.n) + ".")
	return m
}

// writeClipboard runs the program that takes the clipboard, with the text on its standard
// input, and says whether it worked; it may run for clipboardTimeout. Such a program reads the
// text, and then leaves a process behind that keeps the text for the desktop until something
// else is copied: that is how a clipboard works there, and that process is the desktop's, not
// Tube's.
func writeClipboard(ctx context.Context, argv []string, text string) bool {
	ctx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	// #nosec G204 -- one of the fixed programs of copyPrograms, without a shell.
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run() == nil
}
