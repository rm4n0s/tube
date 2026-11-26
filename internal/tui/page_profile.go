// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
)

// The texts and sizes of a profile's page (docs/SPEC.md §13.4).
const (
	labelWidth     = 17
	maxServerLines = 8
	maxIgnoredLine = 3
	maxUsernameLen = 128
	maxDNSLen      = 200
	dnsPlaceholder = "empty: from the VPN"
)

// fieldKind is the text field open on a profile's page.
type fieldKind int

// The fields of a profile's page.
const (
	noField fieldKind = iota
	renameField
	usernameField
	dnsField
)

// fieldLabels are the labels of the fields.
var fieldLabels = map[fieldKind]string{
	renameField:   "New name",
	usernameField: "User name",
	dnsField:      "DNS servers (IP addresses, separated by spaces or commas)",
}

// profilePage is the state of a profile's page.
type profilePage struct {
	id string
	// open is the field that is open, and field its text.
	open  fieldKind
	field field
	// busy says that the field's command runs.
	busy bool
}

// fieldDoneMsg is the end of the command of a field: its error, or nil.
type fieldDoneMsg struct{ err error }

// forgotMsg is the end of forgetting a password: its error, or nil.
type forgotMsg struct{ err error }

// forgotPassword is the notice after a password was forgotten.
const forgotPassword = "The password is forgotten: Tube asks for it the next time."

// askForget is p on the page of a profile whose password is remembered: a question, then the
// password is forgotten.
func (m Model) askForget(p app.Profile) Model {
	a, ctx := m.app, m.ctx
	m.ask = &asking{
		q: question{text: "Forget the password of \"" + p.Name + "\"? Tube will ask for it the next time.", yes: "Forget"},
		onYes: func(m Model) (Model, tea.Cmd) {
			forget := errors.Async(func() error { return a.ForgetPassword(ctx, p.ID) })
			return m, func() tea.Msg { return forgotMsg{err: forget()} }
		},
	}
	return m
}

// forgot shows how forgetting the password ended, and reads the snapshot again.
func (m Model) forgot(msg forgotMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.setError(msg.err)
	} else {
		m.setNotice(forgotPassword)
	}
	return m, m.readSnapshot()
}

// openProfile opens the page of the profile with the id.
func (m Model) openProfile(id string) Model {
	m.page = pageProfile
	m.prof = &profilePage{id: id}
	return m
}

// profile is the profile of the page, from the snapshot.
func (m Model) profile() (app.Profile, bool) {
	if m.prof == nil {
		return app.Profile{}, false
	}
	for _, p := range m.snap.Profiles {
		if p.ID == m.prof.id {
			return p, true
		}
	}
	return app.Profile{}, false
}

// closeProfile goes back to the list.
func (m Model) closeProfile() Model {
	m.page, m.prof = pageProfiles, nil
	return m
}

// profileKey handles a key on a profile's page.
func (m Model) profileKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	p, ok := m.profile()
	if !ok {
		return m.closeProfile(), nil
	}
	pp := *m.prof
	m.prof = &pp
	if pp.open != noField {
		return m.fieldKey(msg, p)
	}
	name := msg.String()
	switch name {
	case "esc":
		return m.closeProfile(), nil
	case "d":
		return m.askDelete(p), nil
	}
	if name == "enter" && len(m.snap.Target) > 0 {
		return m.startConnect(p)
	}
	if p.Broken != "" {
		return m, nil
	}
	switch name {
	case "p":
		if p.HasPassword {
			return m.askForget(p), nil
		}
	case "r":
		pp.open, pp.field = renameField, field{limit: maxNameLen}
		pp.field.set(p.Name)
	case "u":
		pp.open, pp.field = usernameField, field{limit: maxUsernameLen}
		pp.field.set(p.Username)
	case "n":
		pp.open, pp.field = dnsField, field{limit: maxDNSLen, placeholder: dnsPlaceholder}
		pp.field.set(strings.Join(p.DNS, ", "))
	case "t":
		return m.startEdit(p.ID)
	}
	return m, nil
}

// fieldKey handles a key while a field is open: enter saves, esc closes it.
func (m Model) fieldKey(msg tea.KeyPressMsg, p app.Profile) (Model, tea.Cmd) {
	pp := m.prof
	if pp.busy {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		pp.open, pp.field = noField, field{}
		return m, nil
	case "enter":
		pp.busy = true
		a, ctx, id, text, open := m.app, m.ctx, p.ID, pp.field.value(), pp.open
		save := errors.Async(func() error {
			switch open {
			case renameField:
				return a.RenameProfile(ctx, id, text)
			case usernameField:
				return a.SetUsername(ctx, id, text)
			case dnsField:
				return a.SetDNS(ctx, id, text)
			}
			return nil
		})
		return m, func() tea.Msg { return fieldDoneMsg{err: save()} }
	}
	pp.field.key(msg)
	return m, nil
}

