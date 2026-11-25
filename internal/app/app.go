// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

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
	TagNoProfileOrFile  = "AppNoProfileOrFile"
	TagProfileAmbiguous = "AppProfileAmbiguous"
	TagProfileBroken    = "AppProfileBroken"
	TagNotConnected     = "AppNotConnected"
	TagNoTarget         = "AppNoTarget"
)

// logFile is the name of the log in the state folder (docs/SPEC.md §15).
const logFile = "tube.log"

// maxNameInError is the length of a name in an error.
const maxNameInError = 64

// maxPathInError is the length of a path in an error: its end is what tells the files apart.
const maxPathInError = 512

// Credentials are the user name and the password that the user gave.
type Credentials struct{ Username, Password string }

// Remember says what a connect keeps of the credentials that were given, when it worked
// (docs/SPEC.md §6.6). The zero value changes nothing: a script that gives the credentials
// with --auth-file does not change the profile.
type Remember struct {
	// User keeps the user name.
	User bool
	// Password keeps the password, in a file that only the user can read, and the user name
	// with it: the profile then connects without asking.
	Password bool
	// Forget forgets a password that was remembered before, when Password is not set: the
	// user was asked and did not want it kept.
	Forget bool
}

// String says what is kept, for the tests' records of calls.
func (r Remember) String() string {
	switch {
	case r.Password:
		return "keep the password"
	case r.User && r.Forget:
		return "keep the user name, forget the password"
	case r.User:
		return "keep the user name"
	case r.Forget:
		return "forget the password"
	}
	return "keep nothing"
}

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
	// tunnel is connected and not given to a session yet; profile is its profile, and creds
	// are the credentials of the connect, kept in memory only, for connecting again, until
	// Close.
	tunnel  vpn.Tunnel
	profile profiles.Profile
	creds   *vpn.Credentials
	// session runs the application; running is set while Run runs, and pending is a signal
	// that came before the session could take it.
	session *session.Session
	running bool
	pending syscall.Signal
	// runDNS are the DNS servers of this run only, in place of the profile's, when runDNSSet
	// (UseDNS); none of them are the ones of the VPN.
	runDNS    []netip.Addr
	runDNSSet bool
	// rd is what Start made and what the run changes.
	rd runData
	// tempDir is the folder of edits, made on first use; edits are the files that BeginEdit
	// gave, by profile.
	tempDir string
	edits   map[string]string

	// addFolder is the folder of the profile file that was added last, or "": the add page
	// starts there (state.go).
	stateMu   sync.Mutex
	addFolder string

	// subs are the subscribers to the changes of the profiles.
	subsMu  sync.Mutex
	subs    map[int]func()
	nextSub int
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
	a := &App{
		opts: opts, dirs: dirs, store: profiles.NewStore(dirs.Config, opts.Clock), log: log, logCloser: closer,
		edits: map[string]string{}, subs: map[int]func(){},
	}
	log.Info("tube started", "version", Version())
	a.loadState(ctx)
	return a, nil
}

// Close closes what is open: the application that Start started (Finish), a tunnel that was
// connected and never run, and the log. It forgets the credentials.
func (a *App) Close() error {
	if a.startedRun() != nil {
		if _, err := a.Finish(); err != nil {
			a.log.Warn("the application ended with an error", "err", err)
		}
	}
	a.mu.Lock()
	tun, tmp := a.tunnel, a.tempDir
	a.tunnel, a.creds, a.tempDir = nil, nil, ""
	a.mu.Unlock()
	if tun != nil {
		if err := tun.Close(); err != nil {
			a.log.Warn("a tunnel that was not used was closed with an error", "err", err)
		}
	}
	if tmp != "" {
		if err := os.RemoveAll(tmp); err != nil {
			a.log.Warn("the folder of the edits cannot be removed", "err", err)
		}
	}
	a.log.Info("tube ended")
	return a.logCloser.Close()
}

// Target is the application that will run: its name and arguments, or none.
func (a *App) Target() []string { return slices.Clone(a.opts.Argv) }

