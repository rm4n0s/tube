// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package vpntest

import (
	"net/netip"
	"strings"
	"testing"
)

func TestServerStartsAndStops(t *testing.T) {
	s := StartServer(t, ServerOptions{})
	if !s.Running() {
		t.Fatal("the server does not run after StartServer")
	}
	s.Stop()
	if s.Running() {
		t.Fatal("the server still runs after Stop")
	}
	s.Stop() // a second Stop does nothing
	s.Start()
	if !s.Running() {
		t.Fatal("the server does not run after Start")
	}
	if n := strings.Count(s.Log(), readyLine); n != 2 {
		t.Errorf("the log has %d ready lines, want 2:\n%s", n, s.Log())
	}
}

func TestServerSubnet(t *testing.T) {
	s := StartServer(t, ServerOptions{Subnet: netip.MustParsePrefix("10.9.0.0/24")})
	if got := s.TunnelAddr().String(); got != "10.9.0.1" {
		t.Errorf("TunnelAddr = %s", got)
	}
	if !strings.Contains(strings.Join(s.args, " "), "--server 10.9.0.0 255.255.255.0") {
		t.Errorf("the arguments are %q", s.args)
	}
	if got := StartServer(t, ServerOptions{}).TunnelAddr().String(); got != "10.8.0.1" {
		t.Errorf("the default TunnelAddr = %s", got)
	}
}
