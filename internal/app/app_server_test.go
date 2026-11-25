// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package app

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// addServer adds the profile of the server to the App's store.
func addServer(t *testing.T, a *App, server *vpntest.Server) profiles.Profile {
	t.Helper()
	p, err := a.store.Add(context.Background(), server.ProfilePath(), "test", profiles.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// connect connects with a context of 20 seconds.
func connect(t *testing.T, a *App, id string, creds *Credentials, remember bool) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return a.Connect(ctx, id, creds, Remember{User: remember})
}

func TestConnectAndRun(t *testing.T) {
	if _, err := exec.LookPath("ping"); err != nil {
		t.Skip("ping is not installed")
	}
	server := vpntest.StartServer(t, vpntest.ServerOptions{})
	a, _ := open(t, "ping", "-c", "1", "-W", "2", server.TunnelAddr().String())
	p := addServer(t, a, server)
	if err := connect(t, a, p.ID, nil, false); err != nil {
		t.Fatalf("Connect: %v\n%s", err, server.Log())
	}
	status, err := a.Run(context.Background())
	if err != nil || status != 0 {
		t.Errorf("Run = %d, %v\n%s", status, err, server.Log())
	}
	if got, err := a.store.Get(context.Background(), p.ID); err != nil || got.LastUsed.IsZero() {
		t.Errorf("the profile is not marked as used: %+v, %v", got, err)
	}
}

func TestConnectRemembersTheUserName(t *testing.T) {
	server := vpntest.StartServer(t, vpntest.ServerOptions{Username: "alice", Password: "right"})
	a, _ := open(t)
	p := addServer(t, a, server)
	if err := connect(t, a, p.ID, &Credentials{Username: "alice", Password: "right"}, true); err != nil {
		t.Fatalf("Connect: %v\n%s", err, server.Log())
	}
	if got, err := a.store.Get(context.Background(), p.ID); err != nil || got.Username != "alice" {
		t.Errorf("the user name is not remembered: %q, %v", got.Username, err)
	}
}

func TestConnectWithTheRememberedPassword(t *testing.T) {
	server := vpntest.StartServer(t, vpntest.ServerOptions{Username: "alice", Password: "right"})
	a, dirs := open(t)
	p := addServer(t, a, server)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Without credentials and with nothing remembered, there is no login.
	if err := a.Connect(ctx, p.ID, nil, Remember{}); err == nil {
		t.Fatal("a profile with a password connected without one")
	}
	if err := a.Connect(ctx, p.ID, &Credentials{Username: "alice", Password: "right"}, Remember{Password: true}); err != nil {
		t.Fatalf("Connect: %v\n%s", err, server.Log())
	}
	got, err := a.store.Get(ctx, p.ID)
	if err != nil || got.Username != "alice" || !got.HasPassword {
		t.Fatalf("after the connect the profile is %+v, %v", got, err)
	}
	// The next connect gives nothing: the remembered user name and password are used.
	if err := a.Connect(ctx, p.ID, nil, Remember{}); err != nil {
		t.Fatalf("Connect with the remembered password: %v\n%s", err, server.Log())
	}
	if got, err := a.store.Get(ctx, p.ID); err != nil || !got.HasPassword {
		t.Errorf("a connect that gave nothing forgot the password: %+v, %v", got, err)
	}
	// Credentials that are given win over the remembered ones, and a script changes nothing.
	err = a.Connect(ctx, p.ID, &Credentials{Username: "alice", Password: "wrong"}, Remember{})
	if vpn.KindOf(err) != vpn.KindAuthFailed {
		t.Fatalf("Connect with a wrong password = %v, want the login refused", err)
	}
	// A remembered password that the server refuses is a refused login, and stays until the
	// user gives another one or forgets it.
	if err := a.store.SetPassword(ctx, p.ID, "stale"); err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(ctx, p.ID, nil, Remember{}); vpn.KindOf(err) != vpn.KindAuthFailed {
		t.Fatalf("Connect with a stale remembered password = %v, want the login refused", err)
	}
	if err := a.Connect(ctx, p.ID, &Credentials{Username: "alice", Password: "right"}, Remember{User: true, Forget: true}); err != nil {
		t.Fatalf("Connect: %v\n%s", err, server.Log())
	}
	if got, err := a.store.Get(ctx, p.ID); err != nil || got.HasPassword {
		t.Errorf("the password was not forgotten: %+v, %v", got, err)
	}
	if log := logText(t, dirs); strings.Contains(log, "right") || strings.Contains(log, "stale") {
		t.Error("a password is in the log")
	}
}

func TestAWrongPassword(t *testing.T) {
	server := vpntest.StartServer(t, vpntest.ServerOptions{Username: "alice", Password: "right"})
	a, _ := open(t)
	p := addServer(t, a, server)
	err := connect(t, a, p.ID, &Credentials{Username: "alice", Password: "wrong"}, true)
	if vpn.KindOf(err) != vpn.KindAuthFailed {
		t.Fatalf("Connect = %v, want the login refused", err)
	}
	if got := Explain(err); got != "The VPN server refused the user name or the password. (AuthFailed)" {
		t.Errorf("Explain = %q", got)
	}
	if got, err := a.store.Get(context.Background(), p.ID); err != nil || got.Username != "" {
		t.Errorf("a refused login remembered the user name: %q, %v", got.Username, err)
	}
}
