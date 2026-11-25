// SPDX-License-Identifier: GPL-2.0-only

package tui

// page is a page of the TUI.
type page int

// The pages.
const (
	pageProfiles page = iota
	pageHelp
	pageAdd
	pageProfile
	// pageQuestion and pageField are not pages: their keys are the ones of an open question,
	// and of a field open on a profile's page.
	pageQuestion
	pageField
)

// binding is a key, or keys, and what it does on a page.
type binding struct {
	keys, what string
}

// pageBindings are the keys of one page, under its title, for the help page.
type pageBindings struct {
	title    string
	page     page
	bindings []binding
}

// bindings is the one table of the keys: the footer and the help page are both drawn from it, so
// they cannot disagree. withTarget says whether there is an application to run.
func bindings(withTarget bool) []pageBindings {
	enter := binding{"enter", "open"}
	profile := []binding{{"r", "rename"}, {"u", "user"}, {"n", "DNS"}, {"t", "edit"}, {"d", "delete"}}
	if withTarget {
		enter = binding{"enter", "run"}
		profile = append(profile, enter)
	}
	profile = append(profile, binding{"esc", "back"})
	return []pageBindings{
		{"Profiles", pageProfiles, []binding{
			{"↑↓ jk", "move"}, enter, {"a", "add"}, {"e", "open"}, {"d", "delete"}, {"?", "keys"}, {"q", "quit"},
		}},
		{"Add", pageAdd, []binding{
			{"tab", "complete"}, {"↑↓", "field"}, {"enter", "add"}, {"esc", "back"},
		}},
		{"Profile", pageProfile, profile},
		{"Field", pageField, []binding{{"enter", "save"}, {"esc", "cancel"}}},
		{"Question", pageQuestion, []binding{{"y", "yes"}, {"n esc enter", "no"}}},
		{"Keys", pageHelp, []binding{{"any key", "close"}}},
	}
}

// footerBindings are the keys of the page that the footer shows: all but the ones that only move.
func footerBindings(p page, withTarget bool) []binding {
	for _, pb := range bindings(withTarget) {
		if pb.page != p {
			continue
		}
		var out []binding
		for _, b := range pb.bindings {
			if b.what != "move" {
				out = append(out, b)
			}
		}
		return out
	}
	return nil
}
