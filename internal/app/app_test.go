// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/storage"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

func TestMain(m *testing.M) {
	if sandbox.IsKeeper() {
		os.Exit(sandbox.KeeperMain())
	}
	sandboxtest.HandleProbe()
	nstest.Main(m, nil)
}

// open opens an App in isolated folders, with the target argv; it is closed when the test ends.
func open(t *testing.T, argv ...string) (*App, storage.Dirs) {
	t.Helper()
	dirs := storagetest.Isolate(t)
	a, err := Open(context.Background(), Options{Argv: argv})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	return a, dirs
}

// logText is the content of the App's log.
func logText(t *testing.T, dirs storage.Dirs) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dirs.State, logFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mode is the permission bits of a file.
func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestOpen(t *testing.T) {
	_, dirs := open(t)
	for _, dir := range []string{dirs.Config, dirs.State} {
		if m := mode(t, dir); m != 0o700 {
			t.Errorf("%s has the mode %v", dir, m)
		}
	}
	if m := mode(t, filepath.Join(dirs.State, logFile)); m != 0o600 {
		t.Errorf("the log has the mode %v", m)
	}
	if log := logText(t, dirs); !strings.Contains(log, `"msg":"tube started"`) || !strings.Contains(log, Version()) {
		t.Errorf("the log is:\n%s", log)
	}
	t.Run("no home", func(t *testing.T) {
		nothing := func(string) (string, bool) { return "", false }
		_, err := Open(context.Background(), Options{Env: nothing})
		errtest.Route(t, err, "Open->Resolve.StorageHomeUnknown")
	})
}

// addProfile adds a profile with the options to the App's store and returns it.
func addProfile(t *testing.T, a *App, name string, opts profilestest.Options) profiles.Profile {
	t.Helper()
	path := profilestest.WriteProfile(t, t.TempDir(), "p.ovpn", profilestest.NewPKI(t), opts)
	p, err := a.store.Add(context.Background(), path, name, profiles.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// setName changes the name in a profile's meta.json by hand, as the store would refuse it.
func setName(t *testing.T, dirs storage.Dirs, id, name string) {
	t.Helper()
	path := filepath.Join(dirs.Config, "profiles", id, "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m["name"] = name
	if data, err = json.Marshal(m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindProfile(t *testing.T) {
	a, dirs := open(t)
	work := addProfile(t, a, "Work", profilestest.Options{Password: true})
	addProfile(t, a, "work2", profilestest.Options{})
	addProfile(t, a, "HOME", profilestest.Options{})
	home := addProfile(t, a, "other", profilestest.Options{})
	setName(t, dirs, home.ID, "home") // the store does not let two names differ only by case
	broken := addProfile(t, a, "Broken", profilestest.Options{})
	if err := os.WriteFile(filepath.Join(broken.FilesDir, broken.File), []byte("remote \"x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetUsername(context.Background(), work.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, want, route string
	}{
		{"Work", work.ID, ""},
		{"work", work.ID, ""},
		{"home", home.ID, ""},
		{"Home", "", "App.FindProfile.AppProfileAmbiguous"},
		{"nothing", "", "App.FindProfile.AppProfileNotFound"},
		{"Broken", "", "App.FindProfile.AppProfileBroken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, needsPassword, username, err := a.FindProfile(context.Background(), tt.name)
			if tt.route != "" {
				meta := errtest.Route(t, err, tt.route)
				if meta["name"] != tt.name {
					t.Errorf("metadata = %v", meta)
				}
				return
			}
			if err != nil || id != tt.want {
				t.Fatalf("FindProfile = %q, %v, want %q", id, err, tt.want)
			}
			if id == work.ID && (!needsPassword || username != "alice") {
				t.Errorf("Work: needs a password %v, user name %q", needsPassword, username)
			}
		})
	}
}

func TestConnectNeedsCredentials(t *testing.T) {
	a, dirs := open(t)
	p := addProfile(t, a, "Work", profilestest.Options{Password: true})
	err := a.Connect(context.Background(), p.ID, nil, false)
	errtest.Route(t, err, "Dial.VPNCredentialsNeeded")
	// A server that does not answer: the credentials go nowhere, and stay out of the log.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := a.Connect(ctx, p.ID, &Credentials{Username: "alice", Password: "hunter2"}, true); err == nil {
		t.Fatal("a connection to nothing worked")
	}
	log := logText(t, dirs)
	if strings.Count(log, `"msg":"connect failed"`) != 2 || strings.Contains(log, "hunter2") {
		t.Errorf("the log is:\n%s", log)
	}
	if got, err := a.store.Get(context.Background(), p.ID); err != nil || got.Username != "" || !got.LastUsed.IsZero() {
		t.Errorf("a failed connect changed the profile: %+v, %v", got, err)
	}
}

// fakeNetwork is the network of the fake tunnels.
var fakeNetwork = vpn.Network{
	Addrs: []netip.Prefix{netip.MustParsePrefix("10.9.0.2/24")}, DNSServers: []netip.Addr{netip.MustParseAddr("10.9.0.1")}, MTU: 1400,
}

func TestRunWithoutConnect(t *testing.T) {
	a, _ := open(t, "true")
	status, err := a.Run(context.Background())
	errtest.Route(t, err, "App.Run.AppNotConnected")
	if status != 125 {
		t.Errorf("the status is %d", status)
	}
}

func TestRunWithoutTarget(t *testing.T) {
	a, _ := open(t)
	tun := vpntest.NewTunnel(fakeNetwork)
	a.setTunnel(tun, profiles.Profile{ID: "0123456789abcdef", Name: "fake"})
	_, err := a.Run(context.Background())
	errtest.Route(t, err, "App.Run.AppNoTarget")
	if !tun.Closed() {
		t.Error("the tunnel is not closed")
	}
}

func TestRunWithAFakeTunnel(t *testing.T) {
	nstest.Require(t)
	a, dirs := open(t, "sh", "-c", "exit 4")
	if got := a.Target(); strings.Join(got, " ") != "sh -c exit 4" {
		t.Errorf("Target = %q", got)
	}
	tun := vpntest.NewTunnel(fakeNetwork)
	a.setTunnel(tun, profiles.Profile{ID: "0123456789abcdef", Name: "fake"})
	status, err := a.Run(context.Background())
	if err != nil || status != 4 {
		t.Errorf("Run = %d, %v", status, err)
	}
	if !tun.Closed() {
		t.Error("the tunnel is not closed")
	}
	log := logText(t, dirs)
	for _, want := range []string{`"msg":"run started"`, `"msg":"run ended"`} {
		if !strings.Contains(log, want) {
			t.Errorf("the log has no %s:\n%s", want, log)
		}
	}
}

func TestRunAProgramThatIsNotThere(t *testing.T) {
	a, _ := open(t, "tube-no-such-program")
	tun := vpntest.NewTunnel(fakeNetwork)
	a.setTunnel(tun, profiles.Profile{ID: "0123456789abcdef", Name: "fake"})
	status, err := a.Run(context.Background())
	errtest.Route(t, err, "LookPath.SandboxLookPathFailed")
	if status != 127 || !tun.Closed() {
		t.Errorf("the status is %d, the tunnel closed: %v", status, tun.Closed())
	}
}
