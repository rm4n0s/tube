// SPDX-License-Identifier: GPL-2.0-only

// Package tui is Tube's terminal UI (docs/SPEC.md §13), on Bubble Tea. One model holds plain
// data: the snapshot of the app, the size, the page and each page's own state. View only draws
// what the model holds; a key that changes something returns a command that calls the app and
// answers with a message.
package tui

import (
	"context"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/clock"
)

// The messages of the model.
type (
	// snapshotMsg is the snapshot that was read, or the error.
	snapshotMsg struct {
		snap app.Snapshot
		err  error
	}
	// changedMsg says that the app changed: the snapshot is read again.
	changedMsg struct{}
	// tickMsg comes every second, from Run.
	tickMsg struct{}
	// doneMsg is the end of a command of the app: its error, or nil.
	doneMsg struct{ err error }
)

// Result is what the TUI did.
type Result struct {
	// Started says that the application was started (App.Start worked): cmd/tube then calls
	// App.Finish for its exit status.
	Started bool
}

// Model is the TUI's model.
type Model struct {
	ctx    context.Context
	app    app.Application
	clock  clock.Clock
	styles styles

	width, height int
	snap          app.Snapshot
	loaded        bool
	now           time.Time

	page page
	list list
	// selectID is a profile to select when the next snapshot comes (a new one).
	selectID string
	// add is the add page's state, while it is open.
	add *addPage
	// prof is the state of a profile's page, while it is open.
	prof *profilePage
	// login and conn are the states of the login and the connecting pages.
	login *loginPage
	conn  *connecting
	// run is the state of the run screen, once the application was started.
	run *runScreen
	// helpBack is the page that the help page goes back to.
	helpBack page
	// ask is the open question; it gets every key.
	ask *asking
	// message is the line above the keys; isError draws it as an error.
	message string
	isError bool

	// home and readDir are for the completion of paths.
	home    string
	readDir func(dir string) ([]os.DirEntry, error)

	// getenv and lookPath find the editor; execEditor runs it (the tests replace it). editor is
	// the editor of the edit that runs.
	getenv     func(string) string
	lookPath   func(string) (string, error)
	execEditor func(argv []string, id, path string) tea.Cmd
	editor     []string

	result Result
}

// NewModel makes the model of the TUI over the app.
func NewModel(ctx context.Context, a app.Application, clk clock.Clock) Model {
	if clk == nil {
		clk = clock.System()
	}
	// Without a home folder, "~" is not completed; nothing else needs it.
	home, _ := os.UserHomeDir()
	return Model{
		ctx: ctx, app: a, clock: clk, styles: newStyles(), page: pageProfiles, home: home, readDir: defaultReadDir,
		getenv: os.Getenv, lookPath: exec.LookPath, execEditor: execEditor,
	}
}

// Result is what the user chose.
func (m Model) Result() Result { return m.result }

// Init reads the snapshot.
func (m Model) Init() tea.Cmd { return m.readSnapshot() }

// readSnapshot is the command that reads the snapshot.
func (m Model) readSnapshot() tea.Cmd {
	a, ctx := m.app, m.ctx
	return func() tea.Msg {
		snap, err := a.Snapshot(ctx)
		return snapshotMsg{snap, err}
	}
}

// Update handles a message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.run != nil {
			return m, tea.Batch(m.resizeOutput(), m.readRun())
		}
	case changedMsg:
		if m.run != nil {
			return m, tea.Batch(m.readSnapshot(), m.readRun())
		}
		return m, m.readSnapshot()
	case snapshotMsg:
		m.takeSnapshot(msg)
	case tickMsg:
		m.now = m.clock.Now()
		if m.run != nil {
			return m, m.readRun()
		}
	case doneMsg:
		m.setError(msg.err)
	case addedMsg:
		m, cmd := m.added(msg)
		m.scroll()
		return m, cmd
	case completedMsg:
		m = m.completed(msg)
	case deletedMsg:
		if msg.err != nil {
			m.setError(msg.err)
		} else {
			m.setNotice("Deleted " + msg.name + ".")
			if m.page == pageProfile {
				m = m.closeProfile()
			}
		}
		return m, m.readSnapshot()
	case fieldDoneMsg:
		return m.fieldDone(msg)
	case editBeganMsg:
		return m.editBegan(msg)
	case editorDoneMsg:
		return m.editorDone(msg)
	case editEndedMsg:
		return m.editEnded(msg)
	case connectedMsg:
		return m.connected(msg)
	case startedMsg:
		return m.started(msg)
	case runMsg:
		return m.ran(msg)
	case tea.PasteMsg:
		switch {
		case m.ask != nil:
		case m.page == pageAdd:
			m = m.addPaste(msg.Content)
		case m.page == pageProfile:
			m = m.profilePaste(msg.Content)
		case m.page == pageLogin:
			m = m.loginPaste(msg.Content)
		case m.page == pageRun && m.run.tab == tabOutput && !m.run.ended():
			return m, m.input([]byte(msg.Content))
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	m.scroll()
	return m, nil
}

