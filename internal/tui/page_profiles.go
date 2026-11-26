// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rm4n0s/errors"
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
	// noProfilesAdd is the end of noProfiles: a click on it is the key a.
	noProfilesAdd = "a to add an .ovpn file."
)

// listHeight is the number of rows of the list: the page without the line of the column names.
func (m Model) listHeight() int { return max(m.pageHeight()-1, 1) }

// nameWidth is the width of the name column: what is left, at least 12 cells.
func (m Model) nameWidth() int {
	rest := m.width - len(selectedMark) - 3*len(columnGap) - serverWidth - passwordWidth - lastUsedWidth
	return max(rest, minNameWidth)
}

// profilesBody is the list of profiles, under the names of the columns. A click on a row selects
// it, and a double click is enter.
func (m Model) profilesBody() []line {
	if len(m.snap.Profiles) == 0 {
		if !m.loaded {
			return nil
		}
		return []line{{}, text("  "+noProfiles[:len(noProfiles)-len(noProfilesAdd)]).
			button(m.styles.key.Render(noProfilesAdd), pressKey("a"))}
	}
	lines := []line{m.profilesHead()}
	lst := m.list
	first, last := lst.window(m.listHeight())
	for i := first; i < last; i++ {
		p := m.snap.Profiles[i]
		selected := i == m.list.selected()
		row := m.profileRow(p, selected, selectedOrNot(selected), ago(m.now, p.LastUsed), m.styles.dim)
		lines = append(lines, text(row).whole(m.clickRow(i)))
	}
	return lines
}

// profilesHead is the line of the names of the columns.
func (m Model) profilesHead() line {
	head := unselectedMark + fit("Name", m.nameWidth()) + columnGap + fit("Server", serverWidth) + columnGap +
		fit("", passwordWidth) + columnGap + "Last used"
	return text(m.styles.dim.Render(head))
}

// selectedOrNot is the mark in front of a row.
func selectedOrNot(selected bool) string {
	if selected {
		return selectedMark
	}
	return unselectedMark
}

// clickRow is the click on the row i of the list of profiles: it selects the row, and a double
// click is enter.
func (m Model) clickRow(i int) action {
	return func(m Model, c click) (Model, tea.Cmd) {
		m.list.selectIndex(i)
		if c.double {
			return m.key(keyPress("enter"))
		}
		return m, nil
	}
}

// profileRow is one row, cut to the width: "> Name   server   password   last used", or the
// name and why the profile is broken. mark is its first two cells, and last its last column,
// drawn with lastStyle. The selected row has one style; the columns of another have theirs.
func (m Model) profileRow(p app.Profile, selected bool, mark, last string, lastStyle lipgloss.Style) string {
	name := fit(p.Name, m.nameWidth())
	server, password := "", ""
	if len(p.Remotes) > 0 {
		server = p.Remotes[0]
		if len(p.Remotes) > 1 {
			server += " +" + strconv.Itoa(len(p.Remotes)-1)
		}
	}
	if p.NeedsPassword {
		password = "password"
	}
	server, password = fit(server, serverWidth), fit(password, passwordWidth)
	broken := "broken: " + p.Broken
	st := m.styles
	switch {
	case selected && p.Broken != "":
		return st.selected.Render(fit(mark+name+columnGap+broken, m.width))
	case selected:
		return st.selected.Render(fit(mark+name+columnGap+server+columnGap+password+columnGap+last, m.width))
	case p.Broken != "":
		return fit(st.ok.Render(mark)+name+columnGap+st.err.Render(broken), m.width)
	}
	return fit(st.ok.Render(mark)+name+columnGap+st.info.Render(server)+columnGap+st.warn.Render(password)+columnGap+
		lastStyle.Render(last), m.width)
}

// profilesKey handles a key on the profiles page.
func (m Model) profilesKey(name string) (Model, tea.Cmd) {
	switch name {
	case "q":
		return m, tea.Quit
	case "?":
		m.page, m.helpBack = pageHelp, pageProfiles
	case "a":
		return m.openAdd()
	case "d":
		if len(m.snap.Profiles) > 0 {
			return m.askDelete(m.snap.Profiles[m.list.selected()]), nil
		}
	case "e":
		if len(m.snap.Profiles) > 0 {
			return m.openProfile(m.snap.Profiles[m.list.selected()].ID), nil
		}
	case "enter":
		if len(m.snap.Profiles) == 0 {
			break
		}
		p := m.snap.Profiles[m.list.selected()]
		if len(m.snap.Target) == 0 {
			return m.openProfile(p.ID), nil
		}
		return m.startConnect(p)
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

// askDelete asks whether to delete the profile.
func (m Model) askDelete(p app.Profile) Model {
	a, ctx := m.app, m.ctx
	m.ask = &asking{
		q: question{
			text: "Delete the profile \"" + p.Name + "\"? Its copy of the .ovpn file is removed; the file you added it from is not touched.",
			yes:  "Delete",
		},
		onYes: func(m Model) (Model, tea.Cmd) {
			remove := errors.Async(func() error { return a.DeleteProfile(ctx, p.ID) })
			return m, func() tea.Msg { return deletedMsg{name: p.Name, err: remove()} }
		},
	}
	return m
}
