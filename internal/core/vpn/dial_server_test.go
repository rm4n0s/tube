// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package vpn_test

import (
	"context"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/packettest"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// storedProfile adds the server's profile to a new isolated store.
func storedProfile(t *testing.T, s *vpntest.Server) profiles.Profile {
	t.Helper()
	dirs := storagetest.Isolate(t)
	store := profiles.NewStore(dirs.Config, clock.System())
	p, err := store.Add(context.Background(), s.ProfilePath(), "test", profiles.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// dial connects the profile with a context of 20 seconds.
func dial(t *testing.T, p profiles.Profile, creds *vpn.Credentials) (vpn.Tunnel, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return vpn.Dial(ctx, p, creds)
}

// mustDial connects, fails the test when it cannot, and closes the tunnel when the test ends.
func mustDial(t *testing.T, s *vpntest.Server, creds *vpn.Credentials) vpn.Tunnel {
	t.Helper()
	tun, err := dial(t, storedProfile(t, s), creds)
	if err != nil {
		t.Fatalf("Dial: %v\nthe server's log:\n%s", err, s.Log())
	}
	t.Cleanup(func() {
		if err := tun.Close(); err != nil {
			t.Error(err)
		}
	})
	return tun
}

// pingServer sends an ICMP echo request to the server's tunnel address and waits five seconds
// for its reply.
func pingServer(t *testing.T, tun vpn.Tunnel, src, dst netip.Addr) {
	t.Helper()
	const id = 0x7475
	var request []byte
	if src.Is4() {
		request = packettest.EchoRequest4(src, dst, id, 1)
	} else {
		request = packettest.EchoRequest6(src, dst, id, 1)
	}
	if _, err := tun.Write(request); err != nil {
		t.Fatal(err)
	}
	if err := tun.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2000)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			t.Fatalf("no echo reply from %v: %v", dst, err)
		}
		if packettest.IsEchoReply4(buf[:n], id) || packettest.IsEchoReply6(buf[:n], id) {
			return
		}
	}
}

func TestDialRealServer(t *testing.T) {
	dns := netip.MustParseAddr("10.8.0.1")
	s := vpntest.StartServer(t, vpntest.ServerOptions{DNS: []netip.Addr{dns}, SearchDomain: "tube.test"})
	tun := mustDial(t, s, nil)
	n := tun.Network()
	if want := []netip.Prefix{netip.MustParsePrefix("10.8.0.2/24")}; !slices.Equal(n.Addrs, want) {
		t.Errorf("addresses = %v, want %v", n.Addrs, want)
	}
	if n.MTU != 1500 {
		t.Errorf("MTU = %d, want 1500", n.MTU)
	}
	if !slices.Equal(n.DNSServers, []netip.Addr{dns}) {
		t.Errorf("DNS servers = %v", n.DNSServers)
	}
	if !slices.Equal(n.SearchDomains, []string{"tube.test"}) {
		t.Errorf("search domains = %v", n.SearchDomains)
	}
}

func TestPingThroughTheTunnel(t *testing.T) {
	s := vpntest.StartServer(t, vpntest.ServerOptions{})
	tun := mustDial(t, s, nil)
	pingServer(t, tun, tun.Network().Addrs[0].Addr(), s.TunnelAddr())
}

func TestDialWithPassword(t *testing.T) {
	const password = "TUBE-test-password-5d2e"
	s := vpntest.StartServer(t, vpntest.ServerOptions{Username: "ada", Password: password})
	t.Run("the right password", func(t *testing.T) {
		tun := mustDial(t, s, &vpn.Credentials{Username: "ada", Password: password})
		pingServer(t, tun, tun.Network().Addrs[0].Addr(), s.TunnelAddr())
	})
	t.Run("a wrong password", func(t *testing.T) {
		const wrong = "TUBE-wrong-password-91aa"
		_, err := dial(t, storedProfile(t, s), &vpn.Credentials{Username: "ada", Password: wrong})
		if vpn.KindOf(err) != vpn.KindAuthFailed {
			t.Fatalf("err = %v, want a refused login", err)
		}
		if strings.Contains(err.Error(), wrong) {
			t.Error("the error's message has the password")
		}
		if appErr, ok := errors.FromError(err); ok {
			for k, v := range appErr.Metadata {
				if text, ok := v.(string); ok && strings.Contains(text, wrong) {
					t.Errorf("the metadata %q has the password", k)
				}
			}
		}
	})
}

func TestDialOverTCP(t *testing.T) {
	s := vpntest.StartServer(t, vpntest.ServerOptions{Proto: "tcp"})
	tun := mustDial(t, s, nil)
	pingServer(t, tun, tun.Network().Addrs[0].Addr(), s.TunnelAddr())
}

func TestIPv6(t *testing.T) {
	s := vpntest.StartServer(t, vpntest.ServerOptions{PushIPv6: true})
	tun := mustDial(t, s, nil)
	addrs := tun.Network().Addrs
	if len(addrs) != 2 || !addrs[0].Addr().Is4() || !addrs[1].Addr().Is6() {
		t.Fatalf("addresses = %v, want IPv4 then IPv6", addrs)
	}
	pingServer(t, tun, addrs[1].Addr(), s.TunnelAddr6())
}
