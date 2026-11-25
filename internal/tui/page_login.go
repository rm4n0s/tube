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
	connectButton   = "[ Connect ]"
	useButton       = "[ Use ]"
	loginLabelWidth = 11
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
	// forSwitch says that enter switches the running application to the profile, in place of
	// connecting it.
	forSwitch bool
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
	return m.openLogin(p), nil
}

// openLogin opens the login page for the profile; esc goes back to the page that is open.
func (m Model) openLogin(p app.Profile) Model {
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
	return m
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
		if l.forSwitch {
			return m.switchTo(l.id, l.name, creds, l.remember, &l)
		}
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

// clickLogin is the click on a line of the login page: the line gets the focus; the cursor of a
// field goes where the click was, and the mark of "remember" changes.
func (m Model) clickLogin(focus, width int) action {
	return func(m Model, c click) (Model, tea.Cmd) {
		l := *m.login
		m.login = &l
		l.focus = focus
		switch focus {
		case focusUser:
			l.user.click(c.x, width)
		case focusRemember:
			l.remember = !l.remember
		}
		return m, nil
	}
}

// loginBody is the login page: the two fields, the mark of "remember", and the buttons. The
// empty lines between them are left out on a terminal that is too short for them.
func (m Model) loginBody() []line {
	l := m.login
	st := m.styles
	width := max(m.width-4-loginLabelWidth, 4)
	label := func(name string, focus int) string {
		if l.focus == focus {
			return "  " + st.labelOn.Render(fit(name, loginLabelWidth))
		}
		return "  " + st.label.Render(fit(name, loginLabelWidth))
	}
	box := "[ ]"
	if l.remember {
		box = "[x]"
	}
	remember := box + " " + rememberText
	if l.focus == focusRemember {
		remember = st.selected.Render(remember)
	}
	connect := connectButton
	if l.forSwitch {
		connect = useButton
	}
	gap := line{}
	lines := []line{
		gap,
		text("  " + fit("The profile "+st.accent.Render(l.name)+" asks for a user name and a password.", max(m.width-4, 10))),
		gap,
		text(label(userLabel, focusUser)).
			append(text(l.user.view(width, l.focus == focusUser, st)).whole(m.clickLogin(focusUser, width))),
		text(label(passwordLabel, focusPassword)).
			append(text(l.pass.view(width, l.focus == focusPassword, st)).whole(m.clickLogin(focusPassword, width))),
		gap,
		text("  ").button(remember, m.clickLogin(focusRemember, 0)),
		gap,
		text("  ").button(st.buttonMain.Render(connect), pressKey("enter")).add("  ").button(st.button.Render(cancelButton), pressKey("esc")),
	}
	if len(lines) <= m.pageHeight() {
		return lines
	}
	var tight []line
	for _, ln := range lines {
		if ln.w > 0 {
			tight = append(tight, ln)
		}
	}
	return tight
}