// takeSnapshot keeps a snapshot: the first time the profile used last is selected; later the
// selection stays on the same profile.
func (m *Model) takeSnapshot(msg snapshotMsg) {
	if msg.err != nil {
		m.setError(msg.err)
		return
	}
	selected := ""
	if m.loaded && m.list.selected() < len(m.snap.Profiles) {
		selected = m.snap.Profiles[m.list.selected()].ID
	}
	m.snap, m.now = cleanSnapshot(msg.snap), msg.snap.Now
	m.list.setLen(len(m.snap.Profiles))
	if m.selectID != "" {
		selected, m.selectID = m.selectID, ""
		m.loaded = true
	}
	if !m.loaded {
		m.loaded = true
		m.list.selectIndex(latestUsed(m.snap.Profiles))
		return
	}
	for i, p := range m.snap.Profiles {
		if p.ID == selected {
			m.list.selectIndex(i)
		}
	}
	if _, ok := m.profile(); m.page == pageProfile && !ok {
		*m = m.closeProfile()
	}
}

// cleanSnapshot is a copy of the snapshot whose texts are safe to draw: the core sanitizes what it
// reads, but the TUI does not trust a snapshot to have no control character.
func cleanSnapshot(snap app.Snapshot) app.Snapshot {
	target := make([]string, len(snap.Target))
	for i, a := range snap.Target {
		target[i] = clean(a)
	}
	snap.Target = target
	profiles := make([]app.Profile, len(snap.Profiles))
	for i, p := range snap.Profiles {
		p.Name, p.Broken = clean(p.Name), clean(p.Broken)
		remotes := make([]string, len(p.Remotes))
		for j, r := range p.Remotes {
			remotes[j] = clean(r)
		}
		p.Remotes = remotes
		profiles[i] = p
	}
	snap.Profiles = profiles
	return snap
}

// latestUsed is the index of the profile used last, or 0.
func latestUsed(profiles []app.Profile) int {
	best := 0
	for i, p := range profiles {
		if p.LastUsed.After(profiles[best].LastUsed) {
			best = i
		}
	}
	return best
}

// setError shows the error on the message line, or clears the line for nil.
func (m *Model) setError(err error) {
	if err == nil {
		m.message, m.isError = "", false
		return
	}
	m.message, m.isError = "Error: "+app.Explain(err), true
}

// setNotice shows a notice on the message line.
func (m *Model) setNotice(text string) {
	m.message, m.isError = text, false
}

// scroll moves the list's window so that the selected row is visible.
func (m *Model) scroll() {
	m.list.window(m.listHeight())
}

// key handles a key press. Any key clears the message line first.
func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.message, m.isError = "", false
	name := msg.String()
	if name == "ctrl+c" && m.page != pageRun {
		if m.conn != nil {
			m.conn.cancel()
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	switch {
	case m.ask != nil:
		yes, ok := m.ask.q.answer(name)
		if !ok {
			return m, nil
		}
		ask := m.ask
		m.ask = nil
		if yes {
			m, cmd = ask.onYes(m)
		}
	case m.page == pageHelp:
		m.page = m.helpBack
	case m.page == pageAdd:
		m, cmd = m.addKey(msg)
	case m.page == pageProfile:
		m, cmd = m.profileKey(msg)
	case m.page == pageLogin:
		m, cmd = m.loginKey(msg)
	case m.page == pageConnecting:
		m = m.connectingKey(name)
	case m.page == pageRun:
		m, cmd = m.runKey(msg)
	case m.page == pageProfiles:
		m, cmd = m.profilesKey(name)
	}
	m.scroll()
	return m, cmd
}
