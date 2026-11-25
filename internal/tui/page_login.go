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
	keepText        = "remember the password"
	needBoth        = "Give the user name and the password."
	connectButton   = "[ Connect ]"
	useButton       = "[ Use ]"
	loginLabelWidth = 11
	maxPasswordLen  = 1024
	loginFieldCount = 4
)

// The lines of the login page that take the focus.
const (
	focusUser = iota
	focusPassword
	focusRemember
	focusKeep
)

// loginPage is the state of the login page.
type loginPage struct {
	id, name   string
	user, pass field
	remember   bool
	// keep says that the password is remembered too, in a file that only the user can read.
	keep  bool
	focus int
	// back is the page that esc goes back to.
	back page
	// forSwitch says that enter switches the running application to the profile, in place of
	// connecting it.
	forSwitch bool
}

// startConnect is enter on a profile, with an application to run: the login page for a profile
// with a password that is not remembered, the connecting page for another, the reason for a
// broken one.
func (m Model) startConnect(p app.Profile) (Model, tea.Cmd) {
	if p.Broken != "" {
		m.message, m.isError = "This profile cannot be used: "+p.Broken, true
		return m, nil
	}
	if !p.NeedsPassword || p.HasPassword {
		// Without credentials the app uses the remembered ones.
		return m.connect(p.ID, p.Name, nil, app.Remember{}, nil)
	}
	return m.openLogin(p), nil
}

// openLogin opens the login page for the profile; esc goes back to the page that is open.
func (m Model) openLogin(p app.Profile) Model {
	l := &loginPage{
		id: p.ID, name: p.Name, remember: true, keep: true, back: m.page,
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
		remember := app.Remember{User: l.remember, Password: l.keep, Forget: !l.keep}
		if l.forSwitch {
			return m.switchTo(l.id, l.name, creds, remember, &l)
		}
		return m.connect(l.id, l.name, creds, remember, &l)
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
		case focusKeep:
			if msg.String() == "space" {
				l.keep = !l.keep
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
// field goes where the click was (in the password too: its stars are as many as its
// characters), and the mark of "remember" changes.
func (m Model) clickLogin(focus, width int) action {
	return func(m Model, c click) (Model, tea.Cmd) {
		l := *m.login
		m.login = &l
		l.focus = focus
		switch focus {
		case focusUser:
			l.user.click(c.x, width)
		case focusPassword:
			l.pass.click(c.x, width)
		case focusRemember:
			l.remember = !l.remember
		case focusKeep:
			l.keep = !l.keep
		}
		return m, nil
	}
}

// focusLogin gives a field of the login page the focus, for its menu.
func focusLogin(focus int) action {
	return func(m Model, _ click) (Model, tea.Cmd) {
		l := *m.login
		m.login = &l
		l.focus = focus
		return m, nil
	}
}

// loginBody is the login page: the two fields, the marks of what is remembered, and the buttons.
// On a terminal that is too short for it, the empty lines between them are left out, and then
// the first line.
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
	mark := func(on bool, what string, focus int) line {
		box := "[ ] "
		if on {
			box = "[x] "
		}
		shown := box + what
		if l.focus == focus {
			shown = st.selected.Render(shown)
		}
		return text("  ").button(shown, m.clickLogin(focus, 0))
	}
	connect := connectButton
	if l.forSwitch {
		connect = useButton
	}
	gap := line{}
	intro := text("  " + fit("The profile "+st.accent.Render(l.name)+" asks for a user name and a password.", max(m.width-4, 10)))
	lines := []line{
		gap,
		intro,
		gap,
		text(label(userLabel, focusUser)).
			append(text(l.user.view(width, l.focus == focusUser, st)).field(m.clickLogin(focusUser, width), focusLogin(focusUser))),
		text(label(passwordLabel, focusPassword)).
			append(text(l.pass.view(width, l.focus == focusPassword, st)).field(m.clickLogin(focusPassword, width), focusLogin(focusPassword))),
		gap,
		mark(l.remember, rememberText, focusRemember),
		mark(l.keep, keepText, focusKeep),
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
	if len(tight) > m.pageHeight() {
		// Without the first line the fields, the marks and the buttons fit in five lines.
		tight = tight[1:]
	}
	return tight
}
