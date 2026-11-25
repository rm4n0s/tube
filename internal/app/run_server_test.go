// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package app

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// secondSubnet is the network of the second server of the tests of switching.
var secondSubnet = netip.MustParsePrefix("10.9.0.0/24")

// addAs adds the profile of the server to the App's store under the name.
func addAs(t *testing.T, a *App, server *vpntest.Server, name string) profiles.Profile {
	t.Helper()
	p, err := a.store.Add(context.Background(), server.ProfilePath(), name, profiles.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// switching is an App that runs the probe ping-on-demand through the first of two servers.
type switching struct {
	a                 *App
	p                 *pinger
	first, second     *vpntest.Server
	firstID, secondID string
}

// startSwitching starts two servers, connects the first, and starts the probe.
func startSwitching(t *testing.T, secondOpts vpntest.ServerOptions) switching {
	t.Helper()
	first := vpntest.StartServer(t, vpntest.ServerOptions{})
	secondOpts.Subnet = secondSubnet
	second := vpntest.StartServer(t, secondOpts)
	s := openRun(t, testBinary(t))
	firstP := addAs(t, s.a, first, "first")
	secondP := addAs(t, s.a, second, "second")
	if err := connect(t, s.a, firstP.ID, nil, false); err != nil {
		t.Fatalf("Connect: %v\n%s", err, first.Log())
	}
	p := newPinger(t)
	if err := s.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return switching{a: s.a, p: p, first: first, second: second, firstID: firstP.ID, secondID: secondP.ID}
}

// switchTo switches with a context of 20 seconds.
func (s switching) switchTo(t *testing.T, id string, creds *Credentials) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return s.a.Switch(ctx, id, creds, Remember{})
}

func TestSwitch(t *testing.T) {
	s := startSwitching(t, vpntest.ServerOptions{})
	if !s.p.ping(t, s.first.TunnelAddr().String()) {
		t.Fatal("the first server does not answer before the switch")
	}
	if err := s.switchTo(t, s.secondID, nil); err != nil {
		t.Fatalf("Switch: %v\n%s", err, s.second.Log())
	}
	if !s.p.ping(t, s.second.TunnelAddr().String()) {
		t.Errorf("the second server does not answer after the switch\n%s", s.second.Log())
	}
	// The two servers share the namespace of scripts/vpn-ns.sh, whose kernel owns both of their
	// tunnel addresses: a ping of the first one through the second tunnel is answered too. That
	// the first VPN is no longer used shows in its server, which was told that the client left.
	waitUntil(t, "the first server to see the client leave", func() bool {
		return strings.Contains(s.first.Log(), "exit message received by peer")
	})
	rs := s.a.RunState()
	if rs.ProfileName != "second" || rs.ProfileID != s.secondID || len(rs.Addrs) != 1 || !secondSubnet.Contains(netip.MustParsePrefix(rs.Addrs[0]).Addr()) {
		t.Errorf("the run state is %+v", rs)
	}
	if last := rs.Events[len(rs.Events)-1].Text; last != "now using second" {
		t.Errorf("the last event is %q", last)
	}
}

func TestSwitchToAWrongPassword(t *testing.T) {
	s := startSwitching(t, vpntest.ServerOptions{Username: "alice", Password: "right"})
	err := s.switchTo(t, s.secondID, &Credentials{Username: "alice", Password: "wrong"})
	if vpn.KindOf(err) != vpn.KindAuthFailed {
		t.Fatalf("Switch = %v, want the login refused", err)
	}
	if got := Explain(err); got != "The VPN server refused the user name or the password. (AuthFailed)" {
		t.Errorf("Explain = %q", got)
	}
	if !s.p.ping(t, s.first.TunnelAddr().String()) {
		t.Error("the first VPN is not in use after a failed switch")
	}
	if rs := s.a.RunState(); rs.ProfileName != "first" || rs.State != "connected" {
		t.Errorf("the run state is %+v", rs)
	}
}

func TestSwitchAfterDisconnect(t *testing.T) {
	s := startSwitching(t, vpntest.ServerOptions{})
	s.a.Disconnect()
	if s.p.ping(t, s.first.TunnelAddr().String()) {
		t.Fatal("the first server answers after the disconnect")
	}
	if err := s.switchTo(t, s.secondID, nil); err != nil {
		t.Fatalf("Switch: %v\n%s", err, s.second.Log())
	}
	if !s.p.ping(t, s.second.TunnelAddr().String()) {
		t.Error("the second server does not answer after the switch")
	}
	if rs := s.a.RunState(); rs.State != "connected" || rs.ProfileName != "second" {
		t.Errorf("the run state is %+v", rs)
	}
}
