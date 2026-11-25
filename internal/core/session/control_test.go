// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"context"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// pinger asks the probe ping-on-demand to ping.
type pinger struct {
	readyFile, goFile string
	n                 int
}

// onDemand makes the application of the options the probe ping-on-demand.
func onDemand(t *testing.T, opts *Options) *pinger {
	t.Helper()
	dir := t.TempDir()
	p := &pinger{readyFile: filepath.Join(dir, "ready"), goFile: filepath.Join(dir, "go")}
	opts.ExtraEnv = append(opts.ExtraEnv, sandboxtest.ProbeEnv("ping-on-demand"), sandboxtest.ReadyFileVar+"="+p.readyFile, sandboxtest.GoFileVar+"="+p.goFile)
	return p
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
	return waitFile(t, p.readyFile+"."+strconv.Itoa(p.n), 20*time.Second) == "ok"
}

// end ends the probe.
func (p *pinger) end(t *testing.T) {
	t.Helper()
	p.ask(t, sandboxtest.EndPings)
}

// controlled is a session of the tests of this file: the probe ping-on-demand, the fake clock,
// a Redial, and the lines.
type controlled struct {
	run
	p     *pinger
	clk   *clock.Fake
	r     *redialer
	lines *syncBuffer
}

// startControlled starts a session with the first tunnel.
func startControlled(t *testing.T, tun vpn.Tunnel, r *redialer, change func(*Options)) controlled {
	t.Helper()
	lookPath(t, "ping")
	c := controlled{clk: fakeClock(), r: r, lines: &syncBuffer{}}
	path, argv := probe(t)
	opts, rn := reconnecting(t, tun, path, argv, r, c.clk, c.lines)
	c.p = onDemand(t, &opts)
	if change != nil {
		change(&opts)
	}
	c.run = start(t, opts, rn)
	t.Cleanup(func() { c.p.end(t) })
	return c
}

// connection is a Connection of a test, without Redial.
func connection(tun vpn.Tunnel, name string) Connection {
	return Connection{Tunnel: tun, ProfileName: name}
}

// noRedials is a redialer that must not be called; it fails when it is.
func noRedials() *redialer {
	return &redialer{answers: []func(context.Context) (vpn.Tunnel, error){errorAnswer(noAnswer)}}
}

func TestDisconnect(t *testing.T) {
	tun := pingTunnel(network)
	c := startControlled(t, tun, noRedials(), nil)
	if !c.p.ping(t, "10.9.0.1") {
		t.Fatal("the first ping failed")
	}
	c.s.Disconnect()
	if c.p.ping(t, "10.9.0.1") {
		t.Error("a ping worked after Disconnect")
	}
	if !tun.Closed() {
		t.Error("the tunnel is not closed")
	}
	if st := c.s.Status(); st.State != StateDisconnected || len(st.Network.Addrs) != 0 {
		t.Errorf("the status is %+v", st)
	}
	c.clk.Advance(5 * time.Minute)
	c.s.Disconnect()
	if got := c.r.count(); got != 0 {
		t.Errorf("Redial was called %d times", got)
	}
	if got := strings.Count(c.lines.String(), "tube: disconnected; the application has no network\n"); got != 1 {
		t.Errorf("the lines are %q", c.lines.String())
	}
}

func TestUseAfterDisconnect(t *testing.T) {
	c := startControlled(t, pingTunnel(network), noRedials(), nil)
	c.s.Disconnect()
	if err := c.s.Use(context.Background(), connection(pingTunnel(network2), "second")); err != nil {
		t.Fatal(err)
	}
	if !c.p.ping(t, "10.10.0.1") {
		t.Error("a ping through the second tunnel failed")
	}
	st := c.s.Status()
	if st.State != StateConnected || st.ProfileName != "second" || st.Network.Addrs[0] != network2.Addrs[0] {
		t.Errorf("the status is %+v", st)
	}
	if !strings.Contains(c.lines.String(), "tube: now using second\n") {
		t.Errorf("the lines are %q", c.lines.String())
	}
}

func TestUseWhileConnected(t *testing.T) {
	tun1 := pingTunnel(network)
	c := startControlled(t, tun1, noRedials(), nil)
	if !c.p.ping(t, "10.9.0.1") {
		t.Fatal("the first ping failed")
	}
	for len(tun1.Written()) > 0 {
		<-tun1.Written()
	}
	if err := c.s.Use(context.Background(), connection(pingTunnel(network2), "second")); err != nil {
		t.Fatal(err)
	}
	if !tun1.Closed() {
		t.Error("the first tunnel is not closed")
	}
	for range 2 {
		if !c.p.ping(t, "10.10.0.1") {
			t.Error("a ping through the second tunnel failed")
		}
	}
	if n := len(tun1.Written()); n > 0 {
		t.Errorf("the first tunnel got %d packets after the switch", n)
	}
}

func TestUseWhileReconnecting(t *testing.T) {
	tun1 := pingTunnel(network)
	c := startControlled(t, tun1, noRedials(), nil)
	tun1.Fail(io.EOF)
	eventually(t, "the session waits", func() bool { return c.clk.Pending() == 1 })
	if st := c.s.Status(); st.State != StateReconnecting {
		t.Errorf("the state is %v", st.State)
	}
	if err := c.s.Use(context.Background(), connection(pingTunnel(network2), "second")); err != nil {
		t.Fatal(err)
	}
	if st := c.s.Status(); st.State != StateConnected || c.clk.Pending() != 0 {
		t.Errorf("the state is %v, with %d timers", st.State, c.clk.Pending())
	}
	c.clk.Advance(time.Hour)
	if got := c.r.count(); got != 0 {
		t.Errorf("the old Redial was called %d times", got)
	}
	if !c.p.ping(t, "10.10.0.1") {
		t.Error("a ping through the second tunnel failed")
	}
}

