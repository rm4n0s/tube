// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app"
)

// The texts of the add page (docs/SPEC.md §13.3).
const (
	addPathLabel       = "Path of the .ovpn file"
	addNameLabel       = "Name"
	addNamePlaceholder = "the file's name"
	adding             = "Adding…"
	maxPathLen         = 4096
	maxNameLen         = 64
	candidateGap       = 2
)

// The messages of the add page.
type (
	// addedMsg is the end of an add: the new profile's id and name, or the error.
	addedMsg struct {
		id, name string
		err      error
	}
	// completedMsg is the result of a completion of the path.
	completedMsg struct {
		input string
		names []string
	}
)

// addPage is the state of the add page.
type addPage struct {
	path, name field
	// focus is 0 for the path, 1 for the name.
	focus int
	// candidates are the names of a completion, until the next key.
	candidates []string
	// err is the sentence of the last error, until the next key.
	err string
	// busy says that the add runs.
	busy bool
}

// newAddPage is an empty add page.
func newAddPage() *addPage {
	return &addPage{
		path: field{limit: maxPathLen},
		name: field{limit: maxNameLen, placeholder: addNamePlaceholder},
	}
}

// openAdd opens the add page.
func (m Model) openAdd() Model {
	m.page = pageAdd
	m.add = newAddPage()
	return m
}

// addKey handles a key on the add page.
func (m Model) addKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	if p.busy {
		return m, nil
	}
	p.candidates, p.err = nil, ""
	focused := &p.path
	if p.focus == 1 {
		focused = &p.name
	}
	switch msg.String() {
	case "esc":
		m.page, m.add = pageProfiles, nil
		return m, nil
	case "enter":
		p.busy = true
		return m, m.addProfile(p.path.value(), p.name.value(), false)
	case "tab":
		if p.focus == 0 {
			return m, m.completePath(p.path.value())
		}
		p.focus = 0
	case "up", "down", "shift+tab":
		p.focus = 1 - p.focus
	default:
		focused.key(msg)
	}
	return m, nil
}

// addPaste pastes into the focused field.
func (m Model) addPaste(text string) Model {
	p := *m.add
	m.add = &p
	if p.focus == 0 {
		p.path.paste(text)
	} else {
		p.name.paste(text)
	}
	return m
}

// addProfile is the command that adds the profile.
func (m Model) addProfile(path, name string, withoutScripts bool) tea.Cmd {
	a, ctx := m.app, m.ctx
	return func() tea.Msg {
		id, err := a.AddProfile(ctx, path, name, withoutScripts)
		return addedMsg{id: id, name: shownName(path, name), err: err}
	}
}

// shownName is the name of a new profile as the message says it: the name given, or the file's
// name without ".ovpn", as the app makes it.
func shownName(path, name string) string {
	if name != "" {
		return name
	}
	base := filepath.Base(path)
	if strings.HasSuffix(strings.ToLower(base), ".ovpn") {
		base = base[:len(base)-len(".ovpn")]
	}
	return clean(base)
}

// completePath is the command that completes the path.
func (m Model) completePath(input string) tea.Cmd {
	home, readDir := m.home, m.readDir
	return func() tea.Msg {
		in, names := complete(input, home, readDir)
		return completedMsg{input: in, names: names}
	}
}

// added handles the end of an add.
func (m Model) added(msg addedMsg) (Model, tea.Cmd) {
	if m.add == nil {
		return m, nil
	}
	p := *m.add
	m.add = &p
	p.busy = false
	if options, ok := app.IsRunsScripts(msg.err); ok {
		path, name := p.path.value(), p.name.value()
		m.ask = &asking{
			q: question{
				text: "This profile runs scripts (" + options + "). Tube never runs them, and sets the DNS of the application itself. Add it without them?",
				yes:  "Add without them",
			},
			onYes: func(m Model) (Model, tea.Cmd) {
				if m.add != nil {
					p := *m.add
					p.busy = true
					m.add = &p
				}
				return m, m.addProfile(path, name, true)
			},
		}
		return m, nil
	}
	if msg.err != nil {
		p.err = clean(app.Explain(msg.err))
		return m, nil
	}
	m.page, m.add = pageProfiles, nil
	m.selectID = msg.id
	m.setNotice("Added " + msg.name + ".")
	return m, m.readSnapshot()
}

// completed takes the result of a completion.
func (m Model) completed(msg completedMsg) Model {
	if m.add == nil {
		return m
	}
	p := *m.add
	m.add = &p
	p.path.set(msg.input)
	p.candidates = msg.names
	return m
}

// addBody is the add page.
func (m Model) addBody() []string {
	p := m.add
	width := max(m.width-4, 10)
	lines := []string{
		"",
		"  " + m.styles.title.Render(addPathLabel),
		"  " + p.path.view(width, p.focus == 0 && !p.busy, m.styles),
		"",
		"  " + m.styles.title.Render(addNameLabel),
		"  " + p.name.view(width, p.focus == 1 && !p.busy, m.styles),
		"",
	}
	switch {
	case p.busy:
		lines = append(lines, "  "+adding)
	case p.err != "":
		for _, l := range wrap(p.err, width) {
			lines = append(lines, "  "+m.styles.err.Render(l))
		}
	case len(p.candidates) > 0:
		lines = append(lines, columns(p.candidates, width, "  ")...)
	}
	return lines
}

// columns draws names in columns across width cells.
func columns(names []string, width int, indent string) []string {
	colWidth := 0
	for _, n := range names {
		colWidth = max(colWidth, len([]rune(n))+candidateGap)
	}
	perLine := max(width/max(colWidth, 1), 1)
	var lines []string
	for i := 0; i < len(names); i += perLine {
		var b strings.Builder
		for _, n := range names[i:min(i+perLine, len(names))] {
			b.WriteString(fit(n, colWidth))
		}
		lines = append(lines, indent+strings.TrimRight(b.String(), " "))
	}
	return lines
}

// defaultReadDir reads a folder, for the completion.
func defaultReadDir(dir string) ([]os.DirEntry, error) { return os.ReadDir(dir) }
