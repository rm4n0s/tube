// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rm4n0s/tube/internal/app"
)

// runTab is a tab of the run screen.
type runTab int

// The tabs of the run screen (docs/SPEC.md §13.8).
const (
	tabStatus runTab = iota
	tabOutput
	tabNetwork
	runTabCount
)

// tabNames are the names of the tabs, in their order.
var tabNames = [runTabCount]string{"Status", "Output", "Network"}

// runFrameLines are the lines of the run screen around the tab's page: the header, the tabs,
// two rules, the message and the keys.
const runFrameLines = 6

// The messages of the run screen.
type (
	// startedMsg is the end of Start.
	startedMsg struct{ err error }
	// runMsg is what the run screen draws: the run state and the lines of the output in the
	// window, which begins at from.
	runMsg struct {
		state      app.RunState
		first, end int
		from       int
		lines      []string
	}
)

// runScreen is the state of the run screen.
type runScreen struct {
	tab   runTab
	state app.RunState
	// outFirst and outEnd are the numbers of the output's lines; lines are the ones of the
	// window, which begins at from.
	outFirst, outEnd int
	from             int
	lines            []string
	// follow shows the end of the output; else the window begins at hold.
	follow bool
	hold   int
	// stopping says that the user asked to stop the application and quit.
	stopping bool
	// switching is the switch that runs on the Network tab, or nil.
	switching *switching
}

// runPageHeight is the number of lines of a tab's page.
func (m Model) runPageHeight() int { return max(m.height-runFrameLines, 0) }

// start is the command that starts the application, after the connect.
func (m Model) start() tea.Cmd {
	a, ctx := m.app, m.ctx
	return func() tea.Msg { return startedMsg{err: a.Start(ctx)} }
}

// started shows the run screen, or goes back to the list after an error.
func (m Model) started(msg startedMsg) (Model, tea.Cmd) {
	m.conn, m.login, m.prof = nil, nil, nil
	if msg.err != nil {
		m.page = pageProfiles
		m.setError(msg.err)
		return m, m.readSnapshot()
	}
	m.result.Started = true
	m.page = pageRun
	name := ""
	if len(m.snap.Target) > 0 {
		name = m.snap.Target[0]
	}
	// Until the first run state comes, the application runs.
	m.run = &runScreen{follow: true, state: app.RunState{Started: true, AppRunning: true, AppName: name}}
	return m, tea.Batch(m.resizeOutput(), m.readRun())
}

// resizeOutput is the command that gives the application's terminal the size of a page.
func (m Model) resizeOutput() tea.Cmd {
	a, w, h := m.app, m.width, m.runPageHeight()
	return func() tea.Msg {
		a.ResizeOutput(w, h)
		return nil
	}
}

// readRun is the command that reads the run state and the window of the output.
func (m Model) readRun() tea.Cmd {
	if m.run == nil {
		return nil
	}
	a, n, follow, hold := m.app, m.runPageHeight(), m.run.follow, m.run.hold
	return func() tea.Msg {
		state := a.RunState()
		first, end := a.OutputLen()
		from := max(hold, first)
		if follow {
			from = max(end-n, first)
		}
		to := min(from+n, end)
		var lines []string
		if from < to {
			lines = a.OutputRange(from, to)
		}
		return runMsg{state: state, first: first, end: end, from: from, lines: lines}
	}
}

// ran keeps what the run screen draws. When the application has ended, the model quits: Tube
// ends with its application (docs/SPEC.md D31), whether the user stopped it or it ended by
// itself.
func (m Model) ran(msg runMsg) (Model, tea.Cmd) {
	if m.run == nil {
		return m, nil
	}
	r := *m.run
	m.run = &r
	r.state, r.outFirst, r.outEnd, r.from, r.lines = msg.state, msg.first, msg.end, msg.from, msg.lines
	if !r.follow {
		r.hold = msg.from
	}
	m.now = msg.state.Now
	if !r.state.AppRunning {
		m.result.Ended = !r.stopping
		return m, tea.Quit
	}
	return m, nil
}

// ended says that the application has ended.
func (r *runScreen) ended() bool { return !r.state.AppRunning }

// runKey handles a key on the run screen.
func (m Model) runKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	r := *m.run
	m.run = &r
	name := msg.String()
	switch name {
	case "tab":
		return m.showTab((r.tab + 1) % runTabCount)
	case "shift+tab":
		return m.showTab((r.tab + runTabCount - 1) % runTabCount)
	}
	switch r.tab {
	case tabOutput:
		return m.outputKey(msg)
	case tabNetwork:
		return m.networkKey(name)
	}
	switch name {
	case "q", "ctrl+c":
		return m.quitKey(name)
	case "?":
		m.page, m.helpBack = pageHelp, pageRun
	}
	return m, nil
}

// showTab shows a tab of the run screen; the Network tab opens with the profile in use
// selected.
func (m Model) showTab(tab runTab) (Model, tea.Cmd) {
	r := *m.run
	m.run = &r
	r.tab = tab
	if tab == tabNetwork {
		m.selectInUse()
	}
	return m, m.readRun()
}

