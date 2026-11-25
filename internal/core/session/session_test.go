// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/packettest"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/storage"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// The variables of the helper process of TestNoNetworkAfterTubeIsKilled: it holds a session
// until it is killed.
const (
	holderVar    = "TUBE_SESSION_HOLDER"
	holderDirVar = "TUBE_SESSION_HOLDER_DIR"
)

func TestMain(m *testing.M) {
	if sandbox.IsKeeper() {
		os.Exit(sandbox.KeeperMain())
	}
	sandboxtest.HandleProbe()
	if os.Getenv(holderVar) == "1" {
		os.Exit(holdSession())
	}
	nstest.Main(m, nil)
}

// holdSession runs a session with a fake tunnel and the probe linger, with a kill delay of 3
// seconds, until the process is killed.
func holdSession() int {
	exe, err := os.Executable()
	if err != nil {
		return 2
	}
	s, err := Start(context.Background(), Options{
		Tunnel: vpntest.NewTunnel(network), Dirs: storage.Dirs{TempBase: os.Getenv(holderDirVar)},
		Path: exe, Argv: []string{"probe"}, ProfileName: "held",
		ExtraEnv: []string{sandboxtest.ProbeEnv("linger"), holderVar + "=0"},
		Sandbox:  sandbox.Options{KillDelay: 3 * time.Second},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	status, err := s.Wait()
	if err != nil {
		return 1
	}
	return status
}

// network is the network of the fake tunnels: the server is 10.9.0.1, and it gives a DNS
// server, so that no warning is written.
var network = vpn.Network{
	Addrs: []netip.Prefix{netip.MustParsePrefix("10.9.0.2/24")}, DNSServers: []netip.Addr{netip.MustParseAddr("10.9.0.1")}, MTU: 1400,
}

// pingTunnel is a fake tunnel that answers echo requests.
func pingTunnel(n vpn.Network) *vpntest.Tunnel {
	tun := vpntest.NewTunnel(n)
	tun.OnWrite(func(packet []byte) [][]byte {
		if packettest.IsEchoRequest4(packet) {
			return [][]byte{packettest.EchoReply4(packet)}
		}
		return nil
	})
	return tun
}

// lookPath finds a program, and skips the test when it is not installed.
func lookPath(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed", name)
	}
	return path
}

// probe is the path and argv of the test binary as the application.
func probe(t *testing.T, args ...string) (string, []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe, append([]string{"probe"}, args...)
}

// run is a session of a test and the files of the application's output.
type run struct {
	s              *Session
	dirs           storage.Dirs
	stdout, stderr string
}

// read reads a file of the test.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// options are the options of a session of a test: isolated folders, the profile "work", the
// application's output in files of the test, a kill delay of 500 ms.
func options(t *testing.T, tun vpn.Tunnel, path string, argv []string) (Options, run) {
	t.Helper()
	nstest.Require(t)
	r := run{dirs: storagetest.Isolate(t)}
	dir := t.TempDir()
	r.stdout, r.stderr = filepath.Join(dir, "stdout"), filepath.Join(dir, "stderr")
	opts := Options{Tunnel: tun, Dirs: r.dirs, Path: path, Argv: argv, ProfileName: "work", Sandbox: sandbox.Options{KillDelay: 500 * time.Millisecond}}
	for i, name := range []string{r.stdout, r.stderr} {
		f, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() }) // only written by the application
		if i == 0 {
			opts.Sandbox.Stdout = f
		} else {
			opts.Sandbox.Stderr = f
		}
	}
	return opts, r
}

// start starts the session of the options; it is waited for when the test ends.
func start(t *testing.T, opts Options, r run) run {
	t.Helper()
	s, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Signal(syscall.SIGKILL)
		_, _ = s.Wait() // the test checks the result itself when it matters
	})
	r.s = s
	return r
}

// wait waits for the end of the application and returns the exit status.
func (r run) wait(t *testing.T) int {
	t.Helper()
	status, err := r.s.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return status
}

// noTempLeft fails the test when a tube-* folder is left in the temporary base.
func noTempLeft(t *testing.T, dirs storage.Dirs) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dirs.TempBase, "tube-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Errorf("temporary folders are left: %v", left)
	}
}