// fieldDone closes the field after its command, or keeps it open with its text after an error.
func (m Model) fieldDone(msg fieldDoneMsg) (Model, tea.Cmd) {
	if m.prof == nil {
		return m, nil
	}
	pp := *m.prof
	m.prof = &pp
	pp.busy = false
	if msg.err != nil {
		m.setError(msg.err)
		return m, nil
	}
	pp.open, pp.field = noField, field{}
	m.setNotice(saved)
	return m, m.readSnapshot()
}

// profilePaste pastes into the open field.
func (m Model) profilePaste(text string) Model {
	if m.prof == nil || m.prof.open == noField || m.prof.busy {
		return m
	}
	pp := *m.prof
	m.prof = &pp
	pp.field.paste(text)
	return m
}

// profileBody is a profile's page: a line for each fact, then the open field. When they do not
// fit, the facts are cut so that the field stays visible.
func (m Model) profileBody() []line {
	p, ok := m.profile()
	if !ok {
		return nil
	}
	st := m.styles
	width := max(m.width-2-labelWidth, 1)
	lines := []line{{}}
	add := func(label string, style lipgloss.Style, values ...string) {
		for i, v := range values {
			if i > 0 {
				label = ""
			}
			lines = append(lines, text("  "+st.label.Render(fit(label, labelWidth))+style.Render(fit(v, width))))
		}
	}
	plain := lipgloss.NewStyle()
	add("Name", st.title, p.Name)
	if p.Broken != "" {
		add("Cannot be used", st.err, wrap(p.Broken, width)...)
		return m.withField(lines)
	}
	servers := p.Remotes
	if len(servers) > maxServerLines {
		servers = append(append([]string{}, servers[:maxServerLines]...), "… and "+strconv.Itoa(len(p.Remotes)-maxServerLines)+" more")
	}
	if len(servers) == 0 {
		servers = []string{"none"}
	}
	add("Servers", st.info, servers...)
	login, loginStyle, username := "a certificate only", plain, "not remembered"
	if p.NeedsPassword {
		login, loginStyle = "a user name and a password", st.warn
	}
	if p.Username != "" {
		username = p.Username
	}
	add("Login", loginStyle, login)
	add("User name", plain, username)
	if p.NeedsPassword {
		password, passwordStyle := "asked each time", plain
		if p.HasPassword {
			password, passwordStyle = "remembered", st.ok
		}
		add("Password", passwordStyle, password)
	}
	dns := "from the VPN"
	if len(p.DNS) > 0 {
		dns = strings.Join(p.DNS, ", ")
	}
	add("DNS servers", plain, dns)
	add("Last used", st.dim, ago(m.now, p.LastUsed))
	files := "inside the profile"
	if len(p.Files) > 0 {
		files = strings.Join(p.Files, ", ")
	}
	add("Files", plain, files)
	if len(p.Ignored) > 0 {
		ignored := wrap(strings.Join(p.Ignored, ", "), width)
		if len(ignored) > maxIgnoredLine {
			ignored = ignored[:maxIgnoredLine]
			ignored[maxIgnoredLine-1] = fit(ignored[maxIgnoredLine-1]+" …", width)
		}
		add("Ignored options", st.dim, ignored...)
	}
	return m.withField(lines)
}

// keepFocus is what a click of the right button does on a field that has the focus already.
func keepFocus(m Model, _ click) (Model, tea.Cmd) { return m, nil }

// withField adds the open field under the lines, cutting the lines when the page is too short.
// A click in the field moves its cursor.
func (m Model) withField(lines []line) []line {
	pp := m.prof
	if pp == nil || pp.open == noField {
		return lines
	}
	width := max(m.width-4, 10)
	click := func(m Model, c click) (Model, tea.Cmd) {
		if m.prof == nil || m.prof.busy {
			return m, nil
		}
		pp := *m.prof
		m.prof = &pp
		pp.field.click(c.x, width)
		return m, nil
	}
	f := []line{
		{},
		text("  " + m.styles.labelOn.Render(fieldLabels[pp.open])),
		text("  ").append(text(pp.field.view(width, !pp.busy, m.styles)).field(click, keepFocus)),
	}
	if keep := m.pageHeight() - len(f); len(lines) > keep {
		lines = lines[:max(keep, 0)]
	}
	return append(lines, f...)
}
