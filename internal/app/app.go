// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/logging"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/session"
	"github.com/rm4n0s/tube/internal/core/storage"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// The Tags of the App's own errors.
const (
	TagProfileNotFound  = "AppProfileNotFound"
	TagProfileAmbiguous = "AppProfileAmbiguous"
	TagProfileBroken    = "AppProfileBroken"
	TagNotConnected     = "AppNotConnected"
	TagNoTarget         = "AppNoTarget"
)

// logFile is the name of the log in the state folder (docs/SPEC.md §15).
const logFile = "tube.log"

// maxNameInError is the length of a name in an error.
const maxNameInError = 64

// Credentials are the user name and the password that the user gave.
type Credentials struct{ Username, Password string }

// Options say how the application is opened.
type Options struct {
	// Env looks up the environment; nil is os.LookupEnv.
	Env storage.Env
	// Argv is the application and its arguments; empty when Tube only keeps the profiles.
	Argv []string
	// Lines is where Tube's lines go while the application runs; nil for --quiet.
	Lines io.Writer
	// Clock is nil for the system's.
	Clock clock.Clock
}

// App is Tube behind its command line and its TUI: the folders, the store of profiles, the log,
// a connected tunnel, and the session that runs the application.
type App struct {
	opts      Options
	dirs      storage.Dirs
	store     *profiles.Store
	log       *slog.Logger
	logCloser io.Closer

	mu sync.Mutex
	// tunnel is connected and not given to a session yet; profile is its profile.
	tunnel  vpn.Tunnel
	profile profiles.Profile
	// session runs the application; running is set while Run runs, and pending is a signal
	// that came before the session could take it.
	session *session.Session
	running bool
	pending syscall.Signal
}

// Open resolves and makes the folders, opens the log and the store.
func Open(ctx context.Context, opts Options) (*App, error) {
	if opts.Env == nil {
		opts.Env = os.LookupEnv
	}
	if opts.Clock == nil {
		opts.Clock = clock.System()
	}
	dirs, err := storage.Resolve(opts.Env)
	if err != nil {
		return nil, err
	}
	if err := dirs.Ensure(ctx); err != nil {
		return nil, err
	}
	log, closer, err := logging.Open(ctx, filepath.Join(dirs.State, logFile), slog.LevelInfo)
	if err != nil {
		return nil, err
	}
	a := &App{opts: opts, dirs: dirs, store: profiles.NewStore(dirs.Config, opts.Clock), log: log, logCloser: closer}
	log.Info("tube started", "version", Version())
	return a, nil
}

// Close closes what is open: a tunnel that was connected and never run, and the log.
func (a *App) Close() error {
	a.mu.Lock()
	tun := a.tunnel
	a.tunnel = nil
	a.mu.Unlock()
	if tun != nil {
		if err := tun.Close(); err != nil {
			a.log.Warn("a tunnel that was not used was closed with an error", "err", err)
		}
	}
	a.log.Info("tube ended")
	return a.logCloser.Close()
}

// Target is the application that will run: its name and arguments, or none.
func (a *App) Target() []string { return slices.Clone(a.opts.Argv) }

// FindProfile finds a usable profile by its name, as --profile does (docs/SPEC.md §11): the one
// with exactly that name, else the only one that has it when the case is ignored.
func (a *App) FindProfile(ctx context.Context, name string) (id string, needsPassword bool, username string, err error) {
	list, err := a.store.List(ctx)
	if err != nil {
		return "", false, "", err
	}
	found, count := profiles.Profile{}, 0
	for _, p := range list {
		if p.Name == name {
			found, count = p, 1
			break
		}
		if strings.EqualFold(p.Name, name) {
			found = p
			count++
		}
	}
	safe := sanitize.Display(name, maxNameInError)
	switch {
	case count == 0:
		return "", false, "", errors.New(TagProfileNotFound, "there is no profile with this name", "name", safe)
	case count > 1:
		return "", false, "", errors.New(TagProfileAmbiguous, "more than one profile has this name when the case is ignored", "name", safe, "count", count)
	case found.Broken != "":
		return "", false, "", errors.New(TagProfileBroken, "the profile cannot be used", "name", safe, "reason", found.Broken)
	}
	return found.ID, found.NeedsPassword, found.Username, nil
}

