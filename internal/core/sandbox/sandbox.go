// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

// Package sandbox is the namespace in which the application runs (docs/SPEC.md §8): Tube starts
// itself again as the keeper, in a new user, network and mount namespace; the keeper makes the
// TUN device and gives its descriptor to Tube, sets up the network and the names, and starts the
// application. Tube and the keeper talk over a socket pair.
package sandbox

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"golang.org/x/sys/unix"
)

// The defaults of Options.
const (
	defaultStepTimeout = 10 * time.Second
	defaultKillDelay   = 5 * time.Second
	selfExe            = "/proc/self/exe"
)

// The Tags of Tube's side of the namespace.
const (
	TagStartCancelled      = "SandboxStartCancelled"
	TagSocketPairFailed    = "SandboxSocketPairFailed"
	TagSocketFailed        = "SandboxSocketFailed"
	TagUserNamespaceDenied = "SandboxUserNamespaceDenied"
	TagKeeperStartFailed   = "SandboxKeeperStartFailed"
	TagTunUnavailable      = "SandboxTunUnavailable"
	TagKeeperFailed        = "SandboxKeeperFailed"
	TagProtocolError       = "SandboxProtocolError"
	TagKeeperExit          = "SandboxKeeperExit"
	TagKeeperSilent        = "SandboxKeeperSilent"
	TagCancelled           = "SandboxCancelled"
	TagConfigureFailed     = "SandboxConfigureFailed"
	TagAppNotFound         = "SandboxAppNotFound"
	TagAppNotExecutable    = "SandboxAppNotExecutable"
	TagRunFailed           = "SandboxRunFailed"
	TagKeeperLost          = "SandboxKeeperLost"
)

// replyBuffer is how many answers of the keeper wait for their request. One request is in
// flight at a time; the room is for late answers, which are dropped when they are read.
const replyBuffer = 8

// Options say how the namespace is started.
type Options struct {
	// Path is the application, as an absolute path; "" for a check (docs/SPEC.md §8.8).
	Path string
	// Argv is the application's argv, argv[0] included.
	Argv []string
	// Env is the environment of the keeper and the application; nil is os.Environ().
	Env []string
	// Dir is the working folder; "" is Tube's.
	Dir string
	// Stdin, Stdout and Stderr are the streams of the keeper and the application; nil is
	// Tube's own.
	Stdin, Stdout, Stderr *os.File
	// Exe is the binary to start as the keeper; "" is /proc/self/exe.
	Exe string
	// StepTimeout is how long the keeper has to answer; 0 is 10 seconds.
	StepTimeout time.Duration
	// KillDelay is the time between SIGTERM and SIGKILL to the application; 0 is 5 seconds.
	KillDelay time.Duration
	// TempDir is a folder of Tube's for the keeper's files (a resolv.conf that is mounted
	// over the machine's); "" when there is none, which works only on a machine whose
	// resolv.conf leads into a folder that the keeper covers (docs/SPEC.md §8.4).
	TempDir string
	// resolvConf replaces /etc/resolv.conf for the keeper; tests set it.
	resolvConf string
	// startCmd starts the keeper; nil is (*exec.Cmd).Start. Tests replace it.
	startCmd func(*exec.Cmd) error
}

// Sandbox is a running keeper and the device it made.
type Sandbox struct {
	opts   Options
	cmd    *exec.Cmd
	conn   *net.UnixConn
	device *os.File
	// exited is closed when the keeper has been waited for; waitErr is the result.
	exited  chan struct{}
	waitErr error

	// The reader hands the answers of the keeper to replies, closes appExited when the
	// application has ended (status says how), and closes gone with readErr when the socket
	// can no longer be read.
	replies   chan reply
	appExited chan struct{}
	status    ExitStatus
	gone      chan struct{}
	readErr   error

	// reqMu lets one request be in flight; seq is the number of the last one.
	reqMu sync.Mutex
	seq   int

	closeOnce sync.Once
	closeErr  error
}

