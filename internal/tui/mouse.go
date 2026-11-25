// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The mouse (docs/SPEC.md §13.12).
const (
	// doubleClick is the longest time between the two clicks of a double click.
	doubleClick = 500 * time.Millisecond
	// doubleClickCells is how far apart, in cells of the same line, the two clicks may be.
	doubleClickCells = 2
	// wheelStep is the number of lines that one notch of the wheel scrolls.
	wheelStep = 3
)

// click is a click of the left button on a part of a line: the cell, counted from the part's
// first, and whether it is the second click of a double click.
type click struct {
	x      int
	double bool
}

// action is what a click does.
type action func(m Model, c click) (Model, tea.Cmd)

// hot is a part of a line that can be clicked: w cells from the cell x; a w below 0 is the rest
// of the line.
type hot struct {
	x, w int
	do   action
	// menu is what a click of the right button does first, before the menu of a field opens
	// (docs/SPEC.md §13.13): it gives the field the focus. Only a field has one.
	menu action
}

// line is a line of the screen and its parts that can be clicked. The pages draw lines, so what
// is clicked is always what is drawn.
type line struct {
	s    string
	w    int
	hots []hot
}

// text is a line of text that cannot be clicked. s may hold styles.
func text(s string) line { return line{s: s, w: lipgloss.Width(s)} }

// add adds text at the end of the line.
func (l line) add(s string) line {
	l.s += s
	l.w += lipgloss.Width(s)
	return l
}

// button adds text at the end of the line that does something when it is clicked.
func (l line) button(s string, do action) line {
	w := lipgloss.Width(s)
	l.hots = append(l.hots[:len(l.hots):len(l.hots)], hot{x: l.w, w: w, do: do})
	l.s += s
	l.w += w
	return l
}

// append adds another line at the end of the line, with its parts.
func (l line) append(o line) line {
	hots := l.hots[:len(l.hots):len(l.hots)]
	for _, h := range o.hots {
		h.x += l.w
		hots = append(hots, h)
	}
	l.hots = hots
	l.s += o.s
	l.w += o.w
	return l
}

// whole makes the whole line, to the edge of the screen, do something when it is clicked; the
// parts of the line come first.
func (l line) whole(do action) line {
	l.hots = append([]hot{{x: 0, w: -1, do: do}}, l.hots...)
	return l
}

// field makes the whole line a field: do is its click, and menu gives it the focus for a click
// of the right button, which opens the menu of the field.
func (l line) field(do, menu action) line {
	l.hots = append([]hot{{x: 0, w: -1, do: do, menu: menu}}, l.hots...)
	return l
}

// indent moves the line n cells to the right.
func (l line) indent(n int) line {
	return text(strings.Repeat(" ", n)).append(l)
}

// plain is the line without the parts that can be clicked.
func (l line) plain() line {
	l.hots = nil
	return l
}

// at is what a click on the cell x of the line does, or nil. The part that was added last wins.
func (l line) at(x int) (action, int) {
	for i := len(l.hots) - 1; i >= 0; i-- {
		h := l.hots[i]
		if x >= h.x && (h.w < 0 || x < h.x+h.w) {
			return h.do, x - h.x
		}
	}
	return nil, 0
}

// menuAt is what a click of the right button on the cell x of the line does first, or nil when
// no field is there.
func (l line) menuAt(x int) (action, int) {
	for i := len(l.hots) - 1; i >= 0; i-- {
		h := l.hots[i]
		if x >= h.x && (h.w < 0 || x < h.x+h.w) {
			return h.menu, x - h.x
		}
	}
	return nil, 0
}

// texts makes lines of texts.
func texts(ss ...string) []line {
	out := make([]line, len(ss))
	for i, s := range ss {
		out[i] = text(s)
	}
	return out
}

// pressKey is the action of a click that presses a key, by its name.
func pressKey(name string) action {
	return func(m Model, _ click) (Model, tea.Cmd) { return m.key(keyPress(name)) }
}

// keyPress is the key press of a key's name, as Bubble Tea names it.
func keyPress(name string) tea.KeyPressMsg {
	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	r := []rune(name)
	return tea.KeyPressMsg{Code: r[0], Text: name}
}

// lastClick is where and when the left button was clicked last.
type lastClick struct {
	x, y int
	at   time.Time
}

// clicked handles a click: the left button does what the part of the line under it says. A
// second click near the first, soon after it, is a double click.
func (m Model) clicked(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if m.width < minWidth || m.height < minHeight {
		return m, nil
	}
	if msg.Button == tea.MouseRight {
		return m.rightClicked(msg)
	}
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	m.clip = nil
	now := m.clock.Now()
	last := m.lastClick
	double := !last.at.IsZero() && now.Sub(last.at) <= doubleClick && msg.Y == last.y &&
		msg.X >= last.x-doubleClickCells && msg.X <= last.x+doubleClickCells
	m.lastClick = lastClick{x: msg.X, y: msg.Y, at: now}
	if double {
		// A third click starts again.
		m.lastClick = lastClick{}
	}
	if m.page == pageHelp && m.ask == nil {
		// Any click closes the help page, as any key does.
		return m.key(keyPress("esc"))
	}
	lines := m.draw()
	if msg.Y < 0 || msg.Y >= len(lines) || msg.X < 0 || msg.X >= m.width {
		m.menu = nil
		return m, nil
	}
	do, x := lines[msg.Y].at(msg.X)
	if do == nil {
		// A click beside an open menu closes it.
		m.menu = nil
		return m, nil
	}
	m.message, m.isError = "", false
	m, cmd := do(m, click{x: x, double: double})
	m.scroll()
	return m, cmd
}

// wheeled handles the wheel: it scrolls the list of the page, or the output.
func (m Model) wheeled(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	delta := 0
	switch msg.Button {
	case tea.MouseWheelUp:
		delta = -wheelStep
	case tea.MouseWheelDown:
		delta = wheelStep
	}
	if delta == 0 || m.ask != nil || m.menu != nil {
		return m, nil
	}
	switch {
	case m.page == pageProfiles:
		m.list.scroll(delta)
	case m.page == pageAdd && m.add != nil:
		p := *m.add
		p.br.list.scroll(delta)
		m.add = &p
	case m.page == pageRun && m.run != nil && m.run.tab == tabNetwork:
		m.list.scroll(delta)
	case m.page == pageRun && m.run != nil && m.run.tab == tabOutput:
		r := *m.run
		m.run = &r
		r.scrollOutput(delta, max(m.runPageHeight(), 1))
		return m, m.readRun()
	}
	return m, nil
}
