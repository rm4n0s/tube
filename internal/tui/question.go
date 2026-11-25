// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// question is a question with two answers; the default is no: Enter never destroys anything.
type question struct {
	text string
	// yes is the word of the answer yes, such as "Delete".
	yes string
}

// asking is the open question and the command to run on yes.
type asking struct {
	q     question
	onYes func(m Model) (Model, tea.Cmd)
}

// answer reads a key: yes for y and Y, no for n, N, esc and enter, and nothing (ok false) for
// another key.
func (q question) answer(name string) (yes, ok bool) {
	switch name {
	case "y", "Y":
		return true, true
	case "n", "N", "esc", "enter":
		return false, true
	}
	return false, false
}

// maxQuestionWidth is the width of the box of a question, at most.
const maxQuestionWidth = 64

// box draws the question as a box for a page of width cells.
func (q question) box(width int, st styles) []string {
	inner := min(maxQuestionWidth, width-4) - 4
	lines := wrap(q.text, inner)
	lines = append(lines, "", st.key.Render("[y]")+" "+q.yes+"   "+st.key.Render("[n]")+" Cancel")
	for i, l := range lines {
		lines[i] = fit(l, inner)
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(strings.Join(lines, "\n"))
	return strings.Split(box, "\n")
}

// overlay draws the box over the middle of the page's lines, which are height lines of width
// cells.
func overlay(body []string, box []string, width, height int) []string {
	out := make([]string, height)
	copy(out, body)
	top := max((height-len(box))/2, 0)
	left := max((width-lipgloss.Width(box[0]))/2, 0)
	for i, line := range box {
		if top+i < height {
			out[top+i] = strings.Repeat(" ", left) + line
		}
	}
	return out
}
