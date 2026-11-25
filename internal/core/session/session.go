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
}

// Session is a running application in its namespace.
type Session struct {
	opts    Options
	log     *slog.Logger
	name    string
	tmp     string
	sb      *sandbox.Sandbox
	pump    *pump.Pump
	network vpn.Network

	linesMu sync.Mutex

	// stopWatch ends the watcher of the pump; watchDone is closed when it has ended.
	stopWatch chan struct{}
	watchDone chan struct{}

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
		opts: opts, log: opts.Logger, name: sanitize.Display(opts.ProfileName, maxNameInEnv),
		stopWatch: make(chan struct{}), watchDone: make(chan struct{}),
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if err := s.start(ctx); err != nil {
		s.undo()
		return nil, err
	}
	go s.watch()
	return s, nil
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
	s.network = s.opts.Tunnel.Network()
	warnings, err := s.sb.Configure(ctx, s.network, s.opts.DNS)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		s.line(w)
		s.log.Warn("the names of the namespace", "warning", w)
	}
	s.pump = pump.Start(s.sb.Device(), s.opts.Tunnel, s.network.MTU)
	if err := s.sb.Run(ctx); err != nil {
		return err
	}
	argv0 := ""
	if len(s.opts.Argv) > 0 {
		argv0 = sanitize.Display(s.opts.Argv[0], maxNameInEnv)
	}
	s.line(fmt.Sprintf("connected to %s; running %s", s.name, argv0))
	dns := s.opts.DNS
	if len(dns) == 0 {
		dns = s.network.DNSServers
	}
	s.log.Info("the application runs", "profile", s.name, "addrs", s.network.Addrs, "dns", dns, "mtu", s.network.MTU)
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

// closeTunnel closes the tunnel and logs a failure.
func (s *Session) closeTunnel() {
	if err := s.opts.Tunnel.Close(); err != nil {
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

// watch says so when the VPN connection ends while the application runs.
// TODO(step 4.1): connect again.
func (s *Session) watch() {
	defer close(s.watchDone)
	select {
	case <-s.pump.Done():
	case <-s.stopWatch:
		return
	}
	select {
	case <-s.stopWatch:
		return // the application has ended: the end of the VPN is the session's own
	default:
	}
	err := s.pump.Err()
	s.line(fmt.Sprintf("the VPN connection ended (%s); the application has no network", reason(err)))
	s.log.Warn("the VPN connection ended", "err", err)
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
	st, waitErr := s.sb.Wait()
	close(s.stopWatch)
	<-s.watchDone
	c := s.pump.Stop()
	s.log.Info("the application ended", "status", st.Status(), "to_tunnel_packets", c.ToTunnel.Packets, "to_tunnel_bytes", c.ToTunnel.Bytes,
		"from_tunnel_packets", c.FromTunnel.Packets, "from_tunnel_bytes", c.FromTunnel.Bytes, "dropped", c.Dropped)
	s.closeTunnel()
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