func TestUseWithANetworkThatCannotBeConfigured(t *testing.T) {
	c := startControlled(t, pingTunnel(network), noRedials(), nil)
	bad := network2
	bad.MTU = 100000
	tun2 := pingTunnel(bad)
	err := c.s.Use(context.Background(), connection(tun2, "bad"))
	errtest.Route(t, err, "SandboxConfigureFailed")
	if !tun2.Closed() {
		t.Error("the tunnel that cannot be used is not closed")
	}
	if st := c.s.Status(); st.State != StateDisconnected {
		t.Errorf("the state is %v", st.State)
	}
	if c.p.ping(t, "10.9.0.1") || c.p.ping(t, "10.10.0.1") {
		t.Error("a ping worked after a switch that failed")
	}
}

func TestUseAfterTheEnd(t *testing.T) {
	opts, rn := options(t, pingTunnel(network), lookPath(t, "true"), []string{"true"})
	rn = start(t, opts, rn)
	rn.wait(t)
	tun2 := pingTunnel(network2)
	err := rn.s.Use(context.Background(), connection(tun2, "second"))
	errtest.Route(t, err, "Session.Use.SessionEnded")
	if !tun2.Closed() {
		t.Error("the given tunnel is not closed")
	}
	rn.s.Disconnect() // after the end: nothing happens
}

func TestTheTrafficAddsUp(t *testing.T) {
	c := startControlled(t, pingTunnel(network), noRedials(), nil)
	for range 2 {
		if !c.p.ping(t, "10.9.0.1") {
			t.Fatal("a ping through the first tunnel failed")
		}
	}
	if err := c.s.Use(context.Background(), connection(pingTunnel(network2), "second")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if !c.p.ping(t, "10.10.0.1") {
			t.Fatal("a ping through the second tunnel failed")
		}
	}
	if tr := c.s.Status().Traffic; tr.ToTunnel.Packets < 4 || tr.FromTunnel.Packets < 4 || tr.ToTunnel.Bytes == 0 {
		t.Errorf("the traffic is %+v", tr)
	}
}

func TestOnChange(t *testing.T) {
	tun1 := pingTunnel(network)
	r := &redialer{answers: []func(context.Context) (vpn.Tunnel, error){tunnelAnswer(pingTunnel(network))}}
	var mu sync.Mutex
	var seen []string
	var sess atomic.Pointer[Session]
	var waited atomic.Bool
	c := startControlled(t, tun1, r, func(o *Options) {
		o.OnChange = func() {
			if waited.Load() {
				t.Error("OnChange was called after Wait returned")
			}
			st := sess.Load().Status()
			mu.Lock()
			seen = append(seen, st.State.String()+" "+strconv.FormatBool(st.AppRunning))
			mu.Unlock()
		}
	})
	sess.Store(c.s)
	tun1.Fail(io.EOF)
	eventually(t, "the session waits", func() bool { return c.clk.Pending() == 1 })
	c.clk.Advance(time.Second)
	eventually(t, "the line connected again", hasLine(c.lines, "tube: connected again\n"))
	c.s.Disconnect()
	if err := c.s.Use(context.Background(), connection(pingTunnel(network2), "second")); err != nil {
		t.Fatal(err)
	}
	c.p.end(t)
	c.wait(t)
	waited.Store(true)
	mu.Lock()
	got := strings.Join(seen, ", ")
	mu.Unlock()
	want := "reconnecting true, connected true, disconnected true, connected true, connected false"
	if got != want {
		t.Errorf("the changes are %q, want %q", got, want)
	}
}

func TestDone(t *testing.T) {
	c := startControlled(t, pingTunnel(network), noRedials(), nil)
	select {
	case <-c.s.Done():
		t.Fatal("Done is closed while the application runs")
	case <-time.After(100 * time.Millisecond):
	}
	c.p.end(t)
	select {
	case <-c.s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done is not closed after the application ended")
	}
	if st := c.s.Status(); st.AppRunning {
		eventually(t, "the status says the application ended", func() bool { return !c.s.Status().AppRunning })
	}
}

func TestTheStates(t *testing.T) {
	tun1 := pingTunnel(network)
	refused := &errors.Error{Tag: "AuthFailed", Message: "the server refused the login"}
	c := startControlled(t, tun1, &redialer{answers: []func(context.Context) (vpn.Tunnel, error){errorAnswer(refused)}}, nil)
	before := c.s.Status()
	if before.State != StateConnected || !before.AppRunning || before.ProfileName != "work" || before.DNS[0] != netip.MustParseAddr("10.9.0.1") {
		t.Errorf("the status at the start is %+v", before)
	}
	tun1.Fail(io.EOF)
	eventually(t, "the session waits", func() bool { return c.clk.Pending() == 1 })
	c.clk.Advance(time.Second)
	eventually(t, "the state refused", func() bool { return c.s.Status().State == StateRefused })
	if err := c.s.Use(context.Background(), connection(pingTunnel(network2), "second")); err != nil {
		t.Fatal(err)
	}
	if st := c.s.Status(); st.State != StateConnected {
		t.Errorf("the state is %v", st.State)
	}
}
