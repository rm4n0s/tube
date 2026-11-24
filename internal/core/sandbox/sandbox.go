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
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/rm4n0s/errors"
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
)

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

	s := &Sandbox{opts: opts, conn: conn, exited: make(chan struct{})}
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
	cmd.Env = opts.Env
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

// deadline is when the current step must be answered: the step's timeout, or the context's
// deadline when that comes first.
func (s *Sandbox) deadline(ctx context.Context) time.Time {
	d := time.Now().Add(s.opts.StepTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(d) {
		return ctxDeadline
	}
	return d
}

// awaitReady waits for the keeper's ready message and the device that comes with it.
func (s *Sandbox) awaitReady(ctx context.Context) error {
	m, fd, err := receive(s.conn, s.deadline(ctx))
	if err != nil {
		return err
	}
	switch {
	case m.Type == msgReady && fd >= 0:
		s.device = os.NewFile(uintptr(fd), deviceName)
		return nil
	case m.Type == msgFailed && (m.Tag == TagTunOpenFailed || m.Tag == TagTunSetupFailed):
		return errors.New(TagTunUnavailable, "the TUN device cannot be made", "keeper_tag", m.Tag, "text", m.Text)
	case m.Type == msgFailed:
		meta := []any{"keeper_tag", m.Tag, "text", m.Text}
		for k, v := range m.Meta {
			meta = append(meta, "keeper_"+k, v)
		}
		if fd >= 0 {
			_ = unix.Close(fd) // a failed message has no descriptor to keep
		}
		return errors.New(TagKeeperFailed, "the keeper failed", meta...)
	}
	if fd >= 0 {
		_ = unix.Close(fd) // a message that was not expected has no descriptor to keep
	}
	return errors.New(TagProtocolError, "the keeper sent a message that was not expected", "type", m.Type, "with_descriptor", fd >= 0)
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
	// Closing is all that is asked of these: their own failures change nothing.
	_ = s.conn.Close()
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
