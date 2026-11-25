// SPDX-License-Identifier: GPL-2.0-only

package tui

import "strings"

// The lines of the help page about what Tube does (docs/SPEC.md §13.7, §8.7).
var helpAbout = []string{
	"Tube runs one application through a VPN, and only that application: nothing else of the machine uses it.",
	"Without the VPN the application has no network at all: Tube never lets it use the machine's.",
	"A program that hands its work to one that already runs (a second firefox, a terminal's server) is not in the VPN.",
}

// helpBody is every binding of the table, page by page, then what Tube does.
func (m Model) helpBody() []string {
	var lines []string
	for _, pb := range bindings(len(m.snap.Target) > 0) {
		// One page on one line, wrapped: "Profiles  enter run   a add ...". The keys are
		// set apart by "·", so that the words can wrap anywhere.
		parts := make([]string, 0, len(pb.bindings))
		for _, b := range pb.bindings {
			parts = append(parts, b.keys+": "+b.what)
		}
		for i, l := range wrap(pb.title+" — "+strings.Join(parts, " · "), m.width-2) {
			if i == 0 {
				l = m.styles.title.Render(pb.title) + strings.TrimPrefix(l, pb.title)
			}
			lines = append(lines, "  "+l)
		}
	}
	lines = append(lines, "")
	for _, about := range helpAbout {
		lines = append(lines, wrap(about, m.width)...)
	}
	return lines
}
