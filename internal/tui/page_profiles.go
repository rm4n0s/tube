// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app"
)

// The widths of the columns of the profiles page (docs/SPEC.md §13.2).
const (
	minNameWidth   = 12
	serverWidth    = 34
	passwordWidth  = 8
	lastUsedWidth  = 12
	columnGap      = "  "
	selectedMark   = "> "
	unselectedMark = "  "
	noProfiles     = "No profiles yet. Press a to add an .ovpn file."
)

// listHeight is the number of rows of the list: the page without the line of the column names.
func (m Model) listHeight() int { return max(m.pageHeight()-1, 1) }

// nameWidth is the width of the name column: what is left, at least 12 cells.
func (m Model) nameWidth() int {
	rest := m.width - len(selectedMark) - 3*len(columnGap) - serverWidth - passwordWidth - lastUsedWidth
	return max(rest, minNameWidth)
}

// profilesBody is the list of profiles, under the names of the columns.
func (m Model) profilesBody() []string {
	if len(m.snap.Profiles) == 0 {
		if !m.loaded {
			return nil
		}
		return []string{"", "  " + noProfiles}
	}
	head := unselectedMark + fit("Name", m.nameWidth()) + columnGap + fit("Server", serverWidth) + columnGap +
		fit("", passwordWidth) + columnGap + "Last used"
	lines := []string{m.styles.dim.Render(head)}
	lst := m.list
	first, last := lst.window(m.listHeight())
	for i := first; i < last; i++ {
		row := m.profileRow(m.snap.Profiles[i], i == m.list.selected())
		lines = append(lines, row)
	}
	return lines
}

// profileRow is one row: "> Name   server   password   last used", or the name and why the
// profile is broken.
func (m Model) profileRow(p app.Profile, selected bool) string {
	mark := unselectedMark
	if selected {
		mark = selectedMark
	}
	var row string
	if p.Broken != "" {
		row = mark + fit(p.Name, m.nameWidth()) + columnGap + "broken: " + p.Broken
	} else {
		server := ""
		if len(p.Remotes) > 0 {
			server = p.Remotes[0]
			if len(p.Remotes) > 1 {
				server += " +" + strconv.Itoa(len(p.Remotes)-1)
			}
		}
		password := ""
		if p.NeedsPassword {
			password = "password"
		}
		row = mark + fit(p.Name, m.nameWidth()) + columnGap + fit(server, serverWidth) + columnGap +
			fit(password, passwordWidth) + columnGap + fit(ago(m.now, p.LastUsed), lastUsedWidth)
	}
	row = fit(row, m.width)
	if selected {
		return m.styles.selected.Render(row)
	}
	return row
}

// profilesKey handles a key on the profiles page.
func (m Model) profilesKey(name string) (Model, tea.Cmd) {
	switch name {
	case "q":
		return m, tea.Quit
	case "?":
		m.page = pageHelp
	case "a":
		return m.openAdd(), nil
	case "d":
		return m.askDelete(), nil
	case "enter", "e":
		// TODO(step 5.4, 5.5): the profile's page, connecting.
		m.setNotice(notBuilt)
	default:
		m.list.key(name)
	}
	return m, nil
}

// deletedMsg is the end of a delete: the profile's name, or the error.
type deletedMsg struct {
	name string
	err  error
}

// askDelete asks whether to delete the selected profile.
func (m Model) askDelete() Model {
	if len(m.snap.Profiles) == 0 {
		return m
	}
	p := m.snap.Profiles[m.list.selected()]
	a, ctx := m.app, m.ctx
	m.ask = &asking{
		q: question{
			text: "Delete the profile \"" + p.Name + "\"? Its copy of the .ovpn file is removed; the file you added it from is not touched.",
			yes:  "Delete",
		},
		onYes: func(m Model) (Model, tea.Cmd) {
			return m, func() tea.Msg {
				return deletedMsg{name: p.Name, err: a.DeleteProfile(ctx, p.ID)}
			}
		},
	}
	return m
}
