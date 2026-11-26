// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

// Package session is one run of an application through a VPN (docs/SPEC.md §10): it makes the
// namespace, configures it for a connected tunnel, starts the pump and the application, and
// when the application has ended, cleans everything up.
package session

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/pump"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/storage"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// maxNameInEnv is the length of the profile's name in TUBE_PROFILE and in the lines.
const maxNameInEnv = 64

// statusLost is the exit status when the keeper was lost before the application ended.
const statusLost = sandbox.StatusTubeFailed

// Options say what a session runs and with what.
type Options struct {
	// Tunnel is connected; the session closes it.
	Tunnel vpn.Tunnel
	// Dirs are the folders of Tube, for the temporary folder.
	Dirs storage.Dirs
	// Path is the application, absolute (sandbox.LookPath).
	Path string
	// Argv is its argv, argv[0] included.
	Argv []string
	// Notes are lines of Tube about how the application is started, written after the line
	// that says that it runs.
	Notes []string
	// ProfileName is for TUBE_PROFILE and the lines.
	ProfileName string
	// DNS are the profile's own DNS servers, or none.
	DNS []netip.Addr
	// Logger gets the session's records; nil logs nothing.
	Logger *slog.Logger
	// Lines is where Tube's lines go; nil writes none (--quiet).
	Lines io.Writer
	// Clock is nil for the system's.
	Clock clock.Clock
	// ExtraEnv are more variables for the application ("NAME=value"); one replaces a
	// variable of the same name.
	ExtraEnv []string
	// Sandbox gives Exe, StepTimeout, KillDelay, Stdin, Stdout, Stderr, Terminal and Dir; its
	// other fields are not read.
	Sandbox sandbox.Options
	// Redial connects the same profile again after the tunnel failed (docs/SPEC.md §10, D14);
	// nil does not connect again.
	Redial func(ctx context.Context) (vpn.Tunnel, error)
	// OnChange is called after every change of the state and when the application ends, from
	// the session's own goroutine, never while it holds a lock, and never after Wait returned.
	OnChange func()
}

// Session is a running application in its namespace.
type Session struct {
	opts Options
	log  *slog.Logger
	name string
	tmp  string
	sb   *sandbox.Sandbox

	// The connection in use: the tunnel, the pump, the profile's name, its own DNS servers
	// and its Redial. Start sets them; then only the controller (control) touches them.
	tunnel vpn.Tunnel
	pump   *pump.Pump
	dns    []netip.Addr
	redial func(ctx context.Context) (vpn.Tunnel, error)
	// cmds are the commands of the user to the controller.
	cmds chan command
	// snap is what Status shows; the controller writes it under snapMu.
	snapMu sync.Mutex
	snap   snapshot

	linesMu sync.Mutex

	// appEnded is closed when the application has ended: appStatus and appErr say how.
	appEnded  chan struct{}
	appStatus sandbox.ExitStatus
	appErr    error
	// ctrlDone is closed when the controller has ended, and closed the tunnel and the pump in
	// use; traffic is what all the pumps moved.
	ctrlDone chan struct{}
	traffic  pump.Counters

	waitOnce sync.Once
	status   int
	waitErr  error
}

// Start makes the namespace, configures it, starts the pump and the application. When a step
// fails, everything made before it is undone, the tunnel is closed, and the error is returned.
func Start(ctx context.Context, opts Options) (*Session, error) {
	if opts.Clock == nil {
		opts.Clock = clock.System()
	}
	s := &Session{
		opts: opts, log: opts.Logger, name: sanitize.Display(opts.ProfileName, maxNameInEnv), tunnel: opts.Tunnel,
		dns: opts.DNS, redial: opts.Redial, cmds: make(chan command),
		appEnded: make(chan struct{}), ctrlDone: make(chan struct{}),
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if err := s.start(ctx); err != nil {
		s.undo()
		return nil, err
	}
	network := s.tunnel.Network()
	now := opts.Clock.Now()
	s.publish(func(st *Status) {
		*st = Status{State: StateConnected, Since: now, ProfileName: s.name, Network: network, DNS: s.dnsOf(network), AppRunning: true}
	}, false)
	// The wait is handed to its goroutine through errors.Async: the route of its error, which
	// Wait returns, starts here.
	waitApp := errors.Async(s.waitApp)
	go func() {
		s.appErr = waitApp()
		close(s.appEnded)
	}()
	go s.control()
	return s, nil
}

// waitApp waits for the end of the application and keeps its status. It fails when the keeper
// was lost first.
func (s *Session) waitApp() error {
	st, err := s.sb.Wait()
	s.appStatus = st
	return err
}

// start does the steps of Start; undo undoes what it made.
func (s *Session) start(ctx context.Context) error {
	tmp, err := s.opts.Dirs.NewTempDir(ctx)
	if err != nil {
		return err
	}
	s.tmp = tmp
	so := sandbox.Options{
		Path: s.opts.Path, Argv: s.opts.Argv, Env: s.env(), TempDir: tmp,
		Dir: s.opts.Sandbox.Dir, Exe: s.opts.Sandbox.Exe, Terminal: s.opts.Sandbox.Terminal,
		StepTimeout: s.opts.Sandbox.StepTimeout, KillDelay: s.opts.Sandbox.KillDelay,
		Stdin: s.opts.Sandbox.Stdin, Stdout: s.opts.Sandbox.Stdout, Stderr: s.opts.Sandbox.Stderr,
	}
	if s.sb, err = sandbox.Start(ctx, so); err != nil {
		return err
	}
	network := s.tunnel.Network()
	if err := s.configure(ctx, network, s.dns); err != nil {
		return err
	}
	s.pump = pump.Start(s.sb.Device(), s.tunnel, network.MTU)
	if err := s.sb.Run(ctx); err != nil {
		return err
	}
	argv0 := ""
	if len(s.opts.Argv) > 0 {
		argv0 = sanitize.Display(s.opts.Argv[0], maxNameInEnv)
	}
	s.line(fmt.Sprintf("connected to %s; running %s", s.name, argv0))
	for _, note := range s.opts.Notes {
		s.line(note)
	}
	s.log.Info("the application runs", "profile", s.name, "addrs", network.Addrs, "dns", s.dnsOf(network), "mtu", network.MTU)
	return nil
}

// dnsOf are the DNS servers of the namespace with the network: the profile's, else the ones
// that the server pushed.
func (s *Session) dnsOf(n vpn.Network) []netip.Addr {
	if len(s.dns) > 0 {
		return s.dns
	}
	return n.DNSServers
}

// configure configures the namespace for the network and the profile's own DNS servers, and
// writes the warnings as lines.
func (s *Session) configure(ctx context.Context, n vpn.Network, dns []netip.Addr) error {
	warnings, err := s.sb.Configure(ctx, n, dns)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		s.line(w)
		s.log.Warn("the names of the namespace", "warning", w)
	}
	return nil
}

