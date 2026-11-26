// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/errors"
)

// noOutput is the Output tab of an application that printed nothing yet.
const noOutput = "No output yet."

// keyBytes are the bytes that the named keys send to the application's terminal (docs/SPEC.md
// §13.9); a key with text sends its text.
var keyBytes = map[string]string{
	"enter":     "\r",
	"backspace": "\x7f",
	"ctrl+c":    "\x03",
	"ctrl+d":    "\x04",
	"ctrl+u":    "\x15",
	"esc":       "\x1b",
}

// outputBody is the Output tab: the lines of the window, each cut to the width.
func (m Model) outputBody() []line {
	r := m.run
	if r.outEnd == r.outFirst {
		return []line{text(m.styles.dim.Render(noOutput))}
	}
	lines := make([]line, 0, len(r.lines))
	for _, l := range r.lines {
		lines = append(lines, text(fit(clean(l), m.width)))
	}
	return lines
}

// outputFooter is the keys line of the Output tab. The keys go to the application, so the line
// says so; a click on a key of the line presses it.
func (m Model) outputFooter() line {
	r := m.run
	sep := m.styles.dim.Render(" · ")
	hint := func(key, name, what string) line {
		return line{}.button(m.styles.key.Render(key)+": "+what, pressKey(name))
	}
	scroll := line{}.button(m.styles.key.Render("pgup"), pressKey("pgup")).add(" ").
		button(m.styles.key.Render("pgdn"), pressKey("pgdown")).add(": scroll")
	switch {
	case !r.follow:
		return text(m.styles.warn.Render("scrolled back")).add(sep).append(hint("end", "end", "follow"))
	case r.ended():
		return hint("q", "q", "quit").add(sep).append(hint("tab", "tab", "next tab")).add(sep).append(scroll)
	}
	return text(m.styles.accent.Render("keys go to " + r.state.AppName)).add(sep).append(hint("tab", "tab", "next tab")).add(sep).append(scroll)
}

// scrollOutput moves the window of the output by delta lines; n is the height of the window.
// Scrolling up stops following the end, and scrolling down to the end follows it again.
func (r *runScreen) scrollOutput(delta, n int) {
	if delta < 0 {
		if r.follow {
			r.hold = r.outEnd - n
		}
		r.hold = max(r.hold+delta, r.outFirst)
		r.follow = false
		return
	}
	if !r.follow {
		r.hold += delta
		r.follow = r.hold+n >= r.outEnd
	}
}

// outputKey handles a key on the Output tab: the keys that scroll, and every other key goes to
// the application while it runs. After its end, q and ctrl+c quit.
func (m Model) outputKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	r := m.run
	n := max(m.runPageHeight(), 1)
	name := msg.String()
	switch name {
	case "pgup":
		r.scrollOutput(-n, n)
		return m, m.readRun()
	case "pgdown":
		r.scrollOutput(n, n)
		return m, m.readRun()
	case "home":
		r.hold, r.follow = r.outFirst, false
		return m, m.readRun()
	case "end":
		r.follow = true
		return m, m.readRun()
	}
	if r.ended() {
		if name == "q" || name == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	text, ok := keyBytes[name]
	if !ok {
		text = msg.Text
	}
	if text == "" {
		return m, nil
	}
	return m, m.input([]byte(text))
}

// input is the command that sends bytes to the application's terminal; an error goes to the
// message line.
func (m Model) input(p []byte) tea.Cmd {
	a := m.app
	write := errors.Async(func() error { return a.Input(p) })
	return func() tea.Msg {
		if err := write(); err != nil {
			return doneMsg{err: err}
		}
		return nil
	}
}
