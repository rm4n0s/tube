// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	tea "charm.land/bubbletea/v2"
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
func (m Model) outputBody() []string {
	r := m.run
	if r.outEnd == r.outFirst {
		return []string{m.styles.dim.Render(noOutput)}
	}
	lines := make([]string, 0, len(r.lines))
	for _, l := range r.lines {
		lines = append(lines, fit(clean(l), m.width))
	}
	return lines
}

// outputFooter is the keys line of the Output tab.
func (m Model) outputFooter() string {
	r := m.run
	switch {
	case !r.follow:
		return "scrolled back · end: follow"
	case r.ended():
		return "q: quit · tab: next tab · pgup pgdn: scroll"
	}
	return "keys go to " + r.state.AppName + " · tab: next tab · pgup pgdn: scroll"
}

// outputKey handles a key on the Output tab: the keys that scroll, and every other key goes to
// the application while it runs. After its end, q and ctrl+c quit.
func (m Model) outputKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	r := m.run
	n := max(m.runPageHeight(), 1)
	name := msg.String()
	switch name {
	case "pgup":
		if r.follow {
			r.hold = r.outEnd - n
		}
		r.hold = max(r.hold-n, r.outFirst)
		r.follow = false
		return m, m.readRun()
	case "pgdown":
		if !r.follow {
			r.hold += n
			r.follow = r.hold+n >= r.outEnd
		}
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
	return func() tea.Msg {
		if err := a.Input(p); err != nil {
			return doneMsg{err: err}
		}
		return nil
	}
}
