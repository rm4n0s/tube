// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/pty"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/session"
	"github.com/rm4n0s/tube/internal/core/termlog"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// The Tags of running under the TUI.
const (
	TagAlreadyStarted = "AppAlreadyStarted"
	TagNotStarted     = "AppNotStarted"
	TagInputFailed    = "AppInputFailed"
)

// The sizes of what a run keeps (docs/SPEC.md §12.4, §13.9).
const (
	outputLines   = 5000
	outputWidth   = 1000
	maxEvents     = 100
	maxEventText  = 300
	linePrefix    = "tube: "
	sampleEvery   = time.Second
	notifyPause   = 50 * time.Millisecond
	killDelay     = 5 * time.Second
	outputGrace   = time.Second
	defaultCols   = 80
	defaultRows   = 24
	maxLineBuffer = 4096
)

// run is what Start made: the session, the pseudo-terminal and its log, and the goroutines
// that Finish waits for.
type run struct {
	s       *session.Session
	master  *os.File
	out     *termlog.Log
	appName string
	// copied, sampled and joined are closed when their goroutines end; stopJoin ends the
	// goroutine that joins the notifications.
	copied, sampled, joined chan struct{}
	stopJoin                chan struct{}
	// kick asks for a notification of the subscribers.
	kick chan struct{}
	// finish makes Finish's work happen once; status and err are its result.
	finish sync.Once
	status int
	err    error
}

// runData is what a run changes while the application runs; it is under App.mu.
type runData struct {
	run       *run
	profileID string
	events    []Event
	samples   sampler
	// partial is an unfinished line of Tube's.
	partial []byte
	// cols and rows are the size of the application's terminal.
	cols, rows int
	// killTimer sends SIGKILL after a SIGTERM that did not end the application.
	killTimer *time.Timer
	// killDelay is the time between SIGTERM and SIGKILL; 0 is 5 seconds (the tests shorten it).
	killDelay time.Duration
}

// Start starts the application on a pseudo-terminal, through the VPN that Connect made
// (docs/SPEC.md §12.4). It returns when the application has started; it does not wait for its
// end. The tunnel belongs to the session from then on; when the application cannot be
// started, the tunnel is closed.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.rd.run != nil || a.running {
		a.mu.Unlock()
		return errors.New(TagAlreadyStarted, "the application was started already")
	}
	a.running = true
	cols, rows := a.rd.cols, a.rd.rows
	a.mu.Unlock()
	started := false
	defer func() {
		if !started {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
		}
	}()
	tk, err := a.takeTunnel(ctx)
	if err != nil {
		return err
	}
	tun, p, creds, path := tk.tun, tk.p, tk.creds, tk.path
	master, slave, err := pty.Open()
	if err != nil {
		a.closeTunnelOf(tun)
		return err
	}
	if cols <= 0 || rows <= 0 {
		cols, rows = defaultCols, defaultRows
	}
	// A terminal of no size is still a terminal: a failure changes nothing else.
	_ = pty.SetSize(master, cols, rows)
	r := &run{
		master: master, out: termlog.New(outputLines, outputWidth),
		appName: sanitize.Display(filepath.Base(a.opts.Argv[0]), maxNameInError),
		copied:  make(chan struct{}), sampled: make(chan struct{}), joined: make(chan struct{}),
		stopJoin: make(chan struct{}), kick: make(chan struct{}, 1),
	}
	r.out.OnWrite(func() { r.notify() })
	go a.join(r)
	a.log.Info("run started", "profile", p.ID, "name", p.Name, "application", r.appName, "terminal", true)
	s, err := session.Start(ctx, session.Options{
		Tunnel: tun, Dirs: a.dirs, Path: path, Argv: tk.argv, Notes: notes(tk.note), ProfileName: p.Name, DNS: p.DNS,
		Logger: a.log, Lines: eventWriter{a: a, r: r}, Clock: a.opts.Clock, ExtraEnv: []string{"TERM=dumb"},
		Sandbox:  sandbox.Options{Stdin: slave, Stdout: slave, Stderr: slave, Terminal: true},
		Redial:   a.redialer(p.ID, creds),
		OnChange: r.notify,
	})
	// The session's keeper has the slave side now; Tube keeps only the master side.
	if cerr := slave.Close(); cerr != nil {
		a.log.Warn("the slave side of the terminal was closed with an error", "err", cerr)
	}
	if err != nil {
		// session.Start closed the tunnel.
		close(r.stopJoin)
		<-r.joined
		if cerr := master.Close(); cerr != nil {
			a.log.Warn("the terminal was closed with an error", "err", cerr)
		}
		a.log.Error("run failed", "err", err, "profile", p.ID)
		return err
	}
	r.s = s
	a.mu.Lock()
	a.session, a.rd.run, a.rd.profileID = s, r, p.ID
	pending := a.pending
	a.mu.Unlock()
	started = true
	if pending != 0 {
		a.Signal(pending)
	}
	go a.copyOutput(r)
	// The ticker is made here, so that a second of the clock after Start is never missed.
	go a.sample(r, a.opts.Clock.NewTicker(sampleEvery))
	r.notify()
	return nil
}

