// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The menu of a field (docs/SPEC.md §13.13): a right click on a field reads the clipboard, and
// when it has text, a menu with "Paste" opens under the click.
const (
	pasteItem = "Paste"
	// clipboardTimeout is how long the program that prints the clipboard may run, and
	// clipboardWait how long the terminal may take to answer.
	clipboardTimeout = 2 * time.Second
	clipboardWait    = 2 * time.Second
	// maxClipboard is the number of bytes of the clipboard that are read, at most.
	maxClipboard    = 64 << 10
	noClipboardText = "The clipboard has no text."
	noClipboard     = "The clipboard cannot be read here. Paste with your terminal (often ctrl+shift+v)."
)

// clipboardPrograms are the programs that print the clipboard, each with the variable of the
// display that it needs: the first one that is found is run, without a shell.
var clipboardPrograms = []struct {
	env  string
	argv []string
}{
	{"WAYLAND_DISPLAY", []string{"wl-paste", "--no-newline", "--type", "text"}},
	{"DISPLAY", []string{"xclip", "-selection", "clipboard", "-out"}},
	{"DISPLAY", []string{"xsel", "--clipboard", "--output"}},
}

// The messages and the states of the menu.
type (
	// clipboardMsg is what the program that prints the clipboard printed.
	clipboardMsg struct{ text string }
	// clipWait is a right click that waits for the clipboard: where it was, and since when.
	clipWait struct {
		x, y  int
		since time.Time
	}
	// menu is the open menu: the cell of the click, and the text of the clipboard, which is
	// never drawn (it may be a password) and is forgotten when the menu closes.
	menu struct {
		x, y int
		text string
	}
)

// clipboardCommand is the program that prints the clipboard, with its arguments, or nil when
// there is no display or none of the programs is installed.
func clipboardCommand(getenv func(string) string, lookPath func(string) (string, error)) []string {
	for _, p := range clipboardPrograms {
		if getenv(p.env) == "" {
			continue
		}
		if _, err := lookPath(p.argv[0]); err == nil {
			return p.argv
		}
	}
	return nil
}

// limitedBuffer keeps the first bytes that are written to it, up to its limit, and drops the
// rest.
type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

// Write keeps what fits and says that it took everything, so that the program is not stopped
// by a pipe that is full.
func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// runClipboard runs the program that prints the clipboard and returns what it printed, at most
// maxClipboard bytes. A program that fails, or takes longer than clipboardTimeout, printed
// nothing: an empty clipboard and a program that cannot read it look the same.
func runClipboard(ctx context.Context, argv []string) string {
	ctx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	out := &limitedBuffer{limit: maxClipboard}
	// #nosec G204 -- one of the fixed programs of clipboardPrograms, without a shell.
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return out.buf.String()
}

// askClipboard is a right click on a field, at the cell x, y: the clipboard is read, by the
// program that prints it when there is one, and else by asking the terminal (OSC 52), which
// answers with a tea.ClipboardMsg when it allows that.
func (m Model) askClipboard(x, y int) (Model, tea.Cmd) {
	m.menu = nil
	m.clip = &clipWait{x: x, y: y, since: m.clock.Now()}
	argv := clipboardCommand(m.getenv, m.lookPath)
	if argv == nil {
		return m, tea.ReadClipboard
	}
	ctx, read := m.ctx, m.readClip
	return m, func() tea.Msg { return clipboardMsg{text: read(ctx, argv)} }
}

// clipboardRead takes the text of the clipboard, when a right click waits for it: with text,
// the menu opens where the click was.
func (m Model) clipboardRead(text string) Model {
	if m.clip == nil {
		return m
	}
	c := m.clip
	m.clip = nil
	if len(text) > maxClipboard {
		text = text[:maxClipboard]
	}
	text = strings.TrimRight(text, "\r\n")
	if strings.TrimSpace(clean(text)) == "" {
		m.setNotice(noClipboardText)
		return m
	}
	m.menu = &menu{x: c.x, y: c.y, text: text}
	return m
}

// clipboardLate gives up a right click whose clipboard did not come: the terminal does not
// answer.
func (m *Model) clipboardLate() {
	if m.clip != nil && m.clock.Now().Sub(m.clip.since) >= clipboardWait {
		m.clip = nil
		m.setNotice(noClipboard)
	}
}

// pasted pastes text into the field that has the focus, or, on the Output tab, sends it to the
// application.
func (m Model) pasted(text string) (Model, tea.Cmd) {
	switch {
	case m.ask != nil:
	case m.page == pageAdd && m.add != nil:
		return m.addPaste(text)
	case m.page == pageProfile:
		return m.profilePaste(text), nil
	case m.page == pageLogin && m.login != nil:
		return m.loginPaste(text), nil
	case m.page == pageRun && m.run != nil && m.run.tab == tabOutput && !m.run.ended():
		return m, m.input([]byte(text))
	}
	return m, nil
}

// pasteMenu is the click on "Paste": the menu closes and its text is pasted.
func pasteMenu(m Model, _ click) (Model, tea.Cmd) {
	if m.menu == nil {
		return m, nil
	}
	text := m.menu.text
	m.menu = nil
	return m.pasted(text)
}

// rightClicked is a click of the right button: on a field, the field gets the focus and the
// clipboard is read for its menu. Anywhere, an open menu closes.
func (m Model) rightClicked(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	m.menu, m.clip = nil, nil
	lines := m.draw()
	if m.ask != nil || msg.Y < 0 || msg.Y >= len(lines) || msg.X < 0 || msg.X >= m.width {
		return m, nil
	}
	do, x := lines[msg.Y].menuAt(msg.X)
	if do == nil {
		return m, nil
	}
	m.message, m.isError = "", false
	m, _ = do(m, click{x: x})
	return m.askClipboard(msg.X, msg.Y)
}

// withMenu draws the open menu over the lines of the screen, under the cell of the click, or
// above it when there is no room under. Under the menu only "Paste" and the keys of the last
// line can be clicked.
func (m Model) withMenu(lines []line) []line {
	st := m.styles
	edge := st.borderOn.Render
	item := " " + pasteItem + " "
	box := []line{
		text(edge("╭" + strings.Repeat("─", len(item)) + "╮")),
		text(edge("│")).button(st.selected.Render(item), pasteMenu).add(edge("│")),
		text(edge("╰" + strings.Repeat("─", len(item)) + "╯")),
	}
	width := box[0].w
	left := max(min(m.menu.x, m.width-width), 0)
	top := m.menu.y + 1
	if top+len(box) > len(lines) {
		top = m.menu.y - len(box)
	}
	top = max(top, 0)
	out := make([]line, len(lines))
	for i, l := range lines {
		if i < len(lines)-1 {
			l = l.plain()
		}
		if i >= top && i < top+len(box) {
			before := ansi.Truncate(l.s, left, "")
			before += strings.Repeat(" ", max(left-ansi.StringWidth(before), 0))
			l = text(before).append(box[i-top]).add(ansi.TruncateLeft(l.s, left+width, ""))
		}
		out[i] = l
	}
	return out
}