// Start starts the keeper in new namespaces and waits until it has made the device.
func Start(ctx context.Context, opts Options) (*Sandbox, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr(TagStartCancelled, err)
	}
	if opts.StepTimeout <= 0 {
		opts.StepTimeout = defaultStepTimeout
	}
	if opts.KillDelay <= 0 {
		opts.KillDelay = defaultKillDelay
	}
	if opts.startCmd == nil {
		opts.startCmd = (*exec.Cmd).Start
	}
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, errors.NewErr(TagSocketPairFailed, err)
	}
	parent := os.NewFile(uintptr(fds[0]), "tube-socket")
	child := os.NewFile(uintptr(fds[1]), "tube-keeper-socket")
	// The child's end is closed when the keeper has its copy, or when the start failed.
	defer func() { _ = child.Close() }()
	c, err := net.FileConn(parent)
	// The connection has its own copy of the descriptor.
	_ = parent.Close()
	if err != nil {
		return nil, errors.NewErr(TagSocketFailed, err)
	}
	// A socket of AF_UNIX is always a *net.UnixConn.
	conn := c.(*net.UnixConn)

	s := &Sandbox{
		opts: opts, conn: conn, exited: make(chan struct{}),
		replies: make(chan reply, replyBuffer), appExited: make(chan struct{}), gone: make(chan struct{}),
	}
	s.cmd = keeperCommand(opts, child)
	if err := opts.startCmd(s.cmd); err != nil {
		// The start's error is the one that matters.
		_ = conn.Close()
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOSPC) ||
			errors.Is(err, syscall.EUSERS) || errors.Is(err, syscall.EINVAL) {
			return nil, errors.NewErr(TagUserNamespaceDenied, err).SetMetadata("reason", "the kernel refused a new user namespace")
		}
		return nil, errors.NewErr(TagKeeperStartFailed, err).SetMetadata("exe", s.cmd.Path)
	}
	go func() {
		s.waitErr = s.cmd.Wait()
		close(s.exited)
	}()
	go s.read()
	if err := s.awaitReady(ctx); err != nil {
		// The error of the start is the one that matters.
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// keeperCommand is the command that starts the keeper in new user, network and mount
// namespaces, where the user's uid and gid are root, with the child's end of the socket as
// its descriptor 3.
func keeperCommand(opts Options, child *os.File) *exec.Cmd {
	exe := opts.Exe
	if exe == "" {
		exe = selfExe
	}
	cmd := exec.Command(exe)
	cmd.Args = []string{KeeperArg0}
	if opts.Path != "" {
		cmd.Args = append(cmd.Args, opts.Path)
		cmd.Args = append(cmd.Args, opts.Argv...)
	}
	cmd.Env = keeperEnv(opts)
	cmd.Dir = opts.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}
	cmd.ExtraFiles = []*os.File{child}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	return cmd
}

// keeperEnv is the keeper's environment: the one of the options, or Tube's, without any
// variable that only the keeper reads, and with the ones that Start sets.
func keeperEnv(opts Options) []string {
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	env = withoutKeeperVars(env)
	if opts.TempDir != "" {
		env = append(env, keeperTmpVar+"="+opts.TempDir)
	}
	if opts.resolvConf != "" {
		env = append(env, keeperResolvVar+"="+opts.resolvConf)
	}
	return env
}

// reply is an answer of the keeper and the descriptor that came with it, or -1.
type reply struct {
	m  message
	fd int
}

// read reads every message of the keeper until the socket closes: the exited message closes
// appExited, and every other one goes to replies. Only a ready message keeps its descriptor.
func (s *Sandbox) read() {
	defer close(s.gone)
	for {
		m, fd, err := receive(s.conn)
		if err != nil {
			s.readErr = err
			return
		}
		if fd >= 0 && m.Type != msgReady {
			_ = unix.Close(fd) // only the device is ever wanted
			fd = -1
		}
		if m.Type == msgExited {
			select {
			case <-s.appExited: // a second one changes nothing
			default:
				s.status = ExitStatus{Code: m.Code, Signal: syscall.Signal(m.Signal)}
				close(s.appExited)
			}
			continue
		}
		select {
		case s.replies <- reply{m, fd}:
		default:
			// Nobody can be waiting for this many answers.
			if fd >= 0 {
				_ = unix.Close(fd)
			}
		}
	}
}

// exchange sends the request out (none when it is nil) and waits, for the step's timeout, for
// its answer: a message of the type want, or failed. Only a ready message comes with a
// descriptor, and it always does.
func (s *Sandbox) exchange(ctx context.Context, out *message, want string) (message, int, error) {
	s.reqMu.Lock()
	defer s.reqMu.Unlock()
	seq := 0
	if out != nil {
		s.seq++
		seq = s.seq
		out.Seq = seq
		if err := send(s.conn, *out, -1); err != nil {
			return message{}, -1, err
		}
	}
	timer := time.NewTimer(s.opts.StepTimeout)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return message{}, -1, errors.NewErr(TagCancelled, err).SetMetadata("waiting_for", want)
		}
		var r reply
		select {
		case r = <-s.replies:
		case <-s.gone:
			// An answer sent just before the socket closed is still the answer.
			select {
			case r = <-s.replies:
			default:
				return message{}, -1, s.readErr
			}
		case <-timer.C:
			return message{}, -1, errors.New(TagKeeperSilent, "the keeper did not answer in time", "waiting_for", want, "timeout", s.opts.StepTimeout.String())
		case <-ctx.Done():
			continue
		}
		if r.m.Seq != seq {
			if r.fd >= 0 {
				_ = unix.Close(r.fd) // a late answer to an earlier request
			}
			continue
		}
		if r.m.Type == msgFailed || (r.m.Type == want && (r.fd >= 0) == (want == msgReady)) {
			return r.m, r.fd, nil
		}
		if r.fd >= 0 {
			_ = unix.Close(r.fd) // a message that was not expected has no descriptor to keep
		}
		return message{}, -1, errors.New(TagProtocolError, "the keeper sent a message that was not expected", "type", r.m.Type, "waiting_for", want, "with_descriptor", r.fd >= 0)
	}
}