// Connect connects the profile. With remember, the user name of the credentials is saved as the
// profile's. After it returned nil, Run runs the application. A tunnel that was connected before
// is closed first.
func (a *App) Connect(ctx context.Context, id string, creds *Credentials, remember bool) error {
	a.closeTunnel()
	err := a.connect(ctx, id, creds, remember)
	if err != nil {
		a.log.Error("connect failed", "err", err, "profile", id)
	}
	return err
}

// connect is Connect without the log of its failure.
func (a *App) connect(ctx context.Context, id string, creds *Credentials, remember bool) error {
	p, err := a.store.Get(ctx, id)
	if err != nil {
		return err
	}
	var vc *vpn.Credentials
	if creds != nil {
		vc = &vpn.Credentials{Username: creds.Username, Password: creds.Password}
	}
	tun, err := vpn.Dial(ctx, p, vc)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.tunnel, a.profile = tun, p
	a.mu.Unlock()
	n := tun.Network()
	a.log.Info("connected", "profile", p.ID, "name", p.Name, "addrs", n.Addrs, "dns", n.DNSServers, "mtu", n.MTU)
	// The connection is what the user asked for: these only change what the list shows.
	if err := a.store.MarkUsed(ctx, id); err != nil {
		a.log.Warn("the profile cannot be marked as used", "err", err, "profile", id)
	}
	if remember && creds != nil {
		if err := a.store.SetUsername(ctx, id, creds.Username); err != nil {
			a.log.Warn("the user name cannot be remembered", "err", err, "profile", id)
		}
	}
	return nil
}

// closeTunnel closes a tunnel that was connected and not given to a session.
func (a *App) closeTunnel() {
	a.mu.Lock()
	tun := a.tunnel
	a.tunnel = nil
	a.mu.Unlock()
	if tun != nil {
		if err := tun.Close(); err != nil {
			a.log.Warn("the tunnel was closed with an error", "err", err)
		}
	}
}

// setTunnel keeps a tunnel as if Connect had connected it: for tests.
func (a *App) setTunnel(tun vpn.Tunnel, p profiles.Profile) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tunnel, a.profile = tun, p
}

// Run runs the application on Tube's terminal through the connected VPN and returns its exit
// status (docs/SPEC.md §12.4). The tunnel belongs to the session from then on; when the
// application cannot be started, the tunnel is closed.
func (a *App) Run(ctx context.Context) (int, error) {
	a.mu.Lock()
	tun, p := a.tunnel, a.profile
	a.tunnel = nil
	a.mu.Unlock()
	if tun == nil {
		return sandbox.StatusTubeFailed, errors.New(TagNotConnected, "no VPN is connected")
	}
	if len(a.opts.Argv) == 0 {
		a.closeTunnelOf(tun)
		return sandbox.StatusTubeFailed, errors.New(TagNoTarget, "there is no application to run")
	}
	path, err := sandbox.LookPath(a.opts.Argv[0])
	if err != nil {
		a.closeTunnelOf(tun)
		return sandbox.ExitCodeOf(err), err
	}
	a.log.Info("run started", "profile", p.ID, "name", p.Name, "application", sanitize.Display(a.opts.Argv[0], maxNameInError))
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()
	s, err := session.Start(ctx, session.Options{
		Tunnel: tun, Dirs: a.dirs, Path: path, Argv: a.opts.Argv, ProfileName: p.Name, DNS: p.DNS,
		Logger: a.log, Lines: a.opts.Lines, Clock: a.opts.Clock,
	})
	if err != nil {
		// session.Start closed the tunnel.
		a.log.Error("run failed", "err", err, "profile", p.ID)
		return sandbox.ExitCodeOf(err), err
	}
	a.mu.Lock()
	a.session = s
	pending := a.pending
	a.mu.Unlock()
	if pending != 0 {
		s.Signal(pending)
	}
	status, err := s.Wait()
	if err != nil {
		a.log.Error("run failed", "err", err, "profile", p.ID)
		return status, err
	}
	a.log.Info("run ended", "profile", p.ID, "status", status)
	return status, nil
}

// closeTunnelOf closes a tunnel that Run took and could not use.
func (a *App) closeTunnelOf(tun vpn.Tunnel) {
	if err := tun.Close(); err != nil {
		a.log.Warn("the tunnel was closed with an error", "err", err)
	}
}

// Signal passes a signal to the application, when one runs. A signal that comes while the
// application is being started is passed on as soon as it has started.
func (a *App) Signal(sig syscall.Signal) {
	a.mu.Lock()
	s := a.session
	if s == nil && a.running {
		a.pending = sig
	}
	a.mu.Unlock()
	if s != nil {
		s.Signal(sig)
	}
}
