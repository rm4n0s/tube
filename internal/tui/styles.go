// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// styles are the Lip Gloss styles of the TUI (docs/SPEC.md §13.1). Color is only an addition:
// what is selected is also bold and has a ">" in front, the current tab is in brackets, an error
// starts with "Error:", and a button is in brackets. Without colors (NO_COLOR, a terminal that
// has none) what is selected is reversed.
type styles struct {
	// title is a heading; badge is the name "Tube" on the first line.
	title, badge lipgloss.Style
	// selected is the selected row, the cursor and the current tab; selectedOff is the selected
	// row of a list that does not have the focus.
	selected, selectedOff lipgloss.Style
	// dim is what matters less: the names of the columns, a placeholder, a time.
	dim lipgloss.Style
	// rule is a line across the screen; border and borderOn are the box of the folders, without
	// and with the focus; ask is the box of a question.
	rule, border, borderOn, ask lipgloss.Style
	// err is an error, warn what needs attention, ok what is well, info a fact.
	err, warn, ok, info lipgloss.Style
	// key is a key in the list of keys; accent is the application and the profile in a sentence.
	key, accent lipgloss.Style
	// label is the label of a fact or of a field; labelOn the one of the field with the focus.
	label, labelOn lipgloss.Style
	// field is the text of a field.
	field lipgloss.Style
	// folder and profile are a folder and an .ovpn file in the list of a folder.
	folder, profile lipgloss.Style
	// button is a button; buttonMain the one that enter presses; buttonYes the answer yes of a
	// question, which enter never gives.
	button, buttonMain, buttonYes lipgloss.Style
	// tab is a tab that is not the current one.
	tab lipgloss.Style
	// down and up are the traffic that was received and sent.
	down, up lipgloss.Style
}

// newStyles makes the styles for a dark or a light terminal, with or without colors. The colors
// are given as 24 bits; Bubble Tea brings them down to what the terminal has.
func newStyles(dark, colors bool) styles {
	pick := lipgloss.LightDark(dark)
	c := func(light, dark string) color.Color { return pick(lipgloss.Color(light), lipgloss.Color(dark)) }
	var (
		violet  = c("#6D3FD6", "#B48EFF")
		onColor = c("#FFFFFF", "#1B1033")
		cyan    = c("#0A7E8C", "#56D4E0")
		green   = c("#1A7F37", "#7BD88F")
		yellow  = c("#9A6700", "#F5C768")
		red     = c("#CF222E", "#FF6B7F")
		blue    = c("#0969DA", "#7AA2F7")
		gray    = c("#6E7781", "#8389A3")
		faint   = c("#B8BCC8", "#4A4E69")
		surface = c("#E9E9F2", "#2A2B3D")
	)
	s := lipgloss.NewStyle
	st := styles{
		title:       s().Bold(true).Foreground(violet),
		badge:       s().Bold(true).Foreground(onColor).Background(violet),
		selected:    s().Bold(true).Foreground(onColor).Background(violet),
		selectedOff: s().Bold(true).Foreground(violet),
		dim:         s().Foreground(gray),
		rule:        s().Foreground(faint),
		border:      s().Foreground(faint),
		borderOn:    s().Foreground(violet),
		ask:         s().Foreground(yellow),
		err:         s().Bold(true).Foreground(red),
		warn:        s().Foreground(yellow),
		ok:          s().Foreground(green),
		info:        s().Foreground(blue),
		key:         s().Bold(true).Foreground(cyan),
		accent:      s().Bold(true).Foreground(cyan),
		label:       s().Foreground(gray),
		labelOn:     s().Bold(true).Foreground(violet),
		field:       s().Background(surface),
		folder:      s().Bold(true).Foreground(blue),
		profile:     s().Foreground(green),
		button:      s().Bold(true).Foreground(cyan).Background(surface),
		buttonMain:  s().Bold(true).Foreground(onColor).Background(green),
		buttonYes:   s().Bold(true).Foreground(onColor).Background(yellow),
		tab:         s().Foreground(gray),
		down:        s().Foreground(green),
		up:          s().Foreground(blue),
	}
	if !colors {
		// The terminal draws no color: what is selected must be seen without one.
		st.badge = s().Bold(true)
		st.selected = s().Bold(true).Reverse(true)
		st.selectedOff = s().Bold(true)
		st.dim = s().Faint(true)
		st.rule, st.border = s().Faint(true), s().Faint(true)
		st.button, st.buttonMain, st.buttonYes = s().Bold(true), s().Bold(true).Reverse(true), s().Bold(true)
	}
	return st
}
