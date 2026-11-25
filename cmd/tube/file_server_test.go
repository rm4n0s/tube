// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package main

import (
	"context"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

const (
	// testName is a name that only the DNS server of the test knows, and testNameAddr what it
	// answers.
	testName     = "tube-test.example"
	testNameAddr = "192.0.2.7"
	// testPassword is the password of the servers of these tests: no output may have it.
	testPassword = "s3cret-pw"
	noTerminal   = "tube: the profile needs a user name and a password and there is no terminal to ask on; use --auth-file, or --username and --password (TubeNoTerminal)\n"
)

// only is the one profile of the store.
func (e e2e) only(t *testing.T) profiles.Profile {
	t.Helper()
	list, err := e.store().List(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("the profiles are %+v, %v", list, err)
	}
	return list[0]
}

// startNames starts a DNS server of the test on the server's side of the tunnel.
func startNames(t *testing.T, server *vpntest.Server) {
	t.Helper()
	vpntest.StartDNS(t, netip.AddrPortFrom(server.TunnelAddr(), 53), map[string]netip.Addr{testName: netip.MustParseAddr(testNameAddr)})
}

func TestTheProfileFile(t *testing.T) {
	bin := buildTube(t)
	t.Run("a file is added, then used as the profile it is", func(t *testing.T) {
		e, server := setupWithoutProfile(t, bin, vpntest.ServerOptions{})
		path := server.ProfilePath()
		status, _, stderr := e.run(t, "--profile", path, "true")
		if status != 0 || !strings.HasPrefix(stderr, "tube: added the profile \"test\"\n") || !strings.Contains(stderr, "tube: connected to test") {
			t.Fatalf("status %d, standard error:\n%s", status, stderr)
		}
		// No terminal is there to ask on: nothing was asked, and the profile has no DNS servers.
		if p := e.only(t); p.Name != "test" || len(p.DNS) != 0 {
			t.Errorf("the profile is %+v", p)
		}
		for _, value := range []string{path, strings.TrimSuffix(path, ".ovpn"), "test", "test.ovpn"} {
			status, _, stderr := e.run(t, "--profile", value, "true")
			if status != 0 || strings.Contains(stderr, "added") {
				t.Errorf("--profile %s: status %d, standard error:\n%s", value, status, stderr)
			}
		}
		e.only(t)
	})
	t.Run("without .ovpn, quiet", func(t *testing.T) {
		e, server := setupWithoutProfile(t, bin, vpntest.ServerOptions{})
		status, _, stderr := e.run(t, "--quiet", "--profile", strings.TrimSuffix(server.ProfilePath(), ".ovpn"), "true")
		if status != 0 || stderr != "" {
			t.Fatalf("status %d, standard error %q", status, stderr)
		}
		e.only(t)
	})
	t.Run("neither a profile nor a file", func(t *testing.T) {
		e, server := setupWithoutProfile(t, bin, vpntest.ServerOptions{})
		missing := filepath.Join(filepath.Dir(server.ProfilePath()), "missing")
		status, _, stderr := e.run(t, "--profile", missing, "true")
		if want := "tube: There is no profile called \"" + missing + "\", and no .ovpn file at that path. (AppNoProfileOrFile)\n"; status != 125 || stderr != want {
			t.Errorf("status %d, standard error %q", status, stderr)
		}
	})
	t.Run("the login of the options", func(t *testing.T) {
		e, server := setupWithoutProfile(t, bin, vpntest.ServerOptions{Username: "alice", Password: testPassword})
		path := server.ProfilePath()
		// Without a login and without a terminal the file is added, and that is all.
		status, _, stderr := e.run(t, "--quiet", "--profile", path, "true")
		if status != 125 || stderr != noTerminal {
			t.Errorf("without a login: status %d, standard error %q", status, stderr)
		}
		// A wrong password, and the right one: neither is kept without --save.
		status, _, stderr = e.run(t, "--quiet", "--profile", path, "--username", "alice", "--password", "wrong-"+testPassword, "true")
		if status != 125 || stderr != "tube: The VPN server refused the user name or the password. (AuthFailed)\n" {
			t.Errorf("a wrong password: status %d, standard error %q", status, stderr)
		}
		if status, _, stderr := e.run(t, "--quiet", "--profile", path, "--username", "alice", "--password", testPassword, "true"); status != 0 || stderr != "" {
			t.Errorf("the login of the options: status %d, standard error %q", status, stderr)
		}
		if p := e.only(t); p.Username != "" || p.HasPassword {
			t.Errorf("a login was kept without --save: user name %q, password %v", p.Username, p.HasPassword)
		}
		if status, _, stderr := e.run(t, "--quiet", "--profile", "test", "true"); status != 125 || stderr != noTerminal {
			t.Errorf("after a run without --save: status %d, standard error %q", status, stderr)
		}
		// A refused login is not kept, --save or not.
		e.run(t, "--quiet", "--profile", "test", "--username", "alice", "--password", "wrong-"+testPassword, "--save", "true")
		if p := e.only(t); p.Username != "" || p.HasPassword {
			t.Errorf("a refused login was kept: user name %q, password %v", p.Username, p.HasPassword)
		}
		// With --save it is kept, in a file that only the user reads, and the next run needs
		// nothing.
		if status, _, stderr := e.run(t, "--quiet", "--profile", "test", "--username", "alice", "--password", testPassword, "--save", "true"); status != 0 || stderr != "" {
			t.Errorf("with --save: status %d, standard error %q", status, stderr)
		}
		p := e.only(t)
		if p.Username != "alice" || !p.HasPassword {
			t.Fatalf("the login was not kept: user name %q, password %v", p.Username, p.HasPassword)
		}
		if got, err := e.store().Password(context.Background(), p.ID); err != nil || got != testPassword {
			t.Errorf("the kept password is not the one of --password: %v", err)
		}
		info, err := os.Stat(filepath.Join(e.dir("TUBE_CONFIG_DIR"), "profiles", p.ID, "password"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("the file of the password: %v, %v", info, err)
		}
		if status, _, stderr := e.run(t, "--quiet", "--profile", "test", "true"); status != 0 || stderr != "" {
			t.Errorf("with the kept login: status %d, standard error %q", status, stderr)
		}
		// The password is in no log.
		logs, err := filepath.Glob(filepath.Join(e.dir("TUBE_STATE_DIR"), "*"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range logs {
			if data, err := os.ReadFile(name); err == nil && strings.Contains(string(data), testPassword) {
				t.Errorf("%s has the password", name)
			}
		}
	})
	t.Run("--dns", func(t *testing.T) {
		if _, err := exec.LookPath("getent"); err != nil {
			t.Skip("getent is not installed")
		}
		// The server pushes no DNS server: without one of its own the application has none.
		e, server := setup(t, bin, vpntest.ServerOptions{})
		startNames(t, server)
		dns := server.TunnelAddr().String()
		lookup := func(options ...string) bool {
			t.Helper()
			args := append([]string{"--quiet", "--profile", "test"}, options...)
			status, stdout, _ := e.run(t, append(args, "getent", "hosts", testName)...)
			return status == 0 && strings.Contains(stdout, testNameAddr)
		}
		if lookup() {
			t.Fatal("the name is found without a DNS server")
		}
		if !lookup("--dns", dns) {
			t.Error("the name is not found with --dns")
		}
		if p := e.only(t); len(p.DNS) != 0 {
			t.Errorf("--dns without --save changed the profile: %v", p.DNS)
		}
		if lookup() {
			t.Error("the DNS server of one run was used by the next")
		}
		status, _, stderr := e.run(t, "--profile", "test", "--dns", "dns.example", "true")
		if status != 125 || stderr != "tube: \"dns.example\" is not an IP address. (ProfileDNSInvalid)\n" {
			t.Errorf("a wrong --dns: status %d, standard error %q", status, stderr)
		}
		if !lookup("--dns", dns, "--save") {
			t.Error("the name is not found with --dns and --save")
		}
		if p := e.only(t); len(p.DNS) != 1 || p.DNS[0].String() != dns {
			t.Errorf("--save did not keep the DNS server: %v", p.DNS)
		}
		if !lookup() {
			t.Error("the kept DNS server is not used")
		}
		// An empty --dns is the servers of the VPN, here none, for this run only.
		if lookup("--dns", "") {
			t.Error("the name is found with an empty --dns")
		}
		if !lookup() {
			t.Error("an empty --dns without --save changed the profile")
		}
		if lookup("--dns", "", "--save") || lookup() {
			t.Error("an empty --dns with --save did not take the DNS servers from the profile")
		}
	})
}

func TestTheQuestionsOnTheTerminal(t *testing.T) {
	if _, err := exec.LookPath("getent"); err != nil {
		t.Skip("getent is not installed")
	}
	e, server := setupWithoutProfile(t, buildTube(t), vpntest.ServerOptions{Username: "alice", Password: testPassword})
	startNames(t, server)
	p := e.inTerminal(t, "--profile", server.ProfilePath(), "getent", "hosts", testName)
	p.screen.waitFor(t, `tube: added the profile "test"`)
	p.screen.waitFor(t, "User name: ")
	p.press(t, "alice\n")
	p.screen.waitFor(t, "Password: ")
	p.press(t, testPassword+"\n")
	p.screen.waitFor(t, "DNS servers (empty: the ones of the VPN): ")
	p.press(t, "not-an-address\n")
	p.screen.waitFor(t, `"not-an-address" is not an IP address.`)
	p.press(t, server.TunnelAddr().String()+"\n")
	p.screen.waitFor(t, "Save the user name, the password and the DNS servers with the profile? [Y/n] ")
	p.press(t, "\n")
	p.screen.waitFor(t, testNameAddr)
	if status := p.wait(t, server); status != 0 {
		t.Errorf("the status is %d; the screen:\n%s", status, ansi.Strip(p.screen.buf.String()))
	}
	if strings.Contains(p.screen.buf.String(), testPassword) {
		t.Error("the terminal showed the password")
	}
	// Everything was saved: the next run asks nothing, also where nothing can be asked.
	got := e.only(t)
	if got.Username != "alice" || !got.HasPassword || len(got.DNS) != 1 {
		t.Errorf("the profile is: user name %q, password %v, DNS %v", got.Username, got.HasPassword, got.DNS)
	}
	status, stdout, stderr := e.run(t, "--quiet", "--profile", "test", "getent", "hosts", testName)
	if status != 0 || !strings.Contains(stdout, testNameAddr) || stderr != "" {
		t.Errorf("the next run: status %d, standard output %q, standard error %q", status, stdout, stderr)
	}

	t.Run("not saved", func(t *testing.T) {
		e, server := setup(t, e.bin, vpntest.ServerOptions{Username: "alice", Password: testPassword})
		p := e.inTerminal(t, "--quiet", "--profile", "test", "true")
		p.screen.waitFor(t, "User name: ")
		p.press(t, "alice\n")
		p.screen.waitFor(t, "Password: ")
		p.press(t, testPassword+"\n")
		p.screen.waitFor(t, "DNS servers (empty: the ones of the VPN): ")
		p.press(t, "\n")
		p.screen.waitFor(t, "Save the user name and the password with the profile? [Y/n] ")
		p.press(t, "n\n")
		if status := p.wait(t, server); status != 0 {
			t.Errorf("the status is %d; the screen:\n%s", status, ansi.Strip(p.screen.buf.String()))
		}
		if got := e.only(t); got.Username != "" || got.HasPassword || len(got.DNS) != 0 {
			t.Errorf("the profile changed: user name %q, password %v, DNS %v", got.Username, got.HasPassword, got.DNS)
		}
	})
}
