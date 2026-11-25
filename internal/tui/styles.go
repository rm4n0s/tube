// SPDX-License-Identifier: GPL-2.0-only

package tui

import "charm.land/lipgloss/v2"

// styles are the Lip Gloss styles of the TUI. Color is only an addition: what is selected is
// also bold and reversed and has a ">" in front, and an error also starts with "Error:".
type styles struct {
	title    lipgloss.Style
	selected lipgloss.Style
	dim      lipgloss.Style
	err      lipgloss.Style
	key      lipgloss.Style
}

// newStyles makes the styles.
func newStyles() styles {
	return styles{
		title:    lipgloss.NewStyle().Bold(true),
		selected: lipgloss.NewStyle().Bold(true).Reverse(true),
		dim:      lipgloss.NewStyle().Faint(true),
		err:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red),
		key:      lipgloss.NewStyle().Bold(true),
	}
}