// Found is a profile that FindProfile found: its id and name, whether it asks for a user name
// and a password, whether the password is remembered, the remembered user name, and whether it
// has DNS servers of its own. Added says that FindOrAddProfile added it just now, from a file.
type Found struct {
	ID            string
	Name          string
	NeedsPassword bool
	HasPassword   bool
	Username      string
	HasDNS        bool
	Added         bool
}

// FindProfile finds a usable profile by its name, as --profile does (docs/SPEC.md §11): the one
// with exactly that name, else the only one that has it when the case is ignored.
func (a *App) FindProfile(ctx context.Context, name string) (Found, error) {
	list, err := a.store.List(ctx)
	if err != nil {
		return Found{}, err
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
		return Found{}, errors.New(TagProfileNotFound, "there is no profile with this name", "name", safe)
	case count > 1:
		return Found{}, errors.New(TagProfileAmbiguous, "more than one profile has this name when the case is ignored", "name", safe, "count", count)
	case found.Broken != "":
		return Found{}, errors.New(TagProfileBroken, "the profile cannot be used", "name", safe, "reason", found.Broken)
	}
	return Found{
		ID: found.ID, Name: found.Name, NeedsPassword: found.NeedsPassword, HasPassword: found.HasPassword,
		Username: found.Username, HasDNS: len(found.DNS) > 0,
	}, nil
}

// isNoProfile says whether the error is FindProfile's "there is no profile with this name".
func isNoProfile(err error) bool {
	appErr, ok := errors.FromError(err)
	return ok && appErr.Tag == TagProfileNotFound
}

// FindOrAddProfile finds the profile that --profile names (docs/SPEC.md §11), or adds it from
// a file:
//
//  1. a stored profile with that name (FindProfile), also when the name is written with
//     ".ovpn";
//  2. else a profile file at that path, as it is written or with ".ovpn" added ("~/" is the
//     home folder): the stored profile with the name of that file when there is one, and else
//     the file is added under that name, and Added is set.
//
// withoutScripts adds a file that runs scripts with those lines disabled; without it, such a
// file is the error of IsRunsScripts.
func (a *App) FindOrAddProfile(ctx context.Context, value string, withoutScripts bool) (Found, error) {
	found, err := a.FindProfile(ctx, value)
	if err == nil || !isNoProfile(err) {
		return found, err
	}
	if strings.HasSuffix(strings.ToLower(value), profileExt) && !strings.ContainsRune(value, filepath.Separator) {
		found, err := a.FindProfile(ctx, profileNameOf(value))
		if err == nil || !isNoProfile(err) {
			return found, err
		}
	}
	candidates := []string{value}
	if !strings.HasSuffix(strings.ToLower(value), profileExt) {
		candidates = append(candidates, value+profileExt)
	}
	for _, candidate := range candidates {
		path := a.expandHome(candidate)
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		found, err := a.FindProfile(ctx, profileNameOf(path))
		if err == nil || !isNoProfile(err) {
			return found, err
		}
		id, err := a.AddProfile(ctx, path, "", "", withoutScripts)
		if err != nil {
			return Found{}, err
		}
		p, err := a.store.Get(ctx, id)
		if err != nil {
			return Found{}, err
		}
		return Found{ID: p.ID, Name: p.Name, NeedsPassword: p.NeedsPassword, Username: p.Username, Added: true}, nil
	}
	return Found{}, errors.New(TagNoProfileOrFile, "there is no profile with this name and no profile file at this path", "name", sanitize.Display(value, maxPathInError))
}

// UseDNS sets the DNS servers of the application for this run only, in place of the ones of
// the profile, which stays as it is: text is IP addresses separated by spaces or commas, and
// empty text is the servers that the VPN pushes. It is called before the application is
// started.
func (a *App) UseDNS(text string) error {
	servers, err := profiles.ParseDNS(text)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.runDNS, a.runDNSSet = servers, true
	a.mu.Unlock()
	return nil
}

// Connect connects the profile. With remember, the user name of the credentials is saved as the
// profile's. After it returned nil, Run runs the application. A tunnel that was connected before
// is closed first.
func (a *App) Connect(ctx context.Context, id string, creds *Credentials, remember Remember) error {
	a.closeTunnel()
	err := a.connect(ctx, id, creds, remember)
	if err != nil {
		a.log.Error("connect failed", "err", err, "profile", id)
	}
	return err
}

