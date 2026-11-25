// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/packettest"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/storage"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

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

// started is an App whose application was started over a fake tunnel that answers pings.
type started struct {
	a    *App
	tun  *vpntest.Tunnel
	clk  *clock.Fake
	dirs storage.Dirs
}

// openRun opens an App in isolated folders with the fake clock, the target argv and a fake
// tunnel of the profile "work"; it is closed when the test ends.
func openRun(t *testing.T, argv ...string) started {
	t.Helper()
	nstest.Require(t)
	dirs := storagetest.Isolate(t)
	clk := clock.NewFake(time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC))
	a, err := Open(context.Background(), Options{Argv: argv, Clock: clk})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	tun := pingTunnel(fakeNetwork)
	a.setTunnel(tun, profiles.Profile{ID: "0123456789abcdef", Name: "work"})
	return started{a: a, tun: tun, clk: clk, dirs: dirs}
}

// startRun is openRun, then Start.
func startRun(t *testing.T, argv ...string) started {
	t.Helper()
	s := openRun(t, argv...)
	if err := s.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

// waitUntil waits until cond is true, 20 seconds at most.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited 20 seconds for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// output is the application's output so far, a line each.
func (s started) output() string {
	first, end := s.a.OutputLen()
	return strings.Join(s.a.OutputRange(first, end), "\n")
}

// waitOutput waits until the output has want.
func (s started) waitOutput(t *testing.T, want string) {
	t.Helper()
	waitUntil(t, "the output "+strconv.Quote(want), func() bool { return strings.Contains(s.output(), want) })
}

// waitEnd waits until the application has ended and returns its status.
func (s started) waitEnd(t *testing.T) int {
	t.Helper()
	waitUntil(t, "the end of the application", func() bool { return !s.a.RunState().AppRunning })
	return s.a.RunState().Exit
}

func TestStartAndFinish(t *testing.T) {
	s := startRun(t, "sh", "-c", "echo hello; exit 3")
	if !s.a.RunState().Started {
		t.Error("Started is false")
	}
	if exit := s.waitEnd(t); exit != 3 {
		t.Errorf("Exit is %d", exit)
	}
	s.waitOutput(t, "hello")
	status, err := s.a.Finish()
	if err != nil || status != 3 {
		t.Errorf("Finish = %d, %v", status, err)
	}
	if status, err := s.a.Finish(); err != nil || status != 3 {
		t.Errorf("a second Finish = %d, %v", status, err)
	}
	entries, err := os.ReadDir(s.dirs.TempBase)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the temporary folders are left: %v", entries)
	}
	if !s.tun.Closed() {
		t.Error("the tunnel is not closed")
	}
}

func TestTheOutputIsLive(t *testing.T) {
	file := t.TempDir() + "/go"
	s := startRun(t, "sh", "-c", "echo first; while [ ! -e "+file+" ]; do sleep 0.05; done; echo second")
	s.waitOutput(t, "first")
	if strings.Contains(s.output(), "second") {
		t.Fatal("the second line came before the file")
	}
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s.waitOutput(t, "second")
}

func TestTheTerminal(t *testing.T) {
	s := startRun(t, "sh", "-c", "echo term=$TERM; tty")
	s.waitEnd(t)
	s.waitOutput(t, "term=dumb")
	s.waitOutput(t, "/dev/pts/")
}

func TestInput(t *testing.T) {
	s := startRun(t, "sh", "-c", "read x; echo got:$x")
	if err := s.a.Input([]byte("abc\n")); err != nil {
		t.Fatal(err)
	}
	s.waitOutput(t, "got:abc")
}

func TestCtrlC(t *testing.T) {
	s := startRun(t, "sleep", "30")
	// The line discipline sends SIGINT once the application waits; a ^C typed before its
	// terminal is its own would be lost, so it is typed until it ends.
	waitUntil(t, "the end after ctrl+c", func() bool {
		if err := s.a.Input([]byte{3}); err != nil {
			t.Fatal(err)
		}
		return !s.a.RunState().AppRunning
	})
	if exit := s.a.RunState().Exit; exit != 130 {
		t.Errorf("Exit is %d", exit)
	}
}

func TestInputFails(t *testing.T) {
	s := startRun(t, "sh", "-c", "read x")
	r := s.a.startedRun()
	if err := r.master.Close(); err != nil {
		t.Fatal(err)
	}
	err := s.a.Input([]byte("x\n"))
	errtest.Route(t, err, "App.Input.AppInputFailed")
	s.a.Stop()
}

