// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
)

// counter subscribes to the App and counts the calls.
func counter(t *testing.T, a *App) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	cancel := a.Subscribe(func() { n.Add(1) })
	t.Cleanup(cancel)
	return &n
}

// profileFile writes a profile file for the tests and returns its path.
func profileFile(t *testing.T, name string, opts profilestest.Options) string {
	t.Helper()
	return profilestest.WriteProfile(t, t.TempDir(), name, profilestest.NewPKI(t), opts)
}

func TestSnapshot(t *testing.T) {
	a, dirs := open(t, "firefox", "-P", "a\x1b[2Jb")
	work := addProfile(t, a, "Work", profilestest.Options{Password: true, Remote: "vpn.example.com 1194"})
	home := addProfile(t, a, "home", profilestest.Options{})
	broken := addProfile(t, a, "Broken", profilestest.Options{})
	if err := os.WriteFile(filepath.Join(broken.FilesDir, broken.File), []byte("remote \"x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setName(t, dirs, home.ID, "home\x1b]0;evil\x07")
	snap, err := a.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(snap.Target, " "); got != "firefox -P "+sanitize.Display("a\x1b[2Jb", 255) {
		t.Errorf("the target is %q", got)
	}
	var names []string
	for _, p := range snap.Profiles {
		names = append(names, p.Name)
		for _, c := range p.Name {
			if c < 0x20 {
				t.Errorf("an unsafe name: %q", p.Name)
			}
		}
	}
	if len(names) != 3 || names[0] != "Broken" || names[2] != "Work" {
		t.Errorf("the profiles are %q", names)
	}
	w := snap.Profiles[2]
	if w.ID != work.ID || !w.NeedsPassword || !slices.Equal(w.Remotes, []string{"vpn.example.com:1194 udp"}) || len(w.Files) != 0 {
		t.Errorf("Work is %+v", w)
	}
	if snap.Profiles[0].Broken == "" {
		t.Error("the broken profile is not broken")
	}
	if snap.Now.IsZero() {
		t.Error("no time")
	}
}

func TestCommands(t *testing.T) {
	a, dirs := open(t)
	n := counter(t, a)
	ctx := context.Background()
	check := func(t *testing.T, err error, route string, calls int32) {
		t.Helper()
		if route != "" {
			errtest.Route(t, err, route)
		} else if err != nil {
			t.Fatal(err)
		}
		if got := n.Swap(0); got != calls {
			t.Errorf("the subscriber was called %d times, want %d", got, calls)
		}
	}
	upper := filepath.Join(t.TempDir(), "Work.OVPN")
	if err := os.Rename(profileFile(t, "w.ovpn", profilestest.Options{}), upper); err != nil {
		t.Fatal(err)
	}
	id, err := a.AddProfile(ctx, upper, "", false)
	check(t, err, "", 1)
	if p, err := a.store.Get(ctx, id); err != nil || p.Name != "Work" {
		t.Errorf("the default name is %q, %v", p.Name, err)
	}
	home, _ := os.LookupEnv("HOME")
	if err := os.Rename(profileFile(t, "x.ovpn", profilestest.Options{}), filepath.Join(home, "x.ovpn")); err != nil {
		t.Fatal(err)
	}
	id2, err := a.AddProfile(ctx, "~/x.ovpn", "", false)
	check(t, err, "", 1)
	if p, err := a.store.Get(ctx, id2); err != nil || p.Name != "x" {
		t.Errorf("the profile of the home folder is %q, %v", p.Name, err)
	}
	_, err = a.AddProfile(ctx, filepath.Join(dirs.Config, "missing.ovpn"), "", false)
	check(t, err, "Store.Add", 0)
	check(t, a.RenameProfile(ctx, id, "Office"), "", 1)
	check(t, a.RenameProfile(ctx, id2, "office"), "ProfileNameTaken", 0)
	check(t, a.SetUsername(ctx, id, "ada"), "", 1)
	check(t, a.SetDNS(ctx, id, "1.1.1.1, 9.9.9.9"), "", 1)
	if p, err := a.store.Get(ctx, id); err != nil || p.Username != "ada" || len(p.DNS) != 2 || p.Name != "Office" {
		t.Errorf("the profile is %+v, %v", p, err)
	}
	check(t, a.SetDNS(ctx, id, ""), "", 1)
	check(t, a.SetDNS(ctx, id, "x"), "ParseDNS.ProfileDNSInvalid", 0)
	check(t, a.DeleteProfile(ctx, id), "", 1)
	check(t, a.DeleteProfile(ctx, id), "ProfileNotFound", 0)
}

func TestAddWithScripts(t *testing.T) {
	a, _ := open(t)
	path := profileFile(t, "s.ovpn", profilestest.Options{Extra: []string{"script-security 2", "up /bin/up.sh", "down /bin/down.sh"}})
	_, err := a.AddProfile(context.Background(), path, "s", false)
	options, ok := IsRunsScripts(err)
	if !ok || options != "up, down" {
		t.Fatalf("IsRunsScripts = %q, %v (err %v)", options, ok, err)
	}
	if _, err := a.AddProfile(context.Background(), path, "s", true); err != nil {
		t.Errorf("with the scripts disabled: %v", err)
	}
	if _, ok := IsRunsScripts(nil); ok {
		t.Error("IsRunsScripts(nil) is true")
	}
	if IsAuthFailed(err) || !IsAuthFailed(tagged("AuthFailed")) {
		t.Error("IsAuthFailed is wrong")
	}
}

func TestEdit(t *testing.T) {
	a, _ := open(t)
	n := counter(t, a)
	p := addProfile(t, a, "Work", profilestest.Options{})
	ctx := context.Background()
	old, err := a.store.Text(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	begin := func() string {
		t.Helper()
		path, err := a.BeginEdit(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != string(old) {
			t.Errorf("the file of the edit has %q, %v", data, err)
		}
		if m := mode(t, path); m != 0o600 {
			t.Errorf("the file of the edit has the mode %v", m)
		}
		return path
	}
	gone := func(path string) {
		t.Helper()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("the file of the edit is left: %v", err)
		}
	}
	t.Run("a new text", func(t *testing.T) {
		path := begin()
		newText := string(old) + "\n# edited\n"
		old = []byte(newText)
		if err := os.WriteFile(path, old, 0o600); err != nil {
			t.Fatal(err)
		}
		changed, err := a.EndEdit(ctx, p.ID, path)
		if err != nil || !changed || n.Swap(0) != 1 {
			t.Errorf("EndEdit = %v, %v", changed, err)
		}
		if got, _ := a.store.Text(ctx, p.ID); string(got) != newText {
			t.Error("the store does not have the new text")
		}
		gone(path)
	})
	t.Run("the same text", func(t *testing.T) {
		path := begin()
		if changed, err := a.EndEdit(ctx, p.ID, path); err != nil || changed {
			t.Errorf("EndEdit = %v, %v", changed, err)
		}
		gone(path)
	})
	t.Run("a text that the parser refuses", func(t *testing.T) {
		path := begin()
		if err := os.WriteFile(path, []byte("remote \"x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		changed, err := a.EndEdit(ctx, p.ID, path)
		if err == nil || changed {
			t.Errorf("EndEdit = %v, %v", changed, err)
		}
		if got, _ := a.store.Text(ctx, p.ID); string(got) != string(old) {
			t.Error("the store lost the old text")
		}
		gone(path)
	})
	t.Run("a path that was not given", func(t *testing.T) {
		_, err := a.EndEdit(ctx, p.ID, "/etc/passwd")
		errtest.Route(t, err, "App.EndEdit.AppEditUnknown")
	})
}

func TestSubscribe(t *testing.T) {
	dirs := storagetest.Isolate(t)
	a, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var first, second atomic.Int32
	cancelFirst := a.Subscribe(func() { first.Add(1) })
	cancelSecond := a.Subscribe(func() { second.Add(1) })
	defer cancelSecond()
	p := addProfile(t, a, "Work", profilestest.Options{})
	if err := a.RenameProfile(context.Background(), p.ID, "Office"); err != nil {
		t.Fatal(err)
	}
	cancelFirst()
	if err := a.RenameProfile(context.Background(), p.ID, "Home"); err != nil {
		t.Fatal(err)
	}
	if first.Load() != 1 || second.Load() != 2 {
		t.Errorf("the subscribers were called %d and %d times", first.Load(), second.Load())
	}
	if _, err := a.BeginEdit(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dirs.TempBase, "tube-*")); len(left) > 0 {
		t.Errorf("Close left %v", left)
	}
}