// connect is Connect without the log of its failure.
func (a *App) connect(ctx context.Context, id string, creds *Credentials, remember Remember) error {
	d, err := a.dial(ctx, id, creds, remember)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.tunnel, a.profile, a.creds = d.tun, d.p, d.creds
	a.mu.Unlock()
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
	tk, err := a.takeTunnel(ctx)
	if err != nil {
		return sandbox.ExitCodeOf(err), err
	}
	tun, p, creds, path := tk.tun, tk.p, tk.creds, tk.path
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
		Tunnel: tun, Dirs: a.dirs, Path: path, Argv: tk.argv, Notes: notes(tk.note), ProfileName: p.Name, DNS: p.DNS,
		Logger: a.log, Lines: a.opts.Lines, Clock: a.opts.Clock, Redial: a.redialer(p.ID, creds),
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
		a.Signal(pending)
	}
	status, err := s.Wait()
	a.stopKillTimer()
	if err != nil {
		a.log.Error("run failed", "err", err, "profile", p.ID)
		return status, err
	}
	a.log.Info("run ended", "profile", p.ID, "status", status)
	return status, nil
}

// taken is what takeTunnel took: the tunnel, its profile and credentials, and the application's
// path.
type taken struct {
	tun   vpn.Tunnel
	p     profiles.Profile
	creds *vpn.Credentials
	path  string
	// argv is what the application is started with, and note a line for the user about it,
	// or "" (applicationArgv).
	argv []string
	note string
}

// notes are the lines of a session about how the application is started.
func notes(note string) []string {
	if note == "" {
		return nil
	}
	return []string{note}
}

// takeTunnel takes the connected tunnel for a run, and finds the application. When there is no
// tunnel, no target, or no such application, it fails; a tunnel that it took is then closed.
func (a *App) takeTunnel(ctx context.Context) (taken, error) {
	a.mu.Lock()
	tk := taken{tun: a.tunnel, p: a.profile, creds: a.creds}
	a.tunnel = nil
	if a.runDNSSet {
		tk.p.DNS = a.runDNS
	}
	a.mu.Unlock()
	if tk.tun == nil {
		return taken{}, errors.New(TagNotConnected, "no VPN is connected")
	}
	if len(a.opts.Argv) == 0 {
		a.closeTunnelOf(tk.tun)
		return taken{}, errors.New(TagNoTarget, "there is no application to run")
	}
	path, err := sandbox.LookPath(a.opts.Argv[0])
	if err != nil {
		a.closeTunnelOf(tk.tun)
		return taken{}, err
	}
	tk.path = path
	tk.argv, tk.note, err = a.applicationArgv(ctx)
	if err != nil {
		a.closeTunnelOf(tk.tun)
		return taken{}, err
	}
	return tk, nil
}

// redialTimeout is how long one try to connect again may take.
const redialTimeout = 60 * time.Second

// redialer is the Redial of a session: it connects the same profile again, with the credentials
// of the connect.
func (a *App) redialer(id string, creds *vpn.Credentials) func(ctx context.Context) (vpn.Tunnel, error) {
	return func(ctx context.Context) (vpn.Tunnel, error) {
		ctx, cancel := context.WithTimeout(ctx, redialTimeout)
		defer cancel()
		p, err := a.store.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return vpn.Dial(ctx, p, creds)
	}
}

// closeTunnelOf closes a tunnel that Run took and could not use.
func (a *App) closeTunnelOf(tun vpn.Tunnel) {
	if err := tun.Close(); err != nil {
		a.log.Warn("the tunnel was closed with an error", "err", err)
	}
}

// Signal passes a signal to the application, when one runs. A signal that comes while the
// application is being started is passed on as soon as it has started. SIGTERM and SIGHUP are
// followed by SIGKILL when the application has not ended five seconds later.
func (a *App) Signal(sig syscall.Signal) {
	a.mu.Lock()
	s := a.session
	if s == nil && a.running {
		a.pending = sig
	}
	if s != nil && (sig == syscall.SIGTERM || sig == syscall.SIGHUP) {
		a.killLater(s)
	}
	a.mu.Unlock()
	if s != nil {
		s.Signal(sig)
	}
}