func TestResize(t *testing.T) {
	if _, err := exec.LookPath("stty"); err != nil {
		t.Skip("stty is not installed")
	}
	s := openRun(t, "sh", "-c", "sleep 0.3; stty size")
	s.a.ResizeOutput(120, 40)
	if err := s.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.a.ResizeOutput(100, 30)
	s.waitOutput(t, "30 100")
}

// pinger is the probe ping-on-demand as the application of an App.
type pinger struct {
	goFile, readyFile string
	n                 int
}

// startPinger starts the probe ping-on-demand as the application.
func startPinger(t *testing.T) (started, *pinger) {
	t.Helper()
	s := openRun(t, testBinary(t))
	p := newPinger(t)
	if err := s.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, p
}

// newPinger makes the test binary, when it is started, run the probe ping-on-demand with the
// files of a new pinger; the probe is ended when the test ends. It comes after openRun.
func newPinger(t *testing.T) *pinger {
	t.Helper()
	dir := t.TempDir()
	p := &pinger{goFile: dir + "/go", readyFile: dir + "/ready"}
	t.Setenv(sandboxtest.GoFileVar, p.goFile)
	t.Setenv(sandboxtest.ReadyFileVar, p.readyFile)
	setProbe(t, "ping-on-demand")
	t.Cleanup(func() { p.ask(t, sandboxtest.EndPings) })
	return p
}

// testBinary is the path of the test binary, which runs a probe as the application.
func testBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// setProbe makes the test binary run the probe when it is started. It comes after openRun:
// nstest.Require runs the test binary too, which must not run the probe.
func setProbe(t *testing.T, name string) {
	t.Helper()
	k, v, _ := strings.Cut(sandboxtest.ProbeEnv(name), "=")
	t.Setenv(k, v)
}

// ask writes the next file of the probe, whole at once.
func (p *pinger) ask(t *testing.T, text string) {
	t.Helper()
	p.n++
	path := p.goFile + "." + strconv.Itoa(p.n)
	if err := os.WriteFile(path+".tmp", []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

// ping makes the application ping the address once, and says whether it was answered.
func (p *pinger) ping(t *testing.T, addr string) bool {
	t.Helper()
	p.ask(t, addr)
	ready := p.readyFile + "." + strconv.Itoa(p.n)
	var got []byte
	waitUntil(t, "the answer of the probe", func() bool {
		var err error
		got, err = os.ReadFile(ready)
		return err == nil && len(got) > 0
	})
	return strings.TrimSpace(string(got)) == "ok"
}

// samples is the number of samples of the traffic so far.
func (a *App) sampleCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rd.samples.n
}

func TestRunState(t *testing.T) {
	s, p := startPinger(t)
	rs := s.a.RunState()
	if rs.State != "connected" || rs.ProfileName != "work" || rs.ProfileID != "0123456789abcdef" || rs.MTU != 1400 ||
		!slices.Equal(rs.Addrs, []string{"10.9.0.2/24"}) || !slices.Equal(rs.DNS, []string{"10.9.0.1"}) || !rs.AppRunning {
		t.Errorf("the run state is %+v", rs)
	}
	if rs.AppName == "" || strings.Contains(rs.AppName, "/") {
		t.Errorf("AppName is %q", rs.AppName)
	}
	tick := func(n int) {
		s.clk.Advance(time.Second)
		waitUntil(t, "sample "+strconv.Itoa(n), func() bool { return s.a.sampleCount() >= n })
	}
	tick(1)
	if !p.ping(t, "10.9.0.1") {
		t.Fatal("the ping was not answered")
	}
	rs = s.a.RunState()
	if rs.Down.Packets < 1 || rs.Up.Packets < 1 {
		t.Errorf("the traffic is %+v %+v", rs.Down, rs.Up)
	}
	tick(2)
	if rs := s.a.RunState(); rs.Down.Rate <= 0 || rs.Up.Rate <= 0 {
		t.Errorf("the rates after a second with traffic: %v, %v", rs.Down.Rate, rs.Up.Rate)
	}
	tick(3)
	if rs := s.a.RunState(); rs.Down.Rate != 0 || rs.Up.Rate != 0 {
		t.Errorf("the rates after a second without traffic: %v, %v", rs.Down.Rate, rs.Up.Rate)
	}
}

func TestRunStateBeforeStart(t *testing.T) {
	s := openRun(t, "true")
	rs := s.a.RunState()
	if rs.Started || rs.Now.IsZero() || rs.State != "" {
		t.Errorf("the run state is %+v", rs)
	}
}

func TestEvents(t *testing.T) {
	s := startRun(t, "sh", "-c", "read x")
	defer s.a.Stop()
	events := s.a.RunState().Events
	if len(events) != 1 || events[0].Text != "connected to work; running sh" || !events[0].Time.Equal(s.clk.Now()) {
		t.Fatalf("the events are %+v", events)
	}
	w := eventWriter{a: s.a, r: s.a.startedRun()}
	// Two writes make one line.
	for i := range 150 {
		if _, err := w.Write([]byte("tube: event " + strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			t.Fatal(err)
		}
	}
	events = s.a.RunState().Events
	if len(events) != 100 || events[0].Text != "event 50" || events[99].Text != "event 149" {
		t.Errorf("%d events, from %q to %q", len(events), events[0].Text, events[len(events)-1].Text)
	}
	if _, err := w.Write([]byte("tube: hostile\x1b[2J\x07\n")); err != nil {
		t.Fatal(err)
	}
	events = s.a.RunState().Events
	if last := events[len(events)-1].Text; !strings.HasPrefix(last, "hostile") || strings.ContainsAny(last, "\x1b\x07") {
		t.Errorf("the hostile event is %q", last)
	}
}

func TestDisconnect(t *testing.T) {
	s := startRun(t, "sh", "-c", "read x")
	defer s.a.Stop()
	s.a.Disconnect()
	rs := s.a.RunState()
	if rs.State != "disconnected" {
		t.Errorf("the state is %s", rs.State)
	}
	if last := rs.Events[len(rs.Events)-1].Text; last != "disconnected; the application has no network" {
		t.Errorf("the last event is %q", last)
	}
	if !s.tun.Closed() {
		t.Error("the tunnel is not closed")
	}
}

func TestStop(t *testing.T) {
	s := startRun(t, "sleep", "30")
	s.a.Stop()
	if exit := s.waitEnd(t); exit != 143 {
		t.Errorf("Exit is %d", exit)
	}
}

func TestStopKills(t *testing.T) {
	pidFile := t.TempDir() + "/pid"
	s := openRun(t, testBinary(t))
	t.Setenv(sandboxtest.PIDFileVar, pidFile)
	setProbe(t, "stubborn")
	s.a.rd.killDelay = 300 * time.Millisecond
	if err := s.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the pid of the probe", func() bool {
		data, err := os.ReadFile(pidFile)
		return err == nil && len(data) > 0
	})
	s.a.Stop()
	if exit := s.waitEnd(t); exit != 137 {
		t.Errorf("Exit is %d", exit)
	}
}

