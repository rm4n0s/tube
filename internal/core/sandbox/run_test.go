// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/packettest"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"golang.org/x/sys/unix"
)

// network4 is the network of most tests: the peer is 10.9.0.1.
var network4 = vpn.Network{Addrs: []netip.Prefix{netip.MustParsePrefix("10.9.0.2/24")}, MTU: 1400}

// network46 has an IPv6 address too: the peer is fd00:9::1.
var network46 = vpn.Network{Addrs: []netip.Prefix{netip.MustParsePrefix("10.9.0.2/24"), netip.MustParsePrefix("fd00:9::2/64")}, MTU: 1400}

// running is a sandbox of a test with its peer and the files of the application's output.
type running struct {
	s              *Sandbox
	peer           *sandboxtest.Peer
	stdout, stderr string
}

// output is what the application wrote to its standard output.
func (r running) output(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(r.stdout)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// errors is what the application wrote to its standard error, for the messages of a failing
// test.
func (r running) errors() string {
	data, err := os.ReadFile(r.stderr)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

// startSandbox starts a sandbox for the application path with the argv, with the variables of
// a run (TUBE=1, TUBE_PROFILE, MOZ_NO_REMOTE=1) and env added; Close is called when the test
// ends. The application's output goes to files of the test.
func startSandbox(t *testing.T, path string, argv []string, env ...string) running {
	t.Helper()
	return startSandboxWith(t, nil, path, argv, env...)
}

// startSandboxWith is startSandbox, with options that change changes before the start.
func startSandboxWith(t *testing.T, change func(*Options), path string, argv []string, env ...string) running {
	t.Helper()
	nstest.Require(t)
	dir := t.TempDir()
	r := running{stdout: filepath.Join(dir, "stdout"), stderr: filepath.Join(dir, "stderr")}
	files := make([]*os.File, 2)
	for i, name := range []string{r.stdout, r.stderr} {
		f, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		closeLater(t, f)
		files[i] = f
	}
	env = append(append(os.Environ(), "TUBE=1", "TUBE_PROFILE=test", "MOZ_NO_REMOTE=1"), env...)
	tmp := filepath.Join(dir, "keeper")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	opts := Options{Path: path, Argv: argv, Env: env, Stdout: files[0], Stderr: files[1], KillDelay: 500 * time.Millisecond, TempDir: tmp}
	if change != nil {
		change(&opts)
	}
	s, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() }) // a test that cares about the result closes it itself
	r.s = s
	r.peer = sandboxtest.NewPeer(t, s.Device())
	return r
}

// startProbe starts a sandbox whose application is the test binary in the role of the probe.
func startProbe(t *testing.T, probe string, args ...string) running {
	t.Helper()
	return startProbeWith(t, nil, probe, args...)
}

// startProbeWith is startProbe, with options that change changes before the start.
func startProbeWith(t *testing.T, change func(*Options), probe string, args ...string) running {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return startSandboxWith(t, change, exe, append([]string{"probe"}, args...), sandboxtest.ProbeEnv(probe))
}

// lookPath finds a program for a test, and skips the test when it is not installed.
func lookPath(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed", name)
	}
	return path
}

// configure configures the sandbox, fails the test when it cannot, and returns the warnings.
func (r running) configure(t *testing.T, n vpn.Network, dns ...netip.Addr) []string {
	t.Helper()
	warnings, err := r.s.Configure(context.Background(), n, dns)
	if err != nil {
		t.Fatal(err)
	}
	return warnings
}

