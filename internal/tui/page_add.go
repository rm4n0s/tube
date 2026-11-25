// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app"
)

// The texts and sizes of the add page (docs/SPEC.md §13.3).
const (
	addPathLabel       = "File"
	addNameLabel       = "Name"
	addLabelWidth      = 6
	addNamePlaceholder = "the file's name"
	addAllText         = "all files"
	addButton          = "[ Add ]"
	cancelButton       = "[ Cancel ]"
	adding             = "Adding…"
	maxPathLen         = 4096
	maxNameLen         = 64
	// maxAddErrorLines is the number of lines that an error takes on the page, at most.
	maxAddErrorLines = 3
	// addFieldLines are the lines of the two fields and of the buttons.
	addFieldLines = 3
)

// The parts of the add page that take the focus, from the top.
const (
	addFocusFolder = iota
	addFocusPath
	addFocusName
	addFocusAll
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
	}
)

// addPage is the state of the add page.
type addPage struct {
	path, name field
	// focus is the part that has the focus: addFocusFolder, ...
	focus int
	// br is the list of a folder, in which the file is chosen.
	br folder
	// err is the sentence of the last error, until the next key.
	err string
	// busy says that the add runs.
	busy bool
}

// startFolder is where the list of folders starts when no add is remembered: the home folder,
// and without one the working folder.
func (m Model) startFolder() string {
	if m.home != "" {
		return m.home
	}
	return m.cwd
}

// openAdd opens the add page, on the folder of the last add, which the app remembers between
// two runs, or on the home folder. When the remembered folder cannot be read any more, the
// home folder is shown.
func (m Model) openAdd() (Model, tea.Cmd) {
	m.page = pageAdd
	m.add = &addPage{
		path: field{limit: maxPathLen},
		name: field{limit: maxNameLen, placeholder: addNamePlaceholder},
	}
	start := m.startFolder()
	dir := m.snap.AddFolder
	if dir == "" {
		dir = start
	}
	if !m.folderShown() {
		m.add.focus = addFocusPath
	}
	m.add.br.want = dir
	read := m.readFolder(dir, false, "", false)
	return m, func() tea.Msg {
		msg := read().(folderMsg)
		msg.fallback = start
		return msg
	}
}

// addErrorLines are the lines of the error of the add page, in width cells.
func (m Model) addErrorLines(width int) []string {
	if m.add.err == "" {
		return nil
	}
	lines := wrap(m.add.err, width)
	if len(lines) > maxAddErrorLines {
		lines = lines[:maxAddErrorLines]
		lines[maxAddErrorLines-1] = fit(lines[maxAddErrorLines-1]+" …", width)
	}
	return lines
}

// folderHeight is the number of lines of the box of the folder: what the fields, the error and
// the buttons leave, or 0 when that is too little for a list.
func (m Model) folderHeight() int {
	status := max(len(m.addErrorLines(max(m.width-4, 10))), 1)
	if height := m.pageHeight() - addFieldLines - status; height >= minFolderBox {
		return height
	}
	return 0
}

// folderShown says whether the page has room for the list of the folder.
func (m Model) folderShown() bool { return m.folderHeight() > 0 }

// addStop is the part that takes the focus after the part focus (step 1), or before it (step
// -1). Without the list of the folder there are only the two fields.
func (m Model) addStop(focus, step int) int {
	stops := []int{addFocusPath, addFocusName}
	if m.folderShown() {
		stops = []int{addFocusFolder, addFocusPath, addFocusName, addFocusAll}
	}
	for i, s := range stops {
		if s == focus {
			return stops[(i+step+len(stops))%len(stops)]
		}
	}
	return stops[0]
}

// addKey handles a key on the add page.
func (m Model) addKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	if p.busy {
		return m, nil
	}
	p.err = ""
	if !m.folderShown() && (p.focus == addFocusFolder || p.focus == addFocusAll) {
		p.focus = addFocusPath
	}
	name := msg.String()
	switch name {
	case "esc":
		m.page, m.add = pageProfiles, nil
		return m, nil
	case "shift+tab":
		p.focus = m.addStop(p.focus, -1)
		return m, nil
	}
	if p.focus == addFocusFolder {
		return m.folderKey(msg)
	}
	switch {
	case name == "enter":
		return m.addSubmit()
	case name == "up":
		p.focus = m.addStop(p.focus, -1)
	case name == "down", name == "tab" && p.focus != addFocusPath:
		p.focus = m.addStop(p.focus, 1)
	case name == "tab":
		return m, m.completePath(p.path.value())
	case p.focus == addFocusAll:
		if name == "space" {
			return m.toggleAll()
		}
	case p.focus == addFocusName:
		p.name.key(msg)
	default:
		before := p.path.value()
		p.path.key(msg)
		if p.path.value() != before {
			return m.follow()
		}
	}
	return m, nil
}