func TestFinishStopsARunningApplication(t *testing.T) {
	s := startRun(t, "sleep", "30")
	status, err := s.a.Finish()
	if err != nil || status != 143 {
		t.Errorf("Finish = %d, %v", status, err)
	}
}

func TestSubscribersAreTold(t *testing.T) {
	s := openRun(t, "sh", "-c", "echo one; read x; echo two")
	var calls atomic.Int64
	cancel := s.a.Subscribe(func() { calls.Add(1) })
	defer cancel()
	if err := s.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.waitOutput(t, "one")
	waitUntil(t, "a call after the output", func() bool { return calls.Load() >= 1 })
	before := calls.Load()
	s.a.Disconnect()
	waitUntil(t, "a call after the change of state", func() bool { return calls.Load() > before })
	before = calls.Load()
	if err := s.a.Input([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	s.waitEnd(t)
	waitUntil(t, "a call after the end", func() bool { return calls.Load() > before })

	flood := openRun(t, "seq", "10000")
	calls.Store(0)
	cancel2 := flood.a.Subscribe(func() { calls.Add(1) })
	defer cancel2()
	if err := flood.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	flood.waitEnd(t)
	flood.waitOutput(t, "10000")
	if n := calls.Load(); n >= 200 {
		t.Errorf("10000 lines called the subscriber %d times", n)
	}
}

func TestStartTwice(t *testing.T) {
	s := startRun(t, "sh", "-c", "read x")
	defer s.a.Stop()
	errtest.Route(t, s.a.Start(context.Background()), "App.Start.AppAlreadyStarted")
}

func TestStartWithoutConnect(t *testing.T) {
	a, _ := open(t, "true")
	errtest.Route(t, a.Start(context.Background()), "App.Start->App.takeTunnel.AppNotConnected")
	errtest.Route(t, a.Switch(context.Background(), "x", nil, false), "App.Switch.AppNotStarted")
	if status, err := a.Finish(); status != 0 || err != nil {
		t.Errorf("Finish without Start = %d, %v", status, err)
	}
}
