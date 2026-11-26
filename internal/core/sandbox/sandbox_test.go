// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"golang.org/x/sys/unix"
)

// fakeKeeperVar makes the test binary play a keeper that misbehaves in the named way.
const fakeKeeperVar = "TUBE_FAKE_KEEPER"

func TestMain(m *testing.M) {
	if role := os.Getenv(fakeKeeperVar); role != "" {
		os.Exit(fakeKeeper(role))
	}
	if IsKeeper() {
		os.Exit(KeeperMain())
	}
	sandboxtest.HandleProbe()
	nstest.Main(m, nil)
}

// fakeKeeper plays a keeper that misbehaves: it sends one message of the role and waits for
// the socket to close.
func fakeKeeper(role string) int {
	f := os.NewFile(keeperFD, "socket")
	c, err := net.FileConn(f)
	if err != nil {
		return 2
	}
	conn := c.(*net.UnixConn)
	status := 0
	switch role {
	case "protocol":
		if err := send(conn, message{Type: msgStarted}, -1); err != nil {
			return 2
		}
	case "failed":
		if err := send(conn, message{Type: msgFailed, Tag: "SomethingElse", Text: "it broke", Meta: map[string]string{"step": "7"}}, -1); err != nil {
			return 2
		}
	case "exit3":
		null, err := os.Open(os.DevNull)
		if err != nil {
			return 2
		}
		if err := send(conn, message{Type: msgReady}, int(null.Fd())); err != nil {
			return 2
		}
		status = 3
	case "garbage":
		if _, _, err := conn.WriteMsgUnix([]byte("not json"), nil, nil); err != nil {
			return 2
		}
	case "leave":
		// Ready, and then gone at the first request, without an answer.
		null, err := os.Open(os.DevNull)
		if err != nil {
			return 2
		}
		if err := send(conn, message{Type: msgReady}, int(null.Fd())); err != nil {
			return 2
		}
		if _, _, err := receive(conn); err != nil {
			return 2
		}
		return 0
	}
	// Wait for Tube to close its end.
	buf := make([]byte, 16)
	for {
		if n, err := conn.Read(buf); err != nil || n == 0 {
			return status
		}
	}
}

// closeLater closes c when the test ends, and fails the test when that fails.
func closeLater(t *testing.T, c io.Closer) {
	t.Helper()
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
}

// pair makes a connected socket pair of SOCK_SEQPACKET, closed when the test ends.
func pair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	var conns [2]*net.UnixConn
	for i, fd := range fds {
		f := os.NewFile(uintptr(fd), "pair")
		c, err := net.FileConn(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		conns[i] = c.(*net.UnixConn)
		t.Cleanup(func() { _ = conns[i].Close() }) // a test may have closed it already
	}
	return conns[0], conns[1]
}