// noChildrenLeft fails the test when a child of the test process is left, after a short wait.
func noChildrenLeft(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		files, err := filepath.Glob("/proc/self/task/*/children")
		if err != nil {
			t.Fatal(err)
		}
		var pids []string
		for _, f := range files {
			if data, err := os.ReadFile(f); err == nil {
				pids = append(pids, strings.Fields(string(data))...)
			}
		}
		if len(pids) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("children of the test process are left: %v", pids)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestARunFromStartToEnd(t *testing.T) {
	tun := pingTunnel(network)
	opts, r := options(t, tun, lookPath(t, "ping"), []string{"ping", "-c", "1", "-W", "2", "10.9.0.1"})
	r = start(t, opts, r)
	if status := r.wait(t); status != 0 {
		t.Errorf("the status is %d:\n%s%s", status, read(t, r.stdout), read(t, r.stderr))
	}
	if !tun.Closed() {
		t.Error("the tunnel is not closed")
	}
	noTempLeft(t, r.dirs)
	noChildrenLeft(t)
}

func TestTheExitStatus(t *testing.T) {
	sh := lookPath(t, "sh")
	for script, want := range map[string]int{"exit 5": 5, "kill -KILL $$": 137} {
		t.Run(script, func(t *testing.T) {
			opts, r := options(t, pingTunnel(network), sh, []string{"sh", "-c", script})
			if status := start(t, opts, r).wait(t); status != want {
				t.Errorf("the status is %d, want %d", status, want)
			}
		})
	}
}

func TestTheEnvironment(t *testing.T) {
	sh := lookPath(t, "sh")
	script := `echo $TUBE $TUBE_PROFILE $MOZ_NO_REMOTE; env | grep -c '^TUBE_PROFILE='`
	for _, outside := range []string{"", "other"} {
		t.Run("TUBE_PROFILE="+outside, func(t *testing.T) {
			if outside != "" {
				t.Setenv("TUBE_PROFILE", outside)
			}
			opts, r := options(t, pingTunnel(network), sh, []string{"sh", "-c", script})
			opts.ExtraEnv = []string{"TUBE_TEST_EXTRA=1"}
			r = start(t, opts, r)
			if status := r.wait(t); status != 0 {
				t.Fatalf("the status is %d", status)
			}
			if out := read(t, r.stdout); out != "1 work 1\n1\n" {
				t.Errorf("the application printed %q", out)
			}
		})
	}
}

func TestTheLines(t *testing.T) {
	sh := lookPath(t, "sh")
	t.Run("connected", func(t *testing.T) {
		var lines bytes.Buffer
		opts, r := options(t, pingTunnel(network), sh, []string{"sh", "-c", "exit 0"})
		opts.Lines = &lines
		start(t, opts, r).wait(t)
		if got := lines.String(); got != "tube: connected to work; running sh\n" {
			t.Errorf("the lines are %q", got)
		}
	})
	t.Run("the notes come after the first line", func(t *testing.T) {
		var lines bytes.Buffer
		opts, r := options(t, pingTunnel(network), sh, []string{"sh", "-c", "exit 0"})
		opts.Lines = &lines
		opts.Notes = []string{"sh runs on a profile of its own", "and a second note"}
		start(t, opts, r).wait(t)
		if got, want := lines.String(), "tube: connected to work; running sh\ntube: sh runs on a profile of its own\ntube: and a second note\n"; got != want {
			t.Errorf("the lines are %q, want %q", got, want)
		}
	})
	t.Run("no DNS server", func(t *testing.T) {
		var lines bytes.Buffer
		n := network
		n.DNSServers = nil
		opts, r := options(t, pingTunnel(n), sh, []string{"sh", "-c", "exit 0"})
		opts.Lines = &lines
		start(t, opts, r).wait(t)
		if got := lines.String(); !strings.Contains(got, "tube: "+"the VPN gave no DNS server") {
			t.Errorf("the lines are %q", got)
		}
	})
	t.Run("none", func(t *testing.T) {
		n := network
		n.DNSServers = nil
		opts, r := options(t, pingTunnel(n), sh, []string{"sh", "-c", "exit 0"})
		start(t, opts, r).wait(t)
		if out := read(t, r.stderr) + read(t, r.stdout); out != "" {
			t.Errorf("something was written: %q", out)
		}
	})
}

func TestStartFailsCleanly(t *testing.T) {
	tun := pingTunnel(network)
	opts, r := options(t, tun, "/nonexistent/app", []string{"app"})
	_, err := Start(context.Background(), opts)
	if err == nil {
		t.Fatal("Start of an application that does not exist did not fail")
	}
	if code := sandbox.ExitCodeOf(err); code != 127 {
		t.Errorf("ExitCodeOf = %d (%v)", code, err)
	}
	if !tun.Closed() {
		t.Error("the tunnel is not closed")
	}
	noTempLeft(t, r.dirs)
	noChildrenLeft(t)
}

func TestTheTunnelFailsWhileTheApplicationRuns(t *testing.T) {
	var lines syncBuffer
	tun := pingTunnel(network)
	opts, r := options(t, tun, lookPath(t, "sleep"), []string{"sleep", "1"})
	opts.Lines = &lines
	r = start(t, opts, r)
	tun.Fail(io.EOF)
	if status := r.wait(t); status != 0 {
		t.Errorf("the status is %d", status)
	}
	want := "tube: the VPN connection ended (the server closed the connection); the application has no network\n"
	if got := lines.String(); !strings.Contains(got, want) {
		t.Errorf("the lines are %q", got)
	}
}

func TestSignal(t *testing.T) {
	opts, r := options(t, pingTunnel(network), lookPath(t, "sleep"), []string{"sleep", "30"})
	r = start(t, opts, r)
	time.Sleep(100 * time.Millisecond)
	r.s.Signal(syscall.SIGTERM)
	if status := r.wait(t); status != 143 {
		t.Errorf("the status is %d", status)
	}
	r.s.Signal(syscall.SIGTERM) // after the end: nothing happens
}

func TestReason(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{io.EOF, "the server closed the connection"},
		{context.Canceled, "it was cancelled"},
		{os.ErrClosed, "it failed"},
	}
	for _, tt := range tests {
		if got := reason(tt.err); got != tt.want {
			t.Errorf("reason(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}
