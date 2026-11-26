// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
)

// The messages of an edit of a profile's text (docs/SPEC.md §13.5).
type (
	// editBeganMsg is the end of BeginEdit: the file to edit, or the error.
	editBeganMsg struct {
		id, path string
		err      error
	}
	// editorDoneMsg says that the editor ended; err is why it could not run.
	editorDoneMsg struct {
		id, path string
		err      error
	}
	// editEndedMsg is the end of EndEdit, and why the editor could not run.
	editEndedMsg struct {
		changed   bool
		err       error
		editorErr error
	}
)

// The texts of an edit.
const (
	noEditor     = "No editor found. Set EDITOR."
	saved        = "Saved."
	nothingNew   = "Nothing changed."
	notChangedBy = "The profile was not changed: "
)

// fallbackEditors are the editors tried when neither VISUAL nor EDITOR names one.
var fallbackEditors = []string{"nano", "vi"}

// findEditor chooses the editor: $VISUAL, else $EDITOR, else the first of nano and vi that
// lookPath finds. The value of a variable is split at spaces into the program and its
// arguments; it is never given to a shell. ok is false when there is none.
func findEditor(getenv func(string) string, lookPath func(string) (string, error)) (argv []string, ok bool) {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		fields := strings.Fields(getenv(name))
		if len(fields) == 0 {
			continue
		}
		if _, err := lookPath(fields[0]); err == nil {
			return fields, true
		}
	}
	for _, name := range fallbackEditors {
		if _, err := lookPath(name); err == nil {
			return []string{name}, true
		}
	}
	return nil, false
}

// execEditor gives the terminal to the editor on the file, and answers editorDoneMsg when it
// ends.
func execEditor(argv []string, id, path string) tea.Cmd {
	args := append(append([]string{}, argv[1:]...), path)
	// #nosec G204 -- the user's own editor, from the user's VISUAL or EDITOR, without a shell.
	cmd := exec.Command(argv[0], args...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{id: id, path: path, err: err}
	})
}

// startEdit is t on a profile's page: BeginEdit, when there is an editor.
func (m Model) startEdit(id string) (Model, tea.Cmd) {
	argv, ok := findEditor(m.getenv, m.lookPath)
	if !ok {
		m.setNotice(noEditor)
		return m, nil
	}
	m.editor = argv
	a, ctx := m.app, m.ctx
	var path string
	begin := errors.Async(func() error {
		var err error
		path, err = a.BeginEdit(ctx, id)
		return err
	})
	return m, func() tea.Msg {
		err := begin()
		return editBeganMsg{id: id, path: path, err: err}
	}
}

// editBegan runs the editor on the file of the edit.
func (m Model) editBegan(msg editBeganMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.setError(msg.err)
		return m, nil
	}
	return m, m.execEditor(m.editor, msg.id, msg.path)
}

// editorDone ends the edit: EndEdit is called even when the editor failed to start, since it
// removes the file.
func (m Model) editorDone(msg editorDoneMsg) (Model, tea.Cmd) {
	a, ctx := m.app, m.ctx
	var changed bool
	end := errors.Async(func() error {
		var err error
		changed, err = a.EndEdit(ctx, msg.id, msg.path)
		return err
	})
	return m, func() tea.Msg {
		err := end()
		return editEndedMsg{changed: changed, err: err, editorErr: msg.err}
	}
}

// editEnded shows how the edit ended. A text that was saved counts, even when the editor then
// ended with an error.
func (m Model) editEnded(msg editEndedMsg) (Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.message, m.isError = notChangedBy+app.Explain(msg.err), true
	case msg.changed:
		m.setNotice(saved)
	case msg.editorErr != nil:
		m.message, m.isError = notChangedBy+"the editor failed: "+app.Explain(msg.editorErr), true
	default:
		m.setNotice(nothingNew)
	}
	return m, m.readSnapshot()
}