// keeperMeta is the metadata of an error made of the keeper's failed message: its Tag, its
// route, its text, and its own metadata with the prefix keeper_.
func keeperMeta(m message) []any {
	meta := []any{"keeper_tag", m.Tag, "keeper_route", m.Route, "text", m.Text}
	for k, v := range m.Meta {
		meta = append(meta, "keeper_"+k, v)
	}
	return meta
}

// awaitReady waits for the keeper's ready message and the device that comes with it.
func (s *Sandbox) awaitReady(ctx context.Context) error {
	m, fd, err := s.exchange(ctx, nil, msgReady)
	if err != nil {
		return err
	}
	switch {
	case m.Type == msgReady:
		s.device = os.NewFile(uintptr(fd), deviceName)
		return nil
	case m.Tag == TagTunOpenFailed || m.Tag == TagTunSetupFailed:
		return errors.New(TagTunUnavailable, "the TUN device cannot be made", "keeper_tag", m.Tag, "text", m.Text)
	}
	return errors.New(TagKeeperFailed, "the keeper failed", keeperMeta(m)...)
}

// Configure sets, or sets again, the network of the namespace: the tunnel's addresses and MTU,
// the default routes, and the DNS servers (dns when it is not empty, else the ones of the
// network) with the network's search domains. The warnings say why names will not be found.
func (s *Sandbox) Configure(ctx context.Context, n vpn.Network, dns []netip.Addr) (warnings []string, err error) {
	nm := networkMessage{MTU: n.MTU, Search: n.SearchDomains}
	for _, p := range n.Addrs {
		nm.Addrs = append(nm.Addrs, p.String())
	}
	if len(dns) == 0 {
		dns = n.DNSServers
	}
	for _, a := range dns {
		nm.DNS = append(nm.DNS, a.String())
	}
	return s.configure(ctx, nm)
}

// configure sends the network as the keeper gets it.
func (s *Sandbox) configure(ctx context.Context, nm networkMessage) ([]string, error) {
	m, _, err := s.exchange(ctx, &message{Type: msgConfigure, Network: &nm}, msgConfigured)
	if err != nil {
		return nil, err
	}
	if m.Type == msgFailed {
		return nil, errors.New(TagConfigureFailed, "the network of the namespace cannot be set", keeperMeta(m)...)
	}
	return m.Warnings, nil
}

// Run starts the application, as the user who runs Tube, and returns when it has started.
func (s *Sandbox) Run(ctx context.Context) error {
	run := message{Type: msgRun, UID: os.Getuid(), GID: os.Getgid(), KillMS: int(s.opts.KillDelay / time.Millisecond)}
	m, _, err := s.exchange(ctx, &run, msgStarted)
	if err != nil {
		return err
	}
	switch {
	case m.Type == msgStarted:
		return nil
	case m.Tag == TagKeeperAppNotFound:
		return errors.New(TagAppNotFound, "the application was not found", keeperMeta(m)...)
	case m.Tag == TagKeeperAppNotExecutable:
		return errors.New(TagAppNotExecutable, "the application cannot be executed", keeperMeta(m)...)
	}
	return errors.New(TagRunFailed, "the application cannot be started", keeperMeta(m)...)
}

// Wait waits until the application has ended and says how it ended. It fails when the keeper
// ends first (or the sandbox is closed).
func (s *Sandbox) Wait() (ExitStatus, error) {
	select {
	case <-s.appExited:
		return s.status, nil
	case <-s.gone:
	}
	// The reader closes appExited before it reads the end of the socket.
	select {
	case <-s.appExited:
		return s.status, nil
	default:
	}
	return ExitStatus{}, errors.New(TagKeeperLost, "the keeper ended before the application did")
}

// Device is the TUN device: one IP packet for each Read and each Write. Tube holds the only
// reference to it.
func (s *Sandbox) Device() *os.File { return s.device }

// Close ends the namespace: it closes the socket and the device, and waits for the keeper,
// which ends when its socket closes; one that does not end within the step's timeout is
// killed. A second call returns what the first returned.
func (s *Sandbox) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.close() })
	return s.closeErr
}

// close is Close, once.
func (s *Sandbox) close() error {
	// Closing is all that is asked of these: their own failures change nothing. The keeper
	// ends the application when its socket closes.
	_ = s.conn.Close()
	<-s.gone
	if s.device != nil {
		_ = s.device.Close()
	}
	killed := false
	select {
	case <-s.exited:
	case <-time.After(s.opts.StepTimeout + s.opts.KillDelay):
		killed = true
		// The wait below says when it has ended; a kill of a process that just ended fails.
		_ = s.cmd.Process.Kill()
		<-s.exited
	}
	if s.waitErr != nil && !killed {
		return errors.NewErr(TagKeeperExit, s.waitErr)
	}
	return nil
}

// Check starts a keeper that only makes the namespace and the device, and ends it: it says
// whether this machine allows what Tube needs (docs/SPEC.md §8.8).
func Check(ctx context.Context) error {
	s, err := Start(ctx, Options{})
	if err != nil {
		return err
	}
	return s.Close()
}