// notify asks for a notification of the subscribers; it never waits.
func (r *run) notify() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// join calls the subscribers for the asked notifications, at most one every 50 ms of real
// time, so that a program that prints fast does not flood the TUI.
func (a *App) join(r *run) {
	defer close(r.joined)
	for {
		select {
		case <-r.kick:
		case <-r.stopJoin:
			return
		}
		a.notify()
		select {
		case <-time.After(notifyPause):
		case <-r.stopJoin:
			return
		}
	}
}

// copyOutput copies the master side into the log until reading fails: EIO is the normal end,
// when every program that had the terminal is gone.
func (a *App) copyOutput(r *run) {
	defer close(r.copied)
	buf := make([]byte, 4096)
	for {
		n, err := r.master.Read(buf)
		if n > 0 {
			// termlog's Write does not fail.
			_, _ = r.out.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// sample takes the traffic every second of the App's clock, for the rates, until the
// application ends.
func (a *App) sample(r *run, t clock.Ticker) {
	defer close(r.sampled)
	defer t.Stop()
	for {
		select {
		case <-t.C():
			c := r.s.Status().Traffic
			a.mu.Lock()
			a.rd.samples.add(c)
			a.mu.Unlock()
			r.notify()
		case <-r.s.Done():
			return
		}
	}
}

// eventWriter turns Tube's lines into events.
type eventWriter struct {
	a *App
	r *run
}

// Write keeps each whole line as an event, without "tube: " in front.
func (w eventWriter) Write(p []byte) (int, error) {
	a := w.a
	now := a.opts.Clock.Now()
	a.mu.Lock()
	a.rd.partial = append(a.rd.partial, p...)
	for {
		i := bytes.IndexByte(a.rd.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimPrefix(string(a.rd.partial[:i]), linePrefix)
		a.rd.partial = a.rd.partial[i+1:]
		a.rd.events = append(a.rd.events, Event{Time: now, Text: sanitize.Display(line, maxEventText)})
		if over := len(a.rd.events) - maxEvents; over > 0 {
			a.rd.events = slices.Delete(a.rd.events, 0, over)
		}
	}
	if len(a.rd.partial) > maxLineBuffer {
		a.rd.partial = a.rd.partial[:0]
	}
	a.mu.Unlock()
	w.r.notify()
	return len(p), nil
}

// RunState is what the run screen draws from. Before Start, it is the zero value with Now.
func (a *App) RunState() RunState {
	a.mu.Lock()
	r, id, samples := a.rd.run, a.rd.profileID, a.rd.samples
	events := slices.Clone(a.rd.events)
	a.mu.Unlock()
	rs := RunState{Now: a.opts.Clock.Now()}
	if r == nil {
		return rs
	}
	st := r.s.Status()
	down, up := samples.rates()
	rs.Started, rs.AppName, rs.AppRunning, rs.Exit = true, r.appName, st.AppRunning, st.Exit
	rs.State, rs.Since, rs.ProfileID, rs.ProfileName = st.State.String(), st.Since, id, st.ProfileName
	for _, p := range st.Network.Addrs {
		rs.Addrs = append(rs.Addrs, p.String())
	}
	rs.DNS = addrStrings(st.DNS)
	rs.MTU = st.Network.MTU
	rs.Down = Traffic{Bytes: st.Traffic.FromTunnel.Bytes, Packets: st.Traffic.FromTunnel.Packets, Rate: down}
	rs.Up = Traffic{Bytes: st.Traffic.ToTunnel.Bytes, Packets: st.Traffic.ToTunnel.Packets, Rate: up}
	rs.Dropped = st.Traffic.Dropped
	rs.Events = events
	return rs
}

// startedRun is the run of Start, or nil.
func (a *App) startedRun() *run {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rd.run
}

// OutputLen is termlog's Len, for the application's output.
func (a *App) OutputLen() (first, end int) {
	if r := a.startedRun(); r != nil {
		return r.out.Len()
	}
	return 0, 0
}

// OutputRange is termlog's Range, for the application's output.
func (a *App) OutputRange(from, to int) []string {
	if r := a.startedRun(); r != nil {
		return r.out.Range(from, to)
	}
	return nil
}

// Input writes what the user typed to the application's terminal. After the application ended
// it does nothing.
func (a *App) Input(p []byte) error {
	r := a.startedRun()
	if r == nil {
		return nil
	}
	select {
	case <-r.s.Done():
		return nil
	default:
	}
	if _, err := r.master.Write(p); err != nil {
		return errors.NewErr(TagInputFailed, err)
	}
	return nil
}

// ResizeOutput tells the application's terminal its size; before Start, the size is kept for
// it.
func (a *App) ResizeOutput(cols, rows int) {
	a.mu.Lock()
	a.rd.cols, a.rd.rows = cols, rows
	r := a.rd.run
	a.mu.Unlock()
	if r != nil && cols > 0 && rows > 0 {
		// The terminal's size only helps the application; a failure changes nothing else.
		_ = pty.SetSize(r.master, cols, rows)
	}
}

// Disconnect takes the network away from the application.
func (a *App) Disconnect() {
	if r := a.startedRun(); r != nil {
		r.s.Disconnect()
	}
}

// Switch connects the profile and gives it to the running application in place of the VPN it
// has. The VPN in use stays until the new one is connected; when the new one fails, nothing
// changes.
func (a *App) Switch(ctx context.Context, id string, creds *Credentials, remember Remember) error {
	r := a.startedRun()
	if r == nil {
		return errors.New(TagNotStarted, "the application was not started")
	}
	d, err := a.dial(ctx, id, creds, remember)
	if err != nil {
		a.log.Error("switch failed", "err", err, "profile", id)
		return err
	}
	err = r.s.Use(ctx, session.Connection{Tunnel: d.tun, ProfileName: d.p.Name, DNS: d.p.DNS, Redial: a.redialer(d.p.ID, d.creds)})
	if err != nil {
		a.log.Error("switch failed", "err", err, "profile", id)
		return err
	}
	a.mu.Lock()
	a.rd.profileID, a.creds = d.p.ID, d.creds
	a.mu.Unlock()
	return nil
}

// Stop asks the application to end: SIGTERM, and SIGKILL five seconds later.
func (a *App) Stop() { a.Signal(syscall.SIGTERM) }

// Finish stops the application when it still runs, waits for it, cleans up, and returns its
// exit status. cmd/tube calls it after the TUI has ended; a second call returns the same.
// Without Start there is nothing to finish: 0.
func (a *App) Finish() (int, error) {
	r := a.startedRun()
	if r == nil {
		return 0, nil
	}
	r.finish.Do(func() { r.status, r.err = a.finish(r) })
	return r.status, r.err
}

// finish is Finish, once.
func (a *App) finish(r *run) (int, error) {
	select {
	case <-r.s.Done():
	default:
		a.Stop()
	}
	status, err := r.s.Wait()
	a.stopKillTimer()
	a.mu.Lock()
	a.running = false
	id := a.rd.profileID
	a.mu.Unlock()
	<-r.sampled
	// What the application wrote last is read until EIO; a program that it left behind with
	// the terminal must not keep Tube waiting.
	select {
	case <-r.copied:
	case <-time.After(outputGrace):
	}
	if cerr := r.master.Close(); cerr != nil {
		a.log.Warn("the terminal was closed with an error", "err", cerr)
	}
	<-r.copied
	close(r.stopJoin)
	<-r.joined
	if err != nil {
		a.log.Error("run failed", "err", err, "profile", id)
		return status, err
	}
	a.log.Info("run ended", "status", status)
	return status, nil
}

// dialed is a connected tunnel, its profile and the credentials of the dial.
type dialed struct {
	tun   vpn.Tunnel
	p     profiles.Profile
	creds *vpn.Credentials
}

// dial connects the profile, logs it, marks the profile as used and, with remember, keeps the
// user name of the credentials as the profile's.
func (a *App) dial(ctx context.Context, id string, creds *Credentials, remember Remember) (dialed, error) {
	p, err := a.store.Get(ctx, id)
	if err != nil {
		return dialed{}, err
	}
	var vc *vpn.Credentials
	switch {
	case creds != nil:
		vc = &vpn.Credentials{Username: creds.Username, Password: creds.Password}
	case p.NeedsPassword && p.HasPassword:
		// No credentials were given: the remembered ones are used.
		password, err := a.store.Password(ctx, id)
		if err != nil {
			return dialed{}, err
		}
		vc = &vpn.Credentials{Username: p.Username, Password: password}
	}
	tun, err := vpn.Dial(ctx, p, vc)
	if err != nil {
		return dialed{}, err
	}
	n := tun.Network()
	a.log.Info("connected", "profile", p.ID, "name", p.Name, "addrs", n.Addrs, "dns", n.DNSServers, "mtu", n.MTU)
	// The connection is what the user asked for: these only change what the list shows.
	changed := false
	if err := a.store.MarkUsed(ctx, id); err != nil {
		a.log.Warn("the profile cannot be marked as used", "err", err, "profile", id)
	} else {
		changed = true
	}
	if creds != nil && a.remember(ctx, p, creds, remember) {
		changed = true
	}
	if changed {
		a.notify()
	}
	return dialed{tun: tun, p: p, creds: vc}, nil
}

// remember keeps, of the credentials that just connected the profile, what remember says, and
// says whether the profile changed. A failure is logged: the connection stands. The password is
// kept with its user name, without which it is of no use.
func (a *App) remember(ctx context.Context, p profiles.Profile, creds *Credentials, remember Remember) bool {
	changed := false
	if remember.User || remember.Password {
		if err := a.store.SetUsername(ctx, p.ID, creds.Username); err != nil {
			a.log.Warn("the user name cannot be remembered", "err", err, "profile", p.ID)
		} else {
			changed = true
		}
	}
	switch {
	case remember.Password:
		if err := a.store.SetPassword(ctx, p.ID, creds.Password); err != nil {
			a.log.Warn("the password cannot be remembered", "err", err, "profile", p.ID)
		} else {
			changed = true
		}
	case remember.Forget && p.HasPassword:
		if err := a.store.SetPassword(ctx, p.ID, ""); err != nil {
			a.log.Warn("the password cannot be forgotten", "err", err, "profile", p.ID)
		} else {
			changed = true
		}
	}
	return changed
}

// killLater sends SIGKILL to the application when a SIGTERM or SIGHUP has not ended it after
// the kill delay (docs/SPEC.md §10). The caller holds a.mu.
func (a *App) killLater(s *session.Session) {
	if a.rd.killTimer != nil {
		return
	}
	delay := a.rd.killDelay
	if delay <= 0 {
		delay = killDelay
	}
	a.rd.killTimer = time.AfterFunc(delay, func() {
		select {
		case <-s.Done():
		default:
			a.log.Warn("the application did not end after SIGTERM; it is killed")
			s.Signal(syscall.SIGKILL)
		}
	})
}

// stopKillTimer stops the timer of killLater.
func (a *App) stopKillTimer() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rd.killTimer != nil {
		a.rd.killTimer.Stop()
		a.rd.killTimer = nil
	}
}
