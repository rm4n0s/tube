// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// ran keeps what the run screen draws; the model quits when the application it was stopping
// has ended.
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
	if r.stopping && !r.state.AppRunning {
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
	case "tab", "shift+tab":
		if name == "tab" {
			r.tab = (r.tab + 1) % runTabCount
		} else {
			r.tab = (r.tab + runTabCount - 1) % runTabCount
		}
		if r.tab == tabNetwork {
			m.selectInUse()
		}
		return m, m.readRun()
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
func (m Model) runView() string {
	lines := make([]string, 0, m.height)
	lines = append(lines, fit(m.runHeader(), m.width), fit(m.tabsLine(), m.width), m.rule())
	var body []string
	switch m.run.tab {
	case tabStatus:
		body = m.statusBody()
	case tabOutput:
		body = m.outputBody()
	case tabNetwork:
		body = m.networkBody()
	}
	if m.ask != nil {
		body = overlay(body, m.ask.q.box(m.width, m.styles), m.width, m.runPageHeight())
	}
	for i := range m.runPageHeight() {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		lines = append(lines, fit(line, m.width))
	}
	return strings.Join(append(lines, m.rule(), fit(m.messageLine(), m.width), fit(m.runFooter(), m.width)), "\n")
}

// runHeader is line 0: the application, the profile, the state and the rates. When it does not
// fit, the rates go first, then the time of the state.
func (m Model) runHeader() string {
	st := m.run.state
	who := st.AppName
	if m.run.ended() {
		who += " ended (status " + strconv.Itoa(st.Exit) + ")"
	}
	if st.ProfileName != "" {
		who += " through " + st.ProfileName
	}
	base := m.styles.title.Render("Tube") + "   " + who
	if m.run.ended() {
		// The session closed the VPN with the application: it has no state, and no rates.
		return base
	}
	word, timed := stateWords(st)
	rates := "↓ " + rateText(st.Down.Rate) + "  ↑ " + rateText(st.Up.Rate)
	for _, line := range []string{base + "   " + timed + "   " + rates, base + "   " + timed, base + "   " + word} {
		if lipgloss.Width(line) <= m.width {
			return line
		}
	}
	return base + "   " + word
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

// tabsLine is line 1: the tabs, the current one in brackets and bold.
func (m Model) tabsLine() string {
	parts := make([]string, 0, runTabCount)
	for i, name := range tabNames {
		if runTab(i) == m.run.tab {
			parts = append(parts, m.styles.title.Render("["+name+"]"))
		} else {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, "  ")
}

// runFooter is the keys of the tab.
func (m Model) runFooter() string {
	if m.ask != nil {
		return m.footer()
	}
	p := pageRunStatus
	switch m.run.tab {
	case tabOutput:
		return m.outputFooter()
	case tabNetwork:
		if f := m.networkFooter(); f != "" {
			return f
		}
		p = pageRunNetwork
	}
	var parts []string
	for _, b := range footerBindings(p, len(m.snap.Target) > 0) {
		parts = append(parts, m.styles.key.Render(b.keys)+" "+b.what)
	}
	return strings.Join(parts, "   ")
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
		return r.state.AppName + " ended with status " + strconv.Itoa(r.state.Exit) + ". Press q to quit."
	}
	return ""
}
