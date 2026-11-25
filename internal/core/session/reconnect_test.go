// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"context"
	"encoding/json"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// network2 is the network of the second tunnel: the server is 10.10.0.1.
var network2 = vpn.Network{
	Addrs: []netip.Prefix{netip.MustParsePrefix("10.10.0.5/24")}, DNSServers: []netip.Addr{netip.MustParseAddr("10.10.0.1")}, MTU: 1400,
}

// redialer is a Redial of a test: it counts its calls and answers each with the next of its
// answers (the last one repeats).
type redialer struct {
	mu      sync.Mutex
	calls   int
	answers []func(ctx context.Context) (vpn.Tunnel, error)
}

// redial is the Redial of the session.
func (r *redialer) redial(ctx context.Context) (vpn.Tunnel, error) {
	r.mu.Lock()
	r.calls++
	answer := r.answers[min(r.calls, len(r.answers))-1]
	r.mu.Unlock()
	return answer(ctx)
}

// count is how many times Redial was called.
func (r *redialer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// tunnelAnswer answers with the tunnel.
func tunnelAnswer(tun vpn.Tunnel) func(context.Context) (vpn.Tunnel, error) {
	return func(context.Context) (vpn.Tunnel, error) { return tun, nil }
}

// errorAnswer answers with the error.
func errorAnswer(err error) func(context.Context) (vpn.Tunnel, error) {
	return func(context.Context) (vpn.Tunnel, error) { return nil, err }
}

// eventually waits five seconds at most for the condition, and fails the test with what when it
// does not come.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("this did not happen: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fakeClock is the fake clock of the tests of reconnecting.
func fakeClock() *clock.Fake { return clock.NewFake(time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)) }

// reconnecting are the options of a session that connects again with r on the fake clock, and
// writes its lines into lines.
func reconnecting(t *testing.T, tun vpn.Tunnel, path string, argv []string, r *redialer, clk *clock.Fake, lines *syncBuffer) (Options, run) {
	t.Helper()
	opts, rn := options(t, tun, path, argv)
	opts.Redial, opts.Clock, opts.Lines = r.redial, clk, lines
	return opts, rn
}

// hasLine says whether the lines have one with text in it.
func hasLine(lines *syncBuffer, text string) func() bool {
	return func() bool { return strings.Contains(lines.String(), text) }
}

