// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app"
)

// The texts and sizes of the login page (docs/SPEC.md §13.6).
const (
	userLabel       = "User name"
	passwordLabel   = "Password"
	rememberText    = "remember the user name"
	needBoth        = "Give the user name and the password."
	maxPasswordLen  = 1024
	loginFieldCount = 3
)

// The lines of the login page that take the focus.
const (
	focusUser = iota
	focusPassword
	focusRemember
)

// loginPage is the state of the login page.
type loginPage struct {
	id, name   string
	user, pass field
	remember   bool
	focus      int
	// back is the page that esc goes back to.
	back page
}

// startConnect is enter on a profile, with an application to run: the login page for a profile
// with a password, the connecting page for another, the reason for a broken one.
func (m Model) startConnect(p app.Profile) (Model, tea.Cmd) {
	if p.Broken != "" {
		m.message, m.isError = "This profile cannot be used: "+p.Broken, true
		return m, nil
	}
	if !p.NeedsPassword {
		return m.connect(p.ID, p.Name, nil, false, nil)
	}
	l := &loginPage{
		id: p.ID, name: p.Name, remember: true, back: m.page,
		user: field{limit: maxUsernameLen},
		pass: field{limit: maxPasswordLen, secret: true},
	}
	l.user.set(p.Username)
	if p.Username != "" {
		l.focus = focusPassword
	}
	m.page, m.login = pageLogin, l
	return m, nil
}

// loginKey handles a key on the login page.
func (m Model) loginKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	l := *m.login
	m.login = &l
	switch msg.String() {
	case "esc":
		m.page, m.login = l.back, nil
		return m, nil
	case "tab", "down":
		l.focus = (l.focus + 1) % loginFieldCount
	case "shift+tab", "up":
		l.focus = (l.focus + loginFieldCount - 1) % loginFieldCount
	case "enter":
		if l.user.value() == "" || l.pass.value() == "" {
			m.message, m.isError = needBoth, true
			return m, nil
		}
		creds := &app.Credentials{Username: l.user.value(), Password: l.pass.value()}
		return m.connect(l.id, l.name, creds, l.remember, &l)
	default:
		switch l.focus {
		case focusUser:
			l.user.key(msg)
		case focusPassword:
			l.pass.key(msg)
		case focusRemember:
			if msg.String() == "space" {
				l.remember = !l.remember
			}
		}
	}
	return m, nil
}

// loginPaste pastes into the focused field.
func (m Model) loginPaste(text string) Model {
	l := *m.login
	m.login = &l
	switch l.focus {
	case focusUser:
		l.user.paste(text)
	case focusPassword:
		l.pass.paste(text)
	}
	return m
}

// loginBody is the login page.
func (m Model) loginBody() []string {
	l := m.login
	width := max(m.width-4, 10)
	box := "[ ]"
	if l.remember {
		box = "[x]"
	}
	remember := box + " " + rememberText
	if l.focus == focusRemember {
		remember = m.styles.selected.Render(remember)
	}
	return []string{
		"",
		"  " + fit("The profile "+l.name+" asks for a user name and a password.", width),
		"",
		"  " + m.styles.title.Render(userLabel),
		"  " + l.user.view(width, l.focus == focusUser, m.styles),
		"",
		"  " + m.styles.title.Render(passwordLabel),
		"  " + l.pass.view(width, l.focus == focusPassword, m.styles),
		"",
		"  " + remember,
	}
}