// addSubmit adds the profile of the File field, with the name of the Name field.
func (m Model) addSubmit() (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	p.err, p.busy = "", true
	return m, m.addProfile(p.path.value(), p.name.value(), false)
}

// addPaste pastes into the Name field when it has the focus, and into the File field when it
// has; with the focus on the list, the pasted text is the new path.
func (m Model) addPaste(text string) (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	if p.busy {
		return m, nil
	}
	switch p.focus {
	case addFocusName:
		p.name.paste(text)
		return m, nil
	case addFocusPath:
		p.path.paste(text)
	default:
		p.focus = addFocusPath
		p.path.set(text)
	}
	return m.follow()
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
	if strings.HasSuffix(strings.ToLower(base), profileSuffix) {
		base = base[:len(base)-len(profileSuffix)]
	}
	return clean(base)
}

// completePath is the command that completes the path.
func (m Model) completePath(input string) tea.Cmd {
	home, readDir := m.home, m.readDir
	return func() tea.Msg {
		in, _ := complete(input, home, readDir)
		return completedMsg{input: in}
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
	// The next add starts in the folder of this one: the app remembers it, and the snapshot
	// that is read now has it.
	m.page, m.add = pageProfiles, nil
	m.selectID = msg.id
	m.setNotice("Added " + msg.name + ".")
	return m, m.readSnapshot()
}

// completed takes the result of a completion: the list follows the completed path.
func (m Model) completed(msg completedMsg) (Model, tea.Cmd) {
	if m.add == nil || m.add.busy {
		return m, nil
	}
	p := *m.add
	m.add = &p
	p.path.set(msg.input)
	return m.follow()
}

// clickField is the click on a field of the add page: the field gets the focus, and the cursor
// goes where the click was.
func (m Model) clickField(focus, width int) action {
	return func(m Model, c click) (Model, tea.Cmd) {
		p := *m.add
		m.add = &p
		p.err, p.focus = "", focus
		if focus == addFocusName {
			p.name.click(c.x, width)
		} else {
			p.path.click(c.x, width)
		}
		return m, nil
	}
}

// focusAdd gives a field of the add page the focus, for its menu.
func focusAdd(focus int) action {
	return func(m Model, _ click) (Model, tea.Cmd) {
		p := *m.add
		m.add = &p
		p.err, p.focus = "", focus
		return m, nil
	}
}

// addBody is the add page: the list of a folder, the File and the Name fields, the error of the
// last add, and the buttons.
func (m Model) addBody() []line {
	p := m.add
	st := m.styles
	var lines []line
	if height := m.folderHeight(); height > 0 {
		for _, l := range m.folderBox(m.width-4, height) {
			lines = append(lines, l.indent(2))
		}
	}
	width := max(m.width-4-addLabelWidth, 4)
	label := func(name string, focus int) string {
		if p.focus == focus && !p.busy {
			return "  " + st.labelOn.Render(fit(name, addLabelWidth))
		}
		return "  " + st.label.Render(fit(name, addLabelWidth))
	}
	name := p.name
	if path := p.path.value(); path != "" && !strings.HasSuffix(path, "/") {
		// What the name will be when none is given.
		name.placeholder = shownName(path, "")
	}
	lines = append(lines,
		text(label(addPathLabel, addFocusPath)).
			append(text(p.path.view(width, p.focus == addFocusPath && !p.busy, st)).field(m.clickField(addFocusPath, width), focusAdd(addFocusPath))),
		text(label(addNameLabel, addFocusName)).
			append(text(name.view(width, p.focus == addFocusName && !p.busy, st)).field(m.clickField(addFocusName, width), focusAdd(addFocusName))),
	)
	switch errLines := m.addErrorLines(max(m.width-4, 10)); {
	case p.busy:
		lines = append(lines, text("  "+st.warn.Render(adding)))
	case len(errLines) > 0:
		for _, l := range errLines {
			lines = append(lines, text("  "+st.err.Render(l)))
		}
	default:
		lines = append(lines, line{})
	}
	buttons := text("  ").button(st.buttonMain.Render(addButton), func(m Model, _ click) (Model, tea.Cmd) { return m.addSubmit() }).
		add("  ").button(st.button.Render(cancelButton), pressKey("esc"))
	if m.folderShown() {
		box := "[ ] "
		if p.br.all {
			box = "[x] "
		}
		all := box + addAllText
		if p.focus == addFocusAll && !p.busy {
			all = st.selected.Render(all)
		}
		buttons = buttons.add("    ").button(all, func(m Model, _ click) (Model, tea.Cmd) {
			p := *m.add
			m.add = &p
			p.err, p.focus = "", addFocusAll
			return m.toggleAll()
		})
	}
	lines = append(lines, buttons)
	if p.busy {
		for i := range lines {
			lines[i] = lines[i].plain()
		}
	}
	return lines
}