// run starts the application and waits for its end.
func (r running) run(t *testing.T) ExitStatus {
	t.Helper()
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := r.s.Wait()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// report runs the probe report, which must end with 0, and reads what it printed.
func (r running) report(t *testing.T) sandboxtest.Report {
	t.Helper()
	if st := r.run(t); st.Status() != 0 {
		t.Fatalf("the probe ended with %+v:\n%s", st, r.errors())
	}
	var rep sandboxtest.Report
	if err := json.Unmarshal([]byte(r.output(t)), &rep); err != nil {
		t.Fatalf("the report cannot be read: %v\n%s", err, r.output(t))
	}
	return rep
}

// defaultRoutes are the default routes of a report, as "family out".
func defaultRoutes(rep sandboxtest.Report) []string {
	var out []string
	for _, rt := range rep.Routes {
		if rt.Dst == "0.0.0.0/0" || rt.Dst == "::/0" {
			family := "ipv4"
			if rt.Family == unix.AF_INET6 {
				family = "ipv6"
			}
			out = append(out, family+" "+rt.Out)
		}
	}
	return out
}

func TestConfigureAndReport(t *testing.T) {
	r := startProbe(t, "report")
	r.configure(t, network4)
	rep := r.report(t)
	if rep.UID != os.Getuid() || rep.GID != os.Getgid() {
		t.Errorf("the application is uid %d gid %d, want %d %d", rep.UID, rep.GID, os.Getuid(), os.Getgid())
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Cwd != cwd {
		t.Errorf("cwd = %q, want %q", rep.Cwd, cwd)
	}
	if rep.Env["TUBE"] != "1" || rep.Env["MOZ_NO_REMOTE"] != "1" || rep.Env["TUBE_PROFILE"] != "test" {
		t.Errorf("env = %v", rep.Env)
	}
	if len(rep.Links) != 2 {
		t.Errorf("links = %+v, want lo and tube0", rep.Links)
	}
	for _, l := range rep.Links {
		switch {
		case l.Name == "lo" && l.Up:
		case l.Name == deviceName && l.Up && l.MTU == 1400:
		default:
			t.Errorf("a link that should not be so: %+v", l)
		}
	}
	if strings.Join(rep.Addrs, " ") != "10.9.0.2/24" {
		t.Errorf("the addresses of tube0 are %v", rep.Addrs)
	}
	if got := strings.Join(defaultRoutes(rep), ", "); got != "ipv4 tube0" {
		t.Errorf("the default routes are %q, want one of IPv4 out of tube0", got)
	}
	if !strings.Contains(rep.AddRouteError, "not permitted") {
		t.Errorf("the application could add a route: the error is %q", rep.AddRouteError)
	}
}

func TestConfigureIPv6(t *testing.T) {
	t.Run("with an IPv6 address", func(t *testing.T) {
		r := startProbe(t, "report")
		r.configure(t, network46)
		rep := r.report(t)
		if got := strings.Join(defaultRoutes(rep), ", "); got != "ipv4 tube0, ipv6 tube0" {
			t.Errorf("the default routes are %q", got)
		}
		if got := strings.Join(rep.Addrs, " "); !strings.Contains(got, "fd00:9::2/64") {
			t.Errorf("the addresses of tube0 are %v", got)
		}
	})
	t.Run("without one, nothing of IPv6 leaves", func(t *testing.T) {
		r := startProbe(t, "report")
		r.configure(t, network4)
		rep := r.report(t)
		if got := strings.Join(defaultRoutes(rep), ", "); got != "ipv4 tube0" {
			t.Errorf("the default routes are %q", got)
		}
		// Router solicitations and the like go out as soon as the link is up.
		time.Sleep(300 * time.Millisecond)
		for _, p := range r.peer.Packets() {
			if packettest.Version(p) == 6 {
				t.Errorf("an IPv6 packet left the namespace: % x", p)
			}
		}
	})
}

func TestConfigureTwice(t *testing.T) {
	r := startProbe(t, "report")
	r.configure(t, network4)
	r.configure(t, vpn.Network{Addrs: []netip.Prefix{netip.MustParsePrefix("10.10.0.5/24")}, MTU: 1300})
	rep := r.report(t)
	if strings.Join(rep.Addrs, " ") != "10.10.0.5/24" {
		t.Errorf("the addresses of tube0 are %v", rep.Addrs)
	}
	if got := strings.Join(defaultRoutes(rep), ", "); got != "ipv4 tube0" {
		t.Errorf("the default routes are %q", got)
	}
	for _, l := range rep.Links {
		if l.Name == deviceName && l.MTU != 1300 {
			t.Errorf("the MTU is %d", l.MTU)
		}
	}
}

func TestPing(t *testing.T) {
	tests := []struct {
		name   string
		net    vpn.Network
		to     string
		from   string
		isEcho func([]byte) bool
	}{
		{"IPv4", network4, "10.9.0.1", "10.9.0.2", packettest.IsEchoRequest4},
		{"IPv6", network46, "fd00:9::1", "fd00:9::2", packettest.IsEchoRequest6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ping := lookPath(t, "ping")
			for _, answer := range []bool{true, false} {
				r := startSandbox(t, ping, []string{"ping", "-c", "1", "-W", "2", tt.to})
				r.peer.AnswerPings(answer)
				r.configure(t, tt.net)
				st := r.run(t)
				if answer && st.Status() != 0 {
					t.Errorf("a ping that was answered ended with %+v:\n%s%s", st, r.output(t), r.errors())
				}
				if !answer && st.Status() == 0 {
					t.Errorf("a ping that nobody answered ended with 0:\n%s", r.output(t))
				}
				r.peer.WaitFor(t, time.Second, func(p []byte) bool {
					return tt.isEcho(p) && packettest.Src(p) == netip.MustParseAddr(tt.from) && packettest.Dst(p) == netip.MustParseAddr(tt.to)
				})
			}
		})
	}
}

func TestExitCodes(t *testing.T) {
	sh := lookPath(t, "sh")
	tests := []struct {
		script string
		want   ExitStatus
		status int
	}{
		{"exit 0", ExitStatus{}, 0},
		{"exit 3", ExitStatus{Code: 3}, 3},
		{"kill -TERM $$", ExitStatus{Signal: syscall.SIGTERM}, 143},
	}
	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			r := startSandbox(t, sh, []string{"sh", "-c", tt.script})
			r.configure(t, network4)
			st := r.run(t)
			if st != tt.want || st.Status() != tt.status {
				t.Errorf("the status is %+v (%d), want %+v (%d)", st, st.Status(), tt.want, tt.status)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		s    ExitStatus
		want int
	}{
		{ExitStatus{}, 0},
		{ExitStatus{Code: 1}, 1},
		{ExitStatus{Code: 127}, 127},
		{ExitStatus{Signal: syscall.SIGINT}, 130},
		{ExitStatus{Signal: syscall.SIGKILL}, 137},
		{ExitStatus{Signal: syscall.SIGTERM}, 143},
	}
	for _, tt := range tests {
		if got := tt.s.Status(); got != tt.want {
			t.Errorf("%+v.Status() = %d, want %d", tt.s, got, tt.want)
		}
	}
}

// keeperRoute fails the test when the keeper's route, in the metadata of an error, does not
// have want in it.
func keeperRoute(t *testing.T, meta map[string]any, want string) {
	t.Helper()
	route, _ := meta["keeper_route"].(string)
	if !strings.Contains(route, want) {
		t.Errorf("the keeper's route is %q, want %q in it", route, want)
	}
}

func TestRunTwice(t *testing.T) {
	r := startSandbox(t, lookPath(t, "sleep"), []string{"sleep", "30"})
	r.configure(t, network4)
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := r.s.Run(context.Background())
	meta := errtest.Route(t, err, "Sandbox.Run.SandboxRunFailed")
	keeperRoute(t, meta, "keeper.handle->keeper.startApp.KeeperAlreadyRunning")
	if err := r.s.Close(); err != nil {
		t.Errorf("Close while the application runs: %v", err)
	}
	if _, err := r.s.Wait(); err == nil {
		t.Error("Wait after Close says the application ended")
	}
}

func TestRunWithoutAnApplication(t *testing.T) {
	s := start(t, Options{})
	err := s.Run(context.Background())
	meta := errtest.Route(t, err, "Sandbox.Run.SandboxRunFailed")
	keeperRoute(t, meta, "keeper.startApp.KeeperNoApplication")
}

func TestRunRefusesBadIDs(t *testing.T) {
	r := startSandbox(t, lookPath(t, "true"), []string{"true"})
	m, _, err := r.s.exchange(context.Background(), &message{Type: msgRun, UID: -7, GID: -7}, msgStarted)
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != msgFailed || !strings.Contains(m.Route, "keeper.startApp.KeeperAppStartFailed") {
		t.Errorf("the answer is %+v", m)
	}
}

func TestAppCannotStart(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "text")
	if err := os.WriteFile(text, []byte("not a program\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, path, route, keeper string
		status                    int
	}{
		{"not found", "/nonexistent/app", "Sandbox.Run.SandboxAppNotFound", "keeper.startApp.KeeperAppNotFound", 127},
		{"a file that is not executable", text, "Sandbox.Run.SandboxAppNotExecutable", "keeper.startApp.KeeperAppNotExecutable", 126},
		{"a folder", dir, "Sandbox.Run.SandboxAppNotExecutable", "keeper.startApp.KeeperAppNotExecutable", 126},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := startSandbox(t, tt.path, []string{"app"})
			r.configure(t, network4)
			err := r.s.Run(context.Background())
			meta := errtest.Route(t, err, tt.route)
			keeperRoute(t, meta, tt.keeper)
			if got := ExitCodeOf(err); got != tt.status {
				t.Errorf("ExitCodeOf = %d, want %d", got, tt.status)
			}
		})
	}
}