func TestProtocol(t *testing.T) {
	t.Run("a message goes and comes back equal", func(t *testing.T) {
		a, b := pair(t)
		want := message{Type: msgFailed, Tag: "X", Text: "text", Meta: map[string]string{"k": "v"}}
		if err := send(a, want, -1); err != nil {
			t.Fatal(err)
		}
		got, fd, err := receive(b)
		if err != nil || fd != -1 || got.Type != want.Type || got.Tag != want.Tag || got.Text != want.Text || got.Meta["k"] != "v" {
			t.Errorf("receive = %+v, %d, %v", got, fd, err)
		}
	})
	t.Run("a descriptor goes with it", func(t *testing.T) {
		a, b := pair(t)
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, []byte("through the socket"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		closeLater(t, f)
		if err := send(a, message{Type: msgReady}, int(f.Fd())); err != nil {
			t.Fatal(err)
		}
		_, fd, err := receive(b)
		if err != nil || fd < 0 {
			t.Fatalf("receive = %d, %v", fd, err)
		}
		got := os.NewFile(uintptr(fd), "received")
		closeLater(t, got)
		data, err := io.ReadAll(got)
		if err != nil || string(data) != "through the socket" {
			t.Errorf("the received descriptor reads %q, %v", data, err)
		}
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Errorf("the received descriptor is not close-on-exec: %d, %v", flags, err)
		}
	})
	t.Run("the other end is closed", func(t *testing.T) {
		a, b := pair(t)
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		_, _, err := receive(b)
		errtest.Route(t, err, "receive.SandboxSocketClosed")
	})
	t.Run("this end is closed", func(t *testing.T) {
		_, b := pair(t)
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
		_, _, err := receive(b)
		errtest.Route(t, err, "receive.SandboxSocketClosed")
	})
	t.Run("bytes that are not JSON", func(t *testing.T) {
		a, b := pair(t)
		if _, _, err := a.WriteMsgUnix([]byte("not json"), nil, nil); err != nil {
			t.Fatal(err)
		}
		_, _, err := receive(b)
		meta := errtest.Route(t, err, "receive.SandboxMessageInvalid")
		if meta["reason"] != "the message is not JSON" {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("a message without a type, with a descriptor that is then closed", func(t *testing.T) {
		a, b := pair(t)
		f, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		closeLater(t, f)
		if _, _, err := a.WriteMsgUnix([]byte("{}"), unix.UnixRights(int(f.Fd())), nil); err != nil {
			t.Fatal(err)
		}
		_, fd, err := receive(b)
		errtest.Route(t, err, "receive.SandboxMessageInvalid")
		if fd != -1 {
			t.Errorf("the descriptor %d was given back", fd)
		}
	})
	t.Run("a message that is too large", func(t *testing.T) {
		a, _ := pair(t)
		err := send(a, message{Type: msgFailed, Text: strings.Repeat("x", maxMessage)}, -1)
		errtest.Route(t, err, "send.SandboxMessageTooLarge")
	})
	t.Run("a send to a closed end", func(t *testing.T) {
		a, b := pair(t)
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
		errtest.Route(t, send(a, message{Type: msgReady}, -1), "send.SandboxSendFailed")
	})
}

func TestOpenTun(t *testing.T) {
	t.Run("a device file that does not exist", func(t *testing.T) {
		_, err := openTun("/nonexistent/tun", deviceName)
		errtest.Route(t, err, "openTun.SandboxTunOpenFailed")
	})
	t.Run("a name that is too long", func(t *testing.T) {
		_, err := openTun(tunPath, "a-name-that-is-too-long-for-a-link")
		meta := errtest.Route(t, err, "openTun.SandboxTunSetupFailed")
		if meta["path"] != tunPath {
			t.Errorf("metadata = %v", meta)
		}
	})
}

// processGone says whether the process pid has ended and been waited for.
func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// start starts a sandbox with the options and fails the test when it cannot.
func start(t *testing.T, opts Options) *Sandbox {
	t.Helper()
	nstest.Require(t)
	s, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() }) // the test closes it itself when it checks the result
	return s
}

func TestStartGivesTheDevice(t *testing.T) {
	s := start(t, Options{})
	if s.Device() == nil {
		t.Fatal("no device")
	}
	// The device is down: a write fails or is dropped, but it is a device.
	_, _ = s.Device().Write([]byte{0x45, 0, 0, 20, 0, 0, 0, 0, 64, 1, 0, 0, 10, 9, 0, 2, 10, 9, 0, 1})
	pid := s.cmd.Process.Pid
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !processGone(pid) {
		t.Error("the keeper still runs after Close")
	}
	if err := s.Close(); err != nil {
		t.Errorf("a second Close: %v", err)
	}
}

func TestTheNamespaceIsNotTheMachines(t *testing.T) {
	s := start(t, Options{})
	if _, err := net.InterfaceByName(deviceName); err == nil {
		t.Error("tube0 is a link of the machine")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownMessage(t *testing.T) {
	s := start(t, Options{})
	m, _, err := s.exchange(context.Background(), &message{Type: "nonsense"}, "nothing")
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != msgFailed || m.Tag != TagKeeperUnknownMessage || m.Meta["type"] != "nonsense" {
		t.Errorf("the answer is %+v", m)
	}
}

func TestKeeperSilent(t *testing.T) {
	nstest.Require(t)
	script := filepath.Join(t.TempDir(), "silent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var pid int
	opts := Options{Exe: script, StepTimeout: 300 * time.Millisecond, KillDelay: 100 * time.Millisecond}
	opts.startCmd = func(cmd *exec.Cmd) error {
		err := cmd.Start()
		if err == nil {
			pid = cmd.Process.Pid
		}
		return err
	}
	_, err := Start(context.Background(), opts)
	errtest.Route(t, err, "Start->Sandbox.awaitReady->Sandbox.exchange.SandboxKeeperSilent")
	if pid == 0 || !processGone(pid) {
		t.Errorf("the silent keeper (pid %d) still runs", pid)
	}
}

func TestKeeperStartFailed(t *testing.T) {
	_, err := Start(context.Background(), Options{Exe: "/nonexistent/tube"})
	meta := errtest.Route(t, err, "Start.SandboxKeeperStartFailed")
	if meta["exe"] != "/nonexistent/tube" {
		t.Errorf("metadata = %v", meta)
	}
}

func TestUserNamespaceDenied(t *testing.T) {
	opts := Options{}
	opts.startCmd = func(*exec.Cmd) error {
		return &os.PathError{Op: "fork/exec", Path: selfExe, Err: syscall.EPERM}
	}
	_, err := Start(context.Background(), opts)
	errtest.Route(t, err, "Start.SandboxUserNamespaceDenied")
}

func TestStartCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Start(ctx, Options{})
	errtest.Route(t, err, "Start.SandboxStartCancelled")
}

func TestTunUnavailable(t *testing.T) {
	nstest.Require(t)
	_, err := Start(context.Background(), Options{Env: append(os.Environ(), tunDeviceVar+"=/nonexistent/tun")})
	meta := errtest.Route(t, err, "Start->Sandbox.awaitReady.SandboxTunUnavailable")
	if meta["keeper_tag"] != TagTunOpenFailed {
		t.Errorf("metadata = %v", meta)
	}
}

func TestMisbehavingKeepers(t *testing.T) {
	nstest.Require(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fake := func(role string) Options {
		return Options{Exe: exe, Env: append(os.Environ(), fakeKeeperVar+"="+role), StepTimeout: 5 * time.Second}
	}
	t.Run("a message that is not expected", func(t *testing.T) {
		_, err := Start(context.Background(), fake("protocol"))
		meta := errtest.Route(t, err, "Start->Sandbox.awaitReady->Sandbox.exchange.SandboxProtocolError")
		if meta["type"] != msgStarted {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("another failure of the keeper", func(t *testing.T) {
		_, err := Start(context.Background(), fake("failed"))
		meta := errtest.Route(t, err, "Start->Sandbox.awaitReady.SandboxKeeperFailed")
		if meta["keeper_tag"] != "SomethingElse" || meta["text"] != "it broke" || meta["keeper_step"] != "7" {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("a keeper that exits with a failure", func(t *testing.T) {
		s, err := Start(context.Background(), fake("exit3"))
		if err != nil {
			t.Fatal(err)
		}
		errtest.Route(t, s.Close(), "Sandbox.close.SandboxKeeperExit")
	})
}

// The reader of the keeper's messages runs in a goroutine of its own, and the error that ends
// it is returned by the request that waits for an answer, in another goroutine. Its route goes
// on from the Start that started the reader, as if Start had called read itself
// (errors.Async), and has nothing of the goroutine in it.
func TestTheErrorOfTheReaderStartsAtStart(t *testing.T) {
	nstest.Require(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		role string
		// fail makes the keeper end the reading, and returns the error of the request.
		fail      func(t *testing.T, opts Options) error
		wantRoute string
	}{
		{
			name: "a first message that cannot be read ends the start",
			role: "garbage",
			fail: func(_ *testing.T, opts Options) error {
				_, err := Start(context.Background(), opts)
				return err
			},
			wantRoute: "TestTheErrorOfTheReaderStartsAtStart.func1->Start->Sandbox.read->receive.SandboxMessageInvalid",
		},
		{
			name: "a keeper that leaves while a request waits for its answer",
			role: "leave",
			fail: func(t *testing.T, opts Options) error {
				s, err := Start(context.Background(), opts)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = s.Close() }) // the keeper has ended; its status does not matter
				_, err = s.Configure(context.Background(), vpn.Network{MTU: 1400}, nil)
				return err
			},
			wantRoute: "TestTheErrorOfTheReaderStartsAtStart.func2->Start->Sandbox.read->receive.SandboxSocketClosed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{Exe: exe, Env: append(os.Environ(), fakeKeeperVar+"="+tt.role), StepTimeout: 5 * time.Second}
			err := tt.fail(t, opts)
			errtest.Route(t, err, tt.wantRoute)
			appErr, _ := errors.FromError(err)
			if route := appErr.Route(); strings.Contains(route, "->Start.func") {
				t.Errorf("the route has the goroutine of the reader in it: %s", route)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	nstest.Require(t)
	if err := Check(context.Background()); err != nil {
		t.Errorf("Check: %v", err)
	}
}

func TestIsKeeper(t *testing.T) {
	if IsKeeper() {
		t.Error("the test process is taken for the keeper")
	}
}
