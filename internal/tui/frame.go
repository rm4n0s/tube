// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The smallest terminal that the TUI draws on (docs/SPEC.md §13).
const (
	minWidth  = 40
	minHeight = 10
	tooSmall  = "The terminal is too small"
	// frameLines are the lines that the frame takes: the title, two rules, the message and the
	// keys.
	frameLines = 5
)

// pageHeight is the number of lines of the page.
func (m Model) pageHeight() int { return max(m.height-frameLines, 0) }

// View draws the screen: the title, a rule, the page, a rule, the message line and the keys.
// Every line is cut to the width, and the view has exactly as many lines as the terminal.
func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	if m.width < minWidth || m.height < minHeight {
		v.SetContent(tooSmall)
		return v
	}
	lines := make([]string, 0, m.height)
	lines = append(lines, fit(m.title(), m.width), m.rule())
	body := m.body()
	if m.ask != nil {
		body = overlay(body, m.ask.q.box(m.width, m.styles), m.width, m.pageHeight())
	}
	for i := range m.pageHeight() {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		lines = append(lines, fit(line, m.width))
	}
	lines = append(lines, m.rule(), fit(m.messageLine(), m.width), fit(m.footer(), m.width))
	v.SetContent(strings.Join(lines, "\n"))
	return v
}

// title is the first line: Tube and what will happen.
func (m Model) title() string {
	what := "profiles"
	if len(m.snap.Target) > 0 {
		what = "select a profile to run " + strings.Join(m.snap.Target, " ")
	}
	return m.styles.title.Render("Tube") + "   " + what
}

// rule is a line across the screen.
func (m Model) rule() string { return m.styles.dim.Render(strings.Repeat("─", m.width)) }

// messageLine is the message or the error.
func (m Model) messageLine() string {
	if m.isError {
		return m.styles.err.Render(clean(m.message))
	}
	return clean(m.message)
}

// footer is the keys of the page, three spaces apart, or two when that does not fit.
func (m Model) footer() string {
	var parts []string
	p := m.page
	prof, ok := m.profile()
	switch {
	case m.ask != nil:
		p = pageQuestion
	case p == pageProfile && m.prof.open != noField:
		p = pageField
	}
	for _, b := range footerBindings(p, len(m.snap.Target) > 0) {
		if p == pageProfile && ok && prof.Broken != "" && b.keys != "d" && b.keys != "esc" {
			continue
		}
		parts = append(parts, m.styles.key.Render(b.keys)+" "+b.what)
	}
	if line := strings.Join(parts, "   "); lipgloss.Width(line) <= m.width {
		return line
	}
	return strings.Join(parts, "  ")
}

// body is the page's lines.
func (m Model) body() []string {
	switch {
	case m.page == pageHelp:
		return m.helpBody()
	case m.page == pageAdd && m.add != nil:
		return m.addBody()
	case m.page == pageProfile:
		return m.profileBody()
	}
	return m.profilesBody()
}
