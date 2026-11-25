// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server && linux

package session

import (
	"context"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// dialServer adds the server's profile to an isolated store and connects it.
func dialServer(t *testing.T, server *vpntest.Server, dirsConfig string) vpn.Tunnel {
	t.Helper()
	store := profiles.NewStore(dirsConfig, clock.System())
	p, err := store.Add(context.Background(), server.ProfilePath(), "test", profiles.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tun, err := vpn.Dial(ctx, p, nil)
	if err != nil {
		t.Fatalf("Dial: %v\nthe server's log:\n%s", err, server.Log())
	}
	return tun
}

func TestThroughARealServer(t *testing.T) {
	tests := []struct {
		name string
		opts vpntest.ServerOptions
		to   func(*vpntest.Server) string
	}{
		{"IPv4", vpntest.ServerOptions{}, func(s *vpntest.Server) string { return s.TunnelAddr().String() }},
		{"IPv6", vpntest.ServerOptions{PushIPv6: true}, func(s *vpntest.Server) string { return s.TunnelAddr6().String() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ping := lookPath(t, "ping")
			server := vpntest.StartServer(t, tt.opts)
			opts, r := options(t, nil, ping, []string{"ping", "-c", "2", "-W", "2", tt.to(server)})
			opts.Tunnel = dialServer(t, server, r.dirs.Config)
			r = start(t, opts, r)
			if status := r.wait(t); status != 0 {
				t.Errorf("ping through the server ended with %d:\n%s%s\nthe server's log:\n%s", status, read(t, r.stdout), read(t, r.stderr), server.Log())
			}
		})
	}
}