// quitKey is q or ctrl+c: after the end, quit; while it runs, ask; while it is being stopped, a
// second ctrl+c quits at once (Finish then kills it).
func (m Model) quitKey(name string) (Model, tea.Cmd) {
	r := m.run
	switch {
	case r.ended():
		return m, tea.Quit
	case r.stopping:
		if name == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	a := m.app
	m.ask = &asking{
		q: question{text: r.state.AppName + " is still running. Stop it and quit?", yes: "Stop and quit"},
		onYes: func(m Model) (Model, tea.Cmd) {
			r := *m.run
			r.stopping = true
			m.run = &r
			return m, func() tea.Msg {
				a.Stop()
				return nil
			}
		},
	}
	return m, nil
}

// runView draws the run screen: the header, the tabs, a rule, the tab's page, a rule, the
// message and the keys.
func (m Model) runView() []line {
	lines := make([]line, 0, m.height)
	lines = append(lines, m.runHeader(), m.tabsLine(), m.rule())
	var body []line
	switch m.run.tab {
	case tabStatus:
		body = m.statusBody()
	case tabOutput:
		body = m.outputBody()
	case tabNetwork:
		body = m.networkBody()
	}
	lines = append(lines, m.paged(body, m.runPageHeight())...)
	return append(lines, m.rule(), m.messageLine(), m.runFooter())
}

// minHeaderName is the number of cells that the name of the profile keeps on line 0, at least,
// before the rates and then the time of the state are left out.
const minHeaderName = 8

// runHeader is line 0: the application, the profile, the state and the rates. When it does not
// fit, the name of the profile is cut first, down to minHeaderName cells; then the rates go,
// then the time of the state.
func (m Model) runHeader() line {
	st := m.run.state
	sty := m.styles
	who := sty.accent.Render(st.AppName)
	if m.run.ended() {
		who += sty.warn.Render(" ended (status " + strconv.Itoa(st.Exit) + ")")
	}
	base := m.badge() + "  " + who
	// through is the profile, in the cells that tail leaves.
	through := func(tail string) (string, bool) {
		if st.ProfileName == "" {
			return "", lipgloss.Width(base)+lipgloss.Width(tail) <= m.width
		}
		room := m.width - lipgloss.Width(base) - len(" through ") - lipgloss.Width(tail)
		name := st.ProfileName
		fits := room >= min(lipgloss.Width(name), minHeaderName)
		if lipgloss.Width(name) > room {
			name = ansi.Truncate(name, max(room, 1), ellipsis)
		}
		return " through " + sty.title.Render(name), fits
	}
	if m.run.ended() {
		// The session closed the VPN with the application: it has no state, and no rates.
		name, _ := through("")
		return text(base + name)
	}
	word, timed := stateWords(st)
	color := m.stateStyle(st.State)
	rates := sty.down.Render("↓ "+rateText(st.Down.Rate)) + "  " + sty.up.Render("↑ "+rateText(st.Up.Rate))
	short := "   " + color.Render(word)
	for _, tail := range []string{"   " + color.Render(timed) + "   " + rates, "   " + color.Render(timed), short} {
		if name, fits := through(tail); fits {
			return text(base + name + tail)
		}
	}
	name, _ := through(short)
	return text(base + name + short)
}

// stateStyle is the style of a state of the VPN: well when it is connected, attention while it
// connects again, an error when the application has no network.
func (m Model) stateStyle(state string) lipgloss.Style {
	switch state {
	case "connected":
		return m.styles.ok
	case "reconnecting":
		return m.styles.warn
	}
	return m.styles.err
}

// stateWords are the words of the state, without and with its time.
func stateWords(st app.RunState) (word, timed string) {
	switch st.State {
	case "connected":
		return "connected", "connected " + durationText(st.Now.Sub(st.Since))
	case "reconnecting":
		return "connecting again…", "connecting again…"
	case "disconnected":
		return "disconnected", "disconnected"
	case "refused":
		return "login refused", "login refused"
	}
	return "", ""
}

// tabsLine is line 1: the tabs, the current one in brackets and selected. A click on a tab
// shows it.
func (m Model) tabsLine() line {
	var l line
	for i, name := range tabNames {
		if i > 0 {
			l = l.add("  ")
		}
		tab := runTab(i)
		label := m.styles.tab.Render(name)
		if tab == m.run.tab {
			label = m.styles.selected.Render("[" + name + "]")
		}
		l = l.button(label, func(m Model, _ click) (Model, tea.Cmd) { return m.showTab(tab) })
	}
	return l
}

// runFooter is the keys of the tab.
func (m Model) runFooter() line {
	if m.ask != nil {
		return m.footer()
	}
	p := pageRunStatus
	switch m.run.tab {
	case tabOutput:
		return m.outputFooter()
	case tabNetwork:
		if m.run.switching != nil {
			return m.hint(binding{"esc", "cancel"})
		}
		p = pageRunNetwork
	}
	var hints []line
	for _, b := range footerBindings(p, len(m.snap.Target) > 0) {
		hints = append(hints, m.hint(b))
	}
	return joinHints(hints, m.width)
}

// runNotice is the message line's text when nothing else is on it: that the application is
// being stopped, or that it ended.
func (m Model) runNotice() string {
	r := m.run
	switch {
	case r == nil:
		return ""
	case r.stopping && !r.ended():
		return "Stopping " + r.state.AppName + "…"
	case r.ended():
		return r.state.AppName + " ended with status " + strconv.Itoa(r.state.Exit) + "."
	}
	return ""
}
