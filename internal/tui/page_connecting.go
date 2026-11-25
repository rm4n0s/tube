// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app"
)

// The texts of the connecting page (docs/SPEC.md §13.6).
const (
	cancelling = "Cancelling…"
	cancelled  = "Cancelled."
	escCancels = "esc cancels"
)

// connectedMsg is the end of a connect.
type connectedMsg struct{ err error }

// connecting is the state of the connecting page.
type connecting struct {
	name    string
	started time.Time
	cancel  context.CancelFunc
	// cancelling says that esc was pressed: the connect is being cancelled.
	cancelling bool
	// login is the login page that the connect came from, or nil.
	login *loginPage
	// starting says that the connect worked and the application is being started.
	starting bool
}

// connect opens the connecting page and returns the command that connects. The credentials
// live in the command until Connect returns.
func (m Model) connect(id, name string, creds *app.Credentials, remember bool, login *loginPage) (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	m.page = pageConnecting
	m.conn = &connecting{name: name, started: m.clock.Now(), cancel: cancel, login: login}
	m.now = m.conn.started
	a := m.app
	return m, func() tea.Msg {
		return connectedMsg{err: a.Connect(ctx, id, creds, remember)}
	}
}

// connectingKey handles a key on the connecting page: esc cancels.
func (m Model) connectingKey(name string) Model {
	if name != "esc" || m.conn.cancelling || m.conn.starting {
		return m
	}
	c := *m.conn
	c.cancelling = true
	c.cancel()
	m.conn = &c
	return m
}

// connected handles the end of a connect.
func (m Model) connected(msg connectedMsg) (Model, tea.Cmd) {
	c := m.conn
	if c == nil {
		return m, nil
	}
	c.cancel()
	if msg.err == nil && !c.cancelling {
		// The password lives no longer than the connect.
		s := *c
		s.starting, s.login = true, nil
		m.conn, m.login = &s, nil
		return m, m.start()
	}
	m.conn = nil
	login := c.login
	if login != nil {
		l := *login
		l.pass = field{limit: maxPasswordLen, secret: true}
		l.focus = focusPassword
		login = &l
	}
	m.login = nil
	switch {
	case c.cancelling:
		m.setNotice(cancelled)
		if login != nil {
			m.page, m.login = pageLogin, login
			return m, nil
		}
		m.page = pageProfiles
		if m.prof != nil {
			m.page = pageProfile
		}
		return m, nil
	case msg.err != nil && login != nil && app.IsAuthFailed(msg.err):
		m.page, m.login = pageLogin, login
		m.setError(msg.err)
		return m, nil
	default:
		m.page, m.prof = pageProfiles, nil
		m.setError(msg.err)
		return m, m.readSnapshot()
	}
}

// connectingBody is the connecting page.
func (m Model) connectingBody() []line {
	c := m.conn
	st := m.styles
	seconds := max(int(m.now.Sub(c.started)/time.Second), 0)
	name := st.accent.Render(c.name)
	switch {
	case c.starting:
		return []line{{}, text("  " + st.ok.Render("Connected to ") + name + st.ok.Render(". Starting the application…"))}
	case c.cancelling:
		return []line{{}, text("  " + st.warn.Render(cancelling))}
	}
	return []line{
		{},
		text("  " + st.warn.Render("Connecting to ") + name + st.warn.Render("…") + "  " + st.dim.Render(strconv.Itoa(seconds)+" s")),
		{},
		text("  ").button(st.button.Render(cancelButton), pressKey("esc")).add("  " + st.dim.Render(escCancels)),
	}
}
