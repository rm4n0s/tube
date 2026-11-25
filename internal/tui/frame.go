// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The smallest terminal that the TUI draws on (docs/SPEC.md §13).
const (
	minWidth  = 40
	minHeight = 10
	tooSmall  = "The terminal is too small"
	// frameLines are the lines that the frame takes: the title, two rules, the message and the
	// keys.
	frameLines = 5
	// hintGap and hintGapNarrow are the spaces between two keys of the last line.
	hintGap       = "   "
	hintGapNarrow = "  "
)

// pageHeight is the number of lines of the page.
func (m Model) pageHeight() int { return max(m.height-frameLines, 0) }

// View draws the screen: the title, a rule, the page, a rule, the message line and the keys.
// Every line is cut to the width, and the view has exactly as many lines as the terminal. The
// mouse is on: its clicks and its wheel come as messages.
func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if m.width < minWidth || m.height < minHeight {
		v.SetContent(tooSmall)
		return v
	}
	lines := m.draw()
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = fit(l.s, m.width)
	}
	v.SetContent(strings.Join(out, "\n"))
	return v
}

// draw is the screen as lines, each with its parts that can be clicked: View draws them, and a
// click looks the cell up in them.
func (m Model) draw() []line {
	if m.page == pageRun && m.run != nil {
		return m.runView()
	}
	lines := make([]line, 0, m.height)
	lines = append(lines, m.title(), m.rule())
	lines = append(lines, m.paged(m.body(), m.pageHeight())...)
	return append(lines, m.rule(), m.messageLine(), m.footer())
}

// paged is the page in exactly height lines. Under an open question nothing of the page can be
// clicked: only the answers of the question can.
func (m Model) paged(body []line, height int) []line {
	out := make([]line, height)
	copy(out, body)
	if m.ask == nil {
		return out
	}
	for i := range out {
		out[i] = out[i].plain()
	}
	return overlay(out, m.ask.q.box(m.width, m.styles), m.width)
}

// title is the first line: Tube and what will happen.
func (m Model) title() line {
	if m.run != nil {
		return m.runHeader()
	}
	what := "profiles"
	if len(m.snap.Target) > 0 {
		what = "select a profile to run " + m.styles.accent.Render(strings.Join(m.snap.Target, " "))
	}
	return text(m.badge() + "  " + what)
}

// badge is the name of the program, as the first line starts.
func (m Model) badge() string { return m.styles.badge.Render(" Tube ") }

// rule is a line across the screen.
func (m Model) rule() line { return text(m.styles.rule.Render(strings.Repeat("─", m.width))) }

// messageLine is the message or the error.
func (m Model) messageLine() line {
	if m.isError {
		return text(m.styles.err.Render(clean(m.message)))
	}
	if m.message == "" {
		return text(m.styles.warn.Render(m.runNotice()))
	}
	return text(m.styles.ok.Render(clean(m.message)))
}

// footer is the keys of the page, three spaces apart, or two when that does not fit. A click on
// a key presses it.
func (m Model) footer() line {
	p := m.page
	prof, ok := m.profile()
	switch {
	case m.ask != nil:
		p = pageQuestion
	case p == pageProfile && m.prof.open != noField:
		p = pageField
	case p == pageAdd && m.add != nil && m.add.focus == addFocusFolder:
		p = pageFolder
	}
	var hints []line
	for _, b := range footerBindings(p, len(m.snap.Target) > 0) {
		if p == pageProfile && ok && prof.Broken != "" && b.keys != "d" && b.keys != "esc" {
			continue
		}
		if p == pageAdd && b.keys == "space" && m.add.focus != addFocusAll {
			// Space is a character in a field: it is a key only on the mark of "all files".
			continue
		}
		hints = append(hints, m.hint(b))
	}
	return joinHints(hints, m.width)
}

// hint is a binding as the last line draws it: its keys, each one a button that presses it, and
// what they do, which presses the first of them.
func (m Model) hint(b binding) line {
	var l line
	first := ""
	for i, word := range strings.Fields(b.keys) {
		if i > 0 {
			l = l.add(" ")
		}
		name := clickKey(word)
		if name == "" {
			l = l.add(m.styles.key.Render(word))
			continue
		}
		if first == "" {
			first = name
		}
		l = l.button(m.styles.key.Render(word), pressKey(name))
	}
	if first == "" {
		return l.add(" " + b.what)
	}
	return l.button(" "+b.what, pressKey(first))
}

// joinHints puts the hints on one line, three spaces apart, or two when that does not fit in
// width cells.
func joinHints(hints []line, width int) line {
	var l line
	for _, gap := range []string{hintGap, hintGapNarrow} {
		l = line{}
		for i, h := range hints {
			if i > 0 {
				l = l.add(gap)
			}
			l = l.append(h)
		}
		if l.w <= width {
			break
		}
	}
	return l
}

// body is the page's lines.
func (m Model) body() []line {
	switch {
	case m.page == pageHelp:
		return m.helpBody()
	case m.page == pageAdd && m.add != nil:
		return m.addBody()
	case m.page == pageProfile:
		return m.profileBody()
	case m.page == pageLogin && m.login != nil:
		return m.loginBody()
	case m.page == pageConnecting && m.conn != nil:
		return m.connectingBody()
	}
	return m.profilesBody()
}