func TestReconnect(t *testing.T) {
	lookPath(t, "ping")
	t.Run("ping", func(t *testing.T) {
		var lines syncBuffer
		tun1, tun2 := pingTunnel(network), pingTunnel(network2)
		clk, r := fakeClock(), &redialer{answers: []func(context.Context) (vpn.Tunnel, error){tunnelAnswer(tun2)}}
		path, argv := probe(t, "10.9.0.1", "10.10.0.1")
		opts, rn := reconnecting(t, tun1, path, argv, r, clk, &lines)
		steps := pingSteps(t, &opts)
		rn = start(t, opts, rn)
		steps.ready(t, 1)
		tun1.Fail(io.EOF)
		eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
		clk.Advance(time.Second)
		eventually(t, "the line connected again", hasLine(&lines, "tube: connected again\n"))
		steps.goOn(t, 2)
		rn.wait(t)
		if out := strings.TrimSpace(read(t, rn.stdout)); out != "1=ok 2=ok" {
			t.Errorf("the probe printed %q", out)
		}
		if !tun1.Closed() || !tun2.Closed() {
			t.Errorf("a tunnel is left open: %v %v", !tun1.Closed(), !tun2.Closed())
		}
		want := "tube: the VPN connection ended (the server closed the connection); the application has no network; connecting again\u2026\n"
		if !strings.Contains(lines.String(), want) {
			t.Errorf("the lines are %q", lines.String())
		}
	})
	t.Run("report", func(t *testing.T) {
		var lines syncBuffer
		tun1 := pingTunnel(network)
		clk, r := fakeClock(), &redialer{answers: []func(context.Context) (vpn.Tunnel, error){tunnelAnswer(pingTunnel(network2))}}
		path, argv := probe(t)
		opts, rn := reconnecting(t, tun1, path, argv, r, clk, &lines)
		goFile := filepath.Join(t.TempDir(), "go")
		opts.ExtraEnv = []string{sandboxtest.ProbeEnv("report"), sandboxtest.GoFileVar + "=" + goFile}
		rn = start(t, opts, rn)
		tun1.Fail(io.EOF)
		eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
		clk.Advance(time.Second)
		eventually(t, "the line connected again", hasLine(&lines, "tube: connected again\n"))
		if err := os.WriteFile(goFile, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if status := rn.wait(t); status != 0 {
			t.Fatalf("the probe ended with %d:\n%s", status, read(t, rn.stderr))
		}
		var rep sandboxtest.Report
		if err := json.Unmarshal([]byte(read(t, rn.stdout)), &rep); err != nil {
			t.Fatal(err)
		}
		defaults := 0
		for _, rt := range rep.Routes {
			if rt.Dst == "0.0.0.0/0" {
				defaults++
			}
		}
		if strings.Join(rep.Addrs, " ") != "10.10.0.5/24" || defaults != 1 {
			t.Errorf("the addresses are %v, with %d default routes", rep.Addrs, defaults)
		}
	})
}

func TestNoNetworkBetweenTheTunnels(t *testing.T) {
	lookPath(t, "ping")
	var lines syncBuffer
	tun1 := pingTunnel(network)
	clk, r := fakeClock(), &redialer{answers: []func(context.Context) (vpn.Tunnel, error){tunnelAnswer(pingTunnel(network2))}}
	path, argv := probe(t, "10.9.0.1", "10.9.0.1", "10.10.0.1")
	opts, rn := reconnecting(t, tun1, path, argv, r, clk, &lines)
	steps := pingSteps(t, &opts)
	rn = start(t, opts, rn)
	steps.ready(t, 1)
	tun1.Fail(io.EOF)
	eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
	steps.goOn(t, 2)
	steps.ready(t, 2)
	clk.Advance(time.Second)
	eventually(t, "the line connected again", hasLine(&lines, "tube: connected again\n"))
	steps.goOn(t, 3)
	rn.wait(t)
	if out := strings.TrimSpace(read(t, rn.stdout)); out != "1=ok 2=failed 3=ok" {
		t.Errorf("the probe printed %q", out)
	}
}

// noAnswer is an error of the kind KindNoAnswer, as the library makes it.
var noAnswer = &errors.Error{Tag: "PingTimeout", Message: "the server did not answer"}

func TestTheDelays(t *testing.T) {
	var lines syncBuffer
	tun1 := pingTunnel(network)
	answers := []func(context.Context) (vpn.Tunnel, error){}
	for range 5 {
		answers = append(answers, errorAnswer(noAnswer))
	}
	answers = append(answers, tunnelAnswer(pingTunnel(network2)))
	clk, r := fakeClock(), &redialer{answers: answers}
	opts, rn := reconnecting(t, tun1, lookPath(t, "sleep"), []string{"sleep", "60"}, r, clk, &lines)
	rn = start(t, opts, rn)
	tun1.Fail(io.EOF)
	for i, d := range []time.Duration{1, 2, 4, 8, 15, 30} {
		eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
		clk.Advance(d*time.Second - time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		if got := r.count(); got != i {
			t.Fatalf("the try %d came before %v: %d tries", i+1, d*time.Second, got)
		}
		clk.Advance(time.Millisecond)
		eventually(t, "the next try", func() bool { return r.count() == i+1 })
	}
	eventually(t, "the line connected again", hasLine(&lines, "tube: connected again\n"))
	if strings.Count(lines.String(), "\n") != 3 { // connected, ended, connected again
		t.Errorf("the lines are %q", lines.String())
	}
	rn.s.Signal(syscall.SIGTERM)
	rn.wait(t)
}

func TestARefusedLogin(t *testing.T) {
	var lines syncBuffer
	tun1 := pingTunnel(network)
	refused := &errors.Error{Tag: "AuthFailed", Message: "the server refused the login"}
	clk, r := fakeClock(), &redialer{answers: []func(context.Context) (vpn.Tunnel, error){errorAnswer(refused)}}
	opts, rn := reconnecting(t, tun1, lookPath(t, "sleep"), []string{"sleep", "2"}, r, clk, &lines)
	rn = start(t, opts, rn)
	tun1.Fail(io.EOF)
	eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
	clk.Advance(time.Second)
	eventually(t, "the line of the refusal", hasLine(&lines, "tube: the VPN server refused the user name or the password; the application stays without network\n"))
	clk.Advance(time.Hour)
	if status := rn.wait(t); status != 0 {
		t.Errorf("the status is %d", status)
	}
	if got := r.count(); got != 1 {
		t.Errorf("Redial was called %d times", got)
	}
}

func TestTheApplicationEndsWhileWaiting(t *testing.T) {
	var lines syncBuffer
	tun1 := pingTunnel(network)
	clk, r := fakeClock(), &redialer{answers: []func(context.Context) (vpn.Tunnel, error){errorAnswer(noAnswer)}}
	opts, rn := reconnecting(t, tun1, lookPath(t, "sleep"), []string{"sleep", "1"}, r, clk, &lines)
	rn = start(t, opts, rn)
	tun1.Fail(io.EOF)
	eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
	rn.wait(t)
	if clk.Pending() != 0 {
		t.Error("the wait was not stopped")
	}
	clk.Advance(time.Hour)
	if got := r.count(); got != 0 {
		t.Errorf("Redial was called %d times", got)
	}
}

func TestTheApplicationEndsWhileDialing(t *testing.T) {
	var lines syncBuffer
	tun1, late := pingTunnel(network), pingTunnel(network2)
	dialing := make(chan struct{})
	// The redial is in flight until it is cancelled; then it still returns a tunnel.
	blocked := func(ctx context.Context) (vpn.Tunnel, error) {
		close(dialing)
		<-ctx.Done()
		return late, nil
	}
	clk, r := fakeClock(), &redialer{answers: []func(context.Context) (vpn.Tunnel, error){blocked}}
	opts, rn := reconnecting(t, tun1, lookPath(t, "sleep"), []string{"sleep", "2"}, r, clk, &lines)
	rn = start(t, opts, rn)
	tun1.Fail(io.EOF)
	eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
	clk.Advance(time.Second)
	<-dialing
	ended := make(chan struct{})
	go func() {
		rn.wait(t)
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(4 * time.Second): // the application ends after 2 seconds
		t.Fatal("Wait did not return")
	}
	if !late.Closed() {
		t.Error("the tunnel that the cancelled redial returned is not closed")
	}
}
