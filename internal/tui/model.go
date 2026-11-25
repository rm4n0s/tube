// SPDX-License-Identifier: GPL-2.0-only

// Package tui is Tube's terminal UI (docs/SPEC.md §13), on Bubble Tea. One model holds plain
// data: the snapshot of the app, the size, the page and each page's own state. View only draws
// what the model holds; a key that changes something returns a command that calls the app and
// answers with a message.
package tui

import (
	"context"
	"os"
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

// notBuilt is the notice of a key whose page comes in a later step.
const notBuilt = "not built yet"

// Result is what the user chose in the TUI.
type Result struct {
	// Run says that the application is to be run (step 5.5).
	Run bool
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
	// ask is the open question; it gets every key.
	ask *asking
	// message is the line above the keys; isError draws it as an error.
	message string
	isError bool

	// home and readDir are for the completion of paths.
	home    string
	readDir func(dir string) ([]os.DirEntry, error)

	result Result
}

// NewModel makes the model of the TUI over the app.
func NewModel(ctx context.Context, a app.Application, clk clock.Clock) Model {
	if clk == nil {
		clk = clock.System()
	}
	// Without a home folder, "~" is not completed; nothing else needs it.
	home, _ := os.UserHomeDir()
	return Model{ctx: ctx, app: a, clock: clk, styles: newStyles(), page: pageProfiles, home: home, readDir: defaultReadDir}
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
	case changedMsg:
		return m, m.readSnapshot()
	case snapshotMsg:
		m.takeSnapshot(msg)
	case tickMsg:
		m.now = m.clock.Now()
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
		}
		return m, m.readSnapshot()
	case tea.PasteMsg:
		if m.ask == nil && m.page == pageAdd {
			m = m.addPaste(msg.Content)
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
	if name == "ctrl+c" {
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
		m.page = pageProfiles
	case m.page == pageAdd:
		m, cmd = m.addKey(msg)
	case m.page == pageProfiles:
		m, cmd = m.profilesKey(name)
	}
	m.scroll()
	return m, cmd
}
