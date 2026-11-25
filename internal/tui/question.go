// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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

// box draws the question as a box for a page of width cells; a click on an answer gives it.
func (q question) box(width int, st styles) []line {
	inner := min(maxQuestionWidth, width-4) - 4
	edge := st.ask.Render
	lines := []line{text(edge("╭" + strings.Repeat("─", inner+2) + "╮"))}
	row := func(l line) line {
		pad := strings.Repeat(" ", max(inner-l.w, 0))
		return text(edge("│") + " ").append(l).add(pad + " " + edge("│"))
	}
	for _, l := range wrap(q.text, inner) {
		lines = append(lines, row(text(fit(l, inner))))
	}
	answers := line{}.button(st.buttonYes.Render("[y] "+q.yes), pressKey("y")).add("   ").
		button(st.button.Render("[n] Cancel"), pressKey("n"))
	if answers.w > inner {
		answers = text(fit(answers.s, inner))
	}
	lines = append(lines, row(line{}), row(answers))
	return append(lines, text(edge("╰"+strings.Repeat("─", inner+2)+"╯")))
}

// overlay draws the box over the middle of the page's lines, which are width cells wide.
func overlay(body []line, box []line, width int) []line {
	out := make([]line, len(body))
	copy(out, body)
	top := max((len(body)-len(box))/2, 0)
	left := max((width-box[0].w)/2, 0)
	for i, l := range box {
		if top+i < len(out) {
			out[top+i] = l.indent(left)
		}
	}
	return out
}
