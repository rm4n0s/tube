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
	pageLogin
	pageConnecting
	pageRun
	// pageRunStatus and pageRunOutput are the tabs of the run screen.
	pageRunStatus
	pageRunOutput
	pageRunNetwork
	// pageQuestion, pageField and pageFolder are not pages: their keys are the ones of an open
	// question, of a field open on a profile's page, and of the list of a folder on the add page.
	pageQuestion
	pageField
	pageFolder
	// pageMenu is the open menu of a field.
	pageMenu
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
	profile := []binding{{"r", "rename"}, {"u", "user"}, {"p", "forget password"}, {"n", "DNS"}, {"t", "edit"}, {"d", "delete"}}
	if withTarget {
		enter = binding{"enter", "run"}
		profile = append(profile, enter)
	}
	profile = append(profile, binding{"esc", "back"})
	return []pageBindings{
		{"Profiles", pageProfiles, []binding{
			{"↑↓ jk", "move"}, enter, {"a", "add"}, {"e", "open"}, {"d", "delete"}, {"?", "keys"}, {"q", "quit"},
		}},
		{"Add: the folder", pageFolder, []binding{
			{"↑↓", "move"}, {"enter", "open"}, {"backspace", "folder above"}, {"tab", "fields"}, {"esc", "back"},
		}},
		{"Add: the fields", pageAdd, []binding{
			{"tab", "complete"}, {"↑↓", "field"}, {"space", "all files"}, {"enter", "add"}, {"esc", "back"},
		}},
		{"Profile", pageProfile, profile},
		{"Login", pageLogin, []binding{
			{"tab ↑↓", "field"}, {"space", "remember"}, {"enter", "connect"}, {"esc", "back"},
		}},
		{"Connecting", pageConnecting, []binding{{"esc", "cancel"}}},
		{"Status", pageRunStatus, []binding{{"tab shift+tab", "tabs"}, {"q", "quit"}, {"?", "keys"}}},
		{"Output", pageRunOutput, []binding{
			{"tab shift+tab", "tabs"}, {"pgup pgdn", "scroll"}, {"home", "oldest"}, {"end", "follow"},
			{"other keys", "go to the application"},
		}},
		{"Network", pageRunNetwork, []binding{
			{"↑↓ jk", "move"}, {"tab shift+tab", "tabs"}, {"enter", "use"}, {"x", "disconnect"}, {"q", "quit"}, {"?", "keys"},
		}},
		{"Field", pageField, []binding{{"enter", "save"}, {"esc", "cancel"}}},
		{"Menu of a field", pageMenu, []binding{{"enter", "paste"}, {"esc", "close"}}},
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

// namedKeys are the keys that the table writes with a name, and the name that Bubble Tea gives
// them.
var namedKeys = map[string]string{
	"enter": "enter", "esc": "esc", "tab": "tab", "shift+tab": "shift+tab", "space": "space",
	"backspace": "backspace", "pgup": "pgup", "pgdn": "pgdown", "home": "home", "end": "end",
}

// clickKey is the key that a click on a word of the table presses: a named key, or a key of one
// ASCII character. It is "" for a word that is no single key ("↑↓", "jk", "other keys").
func clickKey(word string) string {
	if name, ok := namedKeys[word]; ok {
		return name
	}
	if len(word) == 1 && word[0] > ' ' && word[0] < 0x7f {
		return word
	}
	return ""
}