func TestConfigureRefusesBadNetworks(t *testing.T) {
	tests := []struct {
		name string
		n    networkMessage
	}{
		{"an address", networkMessage{Addrs: []string{"10.9.0.2"}, MTU: 1400}},
		{"a DNS server", networkMessage{Addrs: []string{"10.9.0.2/24"}, MTU: 1400, DNS: []string{"10.9.0"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := start(t, Options{})
			_, err := s.configure(context.Background(), tt.n)
			meta := errtest.Route(t, err, "Sandbox.configure.SandboxConfigureFailed")
			keeperRoute(t, meta, "keeper.configure->parseNetwork.KeeperNetworkInvalid")
		})
	}
	t.Run("no network", func(t *testing.T) {
		s := start(t, Options{})
		m, _, err := s.exchange(context.Background(), &message{Type: msgConfigure}, msgConfigured)
		if err != nil {
			t.Fatal(err)
		}
		if m.Type != msgFailed || m.Tag != TagKeeperNetworkInvalid {
			t.Errorf("the answer is %+v", m)
		}
	})
}

func TestWriteSetting(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if err := writeSetting(missing, "1", true); err != nil {
		t.Errorf("a missing file that may be missing: %v", err)
	}
	meta := errtest.Route(t, writeSetting(missing, "1", false), "writeSetting.KeeperSysctlFailed")
	if meta["file"] != filepath.Base(dir)+"/missing" {
		t.Errorf("metadata = %v", meta)
	}
	errtest.Route(t, writeSetting(dir, "1", true), "writeSetting.KeeperSysctlFailed")
}

func TestCancelled(t *testing.T) {
	s := start(t, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Configure(ctx, network4, nil)
	errtest.Route(t, err, "Sandbox.Configure->Sandbox.configure->Sandbox.exchange.SandboxCancelled")
}

func TestKeeperLost(t *testing.T) {
	s := start(t, Options{})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := s.Wait()
	errtest.Route(t, err, "Sandbox.Wait.SandboxKeeperLost")
}

func TestALateAnswerIsDropped(t *testing.T) {
	s := start(t, Options{})
	// An answer to request 99, which nobody sent, comes before the answer to the next one.
	s.replies <- reply{message{Type: msgConfigured, Seq: 99}, -1}
	if _, err := s.Configure(context.Background(), network4, nil); err != nil {
		t.Fatal(err)
	}
}
