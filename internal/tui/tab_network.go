// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app"
)

// inUseMark is the second cell of the row of the profile in use.
const inUseMark = "*"

// networkTop are the lines of the Network tab above the rows: what the application has, a blank
// line, and the names of the columns.
const networkTop = 3

// switchedMsg is the end of a switch.
type switchedMsg struct{ err error }

// switching is a switch that runs: the old VPN is still in use meanwhile.
type switching struct {
	id, name string
	started  time.Time
	cancel   context.CancelFunc
	// cancelling says that esc was pressed.
	cancelling bool
	// login is the login page that the switch came from, or nil.
	login *loginPage
}

// netListHeight is the number of rows of the Network tab's list.
func (m Model) netListHeight() int { return max(m.runPageHeight()-networkTop, 1) }

// selectInUse selects the profile in use on the Network tab.
func (m *Model) selectInUse() {
	for i, p := range m.snap.Profiles {
		if p.ID == m.run.state.ProfileID {
			m.list.selectIndex(i)
		}
	}
}

// networkLine is the line above the list: what the application has now, or the switch that
// runs.
func (m Model) networkLine() string {
	r := m.run
	st := r.state
	sty := m.styles
	if s := r.switching; s != nil {
		if s.cancelling {
			return sty.warn.Render(cancelling)
		}
		seconds := max(int(m.now.Sub(s.started)/time.Second), 0)
		return sty.warn.Render("Connecting to "+s.name+"… "+strconv.Itoa(seconds)+" s") + "   " + sty.dim.Render(escCancels)
	}
	switch {
	case r.ended():
		return sty.warn.Render(st.AppName + " has ended.")
	case st.State == "connected":
		return sty.ok.Render(st.AppName + " uses " + st.ProfileName + ".")
	case st.State == "reconnecting":
		return sty.warn.Render(st.AppName + " has no network: connecting again to " + st.ProfileName + "…")
	case st.State == "refused":
		return sty.err.Render(st.AppName + " has no network: the server refused the login.")
	}
	return sty.err.Render(st.AppName + " has no network: disconnected.")
}

// networkBody is the Network tab: the line of what the application has, and the profiles, the
// one in use marked. A click on a row selects it, and a double click is enter.
func (m Model) networkBody() []line {
	lines := []line{text(m.networkLine()), {}}
	if len(m.snap.Profiles) == 0 {
		return append(lines, text(noProfiles))
	}
	lines = append(lines, m.profilesHead())
	lst := m.list
	first, last := lst.window(m.netListHeight())
	for i := first; i < last; i++ {
		row := m.networkRow(m.snap.Profiles[i], i == m.list.selected())
		lines = append(lines, text(row).whole(m.clickRow(i)))
	}
	return lines
}

// networkRow is a row of the Network tab: a row of the profiles page, with "*" and "in use" (or
// "connecting again…") for the profile in use.
func (m Model) networkRow(p app.Profile, selected bool) string {
	st := m.run.state
	inUse := p.ID == st.ProfileID && !m.run.ended() && (st.State == "connected" || st.State == "reconnecting")
	mark := selectedOrNot(selected)
	if !inUse {
		return m.profileRow(p, selected, mark, ago(m.now, p.LastUsed), m.styles.dim)
	}
	mark = mark[:1] + inUseMark
	if st.State == "reconnecting" {
		return m.profileRow(p, selected, mark, "connecting again…", m.styles.warn)
	}
	return m.profileRow(p, selected, mark, "in use", m.styles.ok)
}

// networkKey handles a key on the Network tab.
func (m Model) networkKey(name string) (Model, tea.Cmd) {
	r := m.run
	switch name {
	case "q", "ctrl+c":
		return m.quitKey(name)
	case "?":
		m.page, m.helpBack = pageHelp, pageRun
		return m, nil
	case "esc":
		if s := r.switching; s != nil && !s.cancelling {
			c := *s
			c.cancelling = true
			c.cancel()
			r.switching = &c
		}
		return m, nil
	case "enter":
		return m.chooseProfile()
	case "x":
		return m.askDisconnect(), nil
	}
	m.list.key(name)
	return m, nil
}

// chooseProfile is enter on the Network tab: switch to the selected profile, through the login
// page when it needs a password.
func (m Model) chooseProfile() (Model, tea.Cmd) {
	r := m.run
	st := r.state
	switch {
	case r.ended():
		m.setNotice(st.AppName + " has ended.")
		return m, nil
	case r.switching != nil || len(m.snap.Profiles) == 0:
		return m, nil
	}
	p := m.snap.Profiles[m.list.selected()]
	switch {
	case p.Broken != "":
		m.message, m.isError = "This profile cannot be used: "+p.Broken, true
		return m, nil
	case p.ID == st.ProfileID && st.State == "connected":
		m.setNotice(st.AppName + " already uses " + p.Name + ".")
		return m, nil
	case p.NeedsPassword:
		m = m.openLogin(p)
		m.login.forSwitch = true
		return m, nil
	}
	return m.switchTo(p.ID, p.Name, nil, false, nil)
}

// switchTo starts a switch and returns its command. The credentials live in the command until
// Switch returns.
func (m Model) switchTo(id, name string, creds *app.Credentials, remember bool, login *loginPage) (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	r := *m.run
	r.switching = &switching{id: id, name: name, started: m.clock.Now(), cancel: cancel, login: login}
	m.run, m.page, m.login = &r, pageRun, nil
	m.now = r.switching.started
	a := m.app
	return m, func() tea.Msg {
		return switchedMsg{err: a.Switch(ctx, id, creds, remember)}
	}
}

// switched handles the end of a switch.
func (m Model) switched(msg switchedMsg) (Model, tea.Cmd) {
	if m.run == nil || m.run.switching == nil {
		return m, nil
	}
	r := *m.run
	s := r.switching
	s.cancel()
	r.switching = nil
	m.run = &r
	switch {
	case s.cancelling:
		m.setNotice(cancelled)
	case msg.err == nil:
		m.setNotice("Now using " + s.name + ".")
	case s.login != nil && app.IsAuthFailed(msg.err):
		l := *s.login
		l.pass = field{limit: maxPasswordLen, secret: true}
		l.focus = focusPassword
		m.page, m.login = pageLogin, &l
		m.setError(msg.err)
	default:
		m.setError(msg.err)
	}
	return m, m.readRun()
}

// askDisconnect is x on the Network tab: a question, then Disconnect.
func (m Model) askDisconnect() Model {
	r := m.run
	switch {
	case r.ended():
		m.setNotice(r.state.AppName + " has ended.")
		return m
	case r.state.State == "disconnected":
		return m
	}
	a := m.app
	m.ask = &asking{
		q: question{text: "Disconnect? " + r.state.AppName + " keeps running without network.", yes: "Disconnect"},
		onYes: func(m Model) (Model, tea.Cmd) {
			read := m.readRun()
			return m, func() tea.Msg {
				a.Disconnect()
				return read()
			}
		},
	}
	return m
}