// undo undoes what start made, in the reverse order, and closes the tunnel. The error of the
// start is the one that matters: the failures of the undoing are only logged.
func (s *Session) undo() {
	if s.pump != nil {
		s.pump.Stop()
	}
	if s.sb != nil {
		if err := s.sb.Close(); err != nil {
			s.log.Warn("the namespace was closed with an error", "err", err)
		}
	}
	s.closeTunnel()
	s.removeTemp()
}

// closeTunnel closes the tunnel in use and logs a failure.
func (s *Session) closeTunnel() {
	if s.tunnel != nil {
		s.closeTunnelOf(s.tunnel)
		s.tunnel = nil
	}
}

// closeTunnelOf closes a tunnel and logs a failure.
func (s *Session) closeTunnelOf(tun vpn.Tunnel) {
	if err := tun.Close(); err != nil {
		s.log.Warn("the tunnel was closed with an error", "err", err)
	}
}

// removeTemp removes the temporary folder and logs a failure.
func (s *Session) removeTemp() {
	if s.tmp == "" {
		return
	}
	if err := os.RemoveAll(s.tmp); err != nil {
		s.log.Warn("the temporary folder cannot be removed", "err", err)
	}
}

// env is the application's environment: Tube's, with TUBE, TUBE_PROFILE, MOZ_NO_REMOTE and the
// extra variables, each replacing a variable of the same name.
func (s *Session) env() []string {
	env := os.Environ()
	vars := append([]string{"TUBE=1", "TUBE_PROFILE=" + s.name, "MOZ_NO_REMOTE=1"}, s.opts.ExtraEnv...)
	for _, v := range vars {
		name, _, _ := strings.Cut(v, "=")
		kept := env[:0:0]
		for _, e := range env {
			if !strings.HasPrefix(e, name+"=") {
				kept = append(kept, e)
			}
		}
		env = append(kept, v)
	}
	return env
}

// line writes one of Tube's lines, when there is a place for them.
func (s *Session) line(text string) {
	if s.opts.Lines == nil {
		return
	}
	s.linesMu.Lock()
	defer s.linesMu.Unlock()
	if _, err := io.WriteString(s.opts.Lines, "tube: "+text+"\n"); err != nil {
		s.log.Warn("a line of Tube cannot be written", "err", err)
	}
}

// reason is a few words that say why a connection ended, never the error's text.
func reason(err error) string {
	switch vpn.KindOf(err) {
	case vpn.KindNoAnswer:
		return "the server did not answer"
	case vpn.KindServerEnded:
		return "the server closed the connection"
	case vpn.KindAuthFailed:
		return "the server refused the login"
	case vpn.KindCertificate:
		return "the server's certificate was refused"
	case vpn.KindUnsupported:
		return "the server asked for what Tube does not support"
	case vpn.KindCancelled:
		return "it was cancelled"
	}
	return "it failed"
}

// Wait returns when the application has ended, with the exit status of docs/SPEC.md §10, and
// cleans up: the pump, the tunnel, the namespace and the temporary folder. When the keeper was
// lost it returns 125 and the error. A second call returns the same values.
func (s *Session) Wait() (int, error) {
	s.waitOnce.Do(func() { s.status, s.waitErr = s.wait() })
	return s.status, s.waitErr
}

// wait is Wait, once.
func (s *Session) wait() (int, error) {
	<-s.appEnded
	// The controller ends with the application, and closes the pump and the tunnel in use.
	<-s.ctrlDone
	st, waitErr := s.appStatus, s.appErr
	c := s.traffic
	s.log.Info("the application ended", "status", st.Status(), "to_tunnel_packets", c.ToTunnel.Packets, "to_tunnel_bytes", c.ToTunnel.Bytes,
		"from_tunnel_packets", c.FromTunnel.Packets, "from_tunnel_bytes", c.FromTunnel.Bytes, "dropped", c.Dropped)
	if err := s.sb.Close(); err != nil {
		s.log.Warn("the namespace was closed with an error", "err", err)
	}
	s.removeTemp()
	if waitErr != nil {
		s.log.Error("the keeper was lost", "err", waitErr)
		return statusLost, waitErr
	}
	return st.Status(), nil
}

// Signal passes a signal to the application. An application that has ended needs none.
func (s *Session) Signal(sig syscall.Signal) {
	if err := s.sb.Signal(sig); err != nil {
		s.log.Debug("a signal did not reach the application", "signal", int(sig), "err", err)
	}
}
