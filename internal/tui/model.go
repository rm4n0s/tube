// SPDX-License-Identifier: GPL-2.0-only

// Package tui is Tube's terminal UI (docs/SPEC.md §13), on Bubble Tea. One model holds plain
// data: the snapshot of the app, the size, the page and each page's own state. View only draws
// what the model holds; a key that changes something returns a command that calls the app and
// answers with a message.
package tui

import (
	"context"
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
	// message is the line above the keys; isError draws it as an error.
	message string
	isError bool

	result Result
}

// NewModel makes the model of the TUI over the app.
func NewModel(ctx context.Context, a app.Application, clk clock.Clock) Model {
	if clk == nil {
		clk = clock.System()
	}
	return Model{ctx: ctx, app: a, clock: clk, styles: newStyles(), page: pageProfiles}
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
	m.snap, m.now = msg.snap, msg.snap.Now
	m.list.setLen(len(m.snap.Profiles))
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
	switch m.page {
	case pageHelp:
		m.page = pageProfiles
	case pageProfiles:
		m, cmd = m.profilesKey(name)
	}
	m.scroll()
	return m, cmd
}
