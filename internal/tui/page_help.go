// SPDX-License-Identifier: GPL-2.0-only

package tui

import "strings"

// The lines of the help page about what Tube does (docs/SPEC.md §13.7, §8.7).
var helpAbout = []string{
	"Tube runs one application through a VPN, and only that application: nothing else of the machine uses it.",
	"Without the VPN the application has no network at all: Tube never lets it use the machine's.",
	"A program that hands its work to one that already runs (a second firefox, a terminal's server) is not in the VPN.",
}

// helpMouse is the line of the help page about the mouse (docs/SPEC.md §13.12).
const helpMouse = "click: select, or press the key or the button under it · double click: open, run or use · " +
	"wheel: scroll · shift and drag: select text, as your terminal does"

// helpBody is every binding of the table, page by page, then the mouse, then what Tube does.
func (m Model) helpBody() []line {
	var lines []line
	entry := func(title, rest string) {
		// One page on one line, wrapped: "Profiles — enter: run · a: add ...". The keys are
		// set apart by "·", so that the words can wrap anywhere.
		for i, l := range wrap(title+" — "+rest, m.width-2) {
			if i == 0 {
				l = m.styles.title.Render(title) + strings.TrimPrefix(l, title)
			}
			lines = append(lines, text("  "+l))
		}
	}
	for _, pb := range bindings(len(m.snap.Target) > 0) {
		parts := make([]string, 0, len(pb.bindings))
		for _, b := range pb.bindings {
			parts = append(parts, b.keys+": "+b.what)
		}
		entry(pb.title, strings.Join(parts, " · "))
	}
	entry("Mouse", helpMouse)
	lines = append(lines, line{})
	for _, about := range helpAbout {
		lines = append(lines, texts(wrap(about, m.width)...)...)
	}
	return lines
}
