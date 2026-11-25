// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// realApp opens an app.App in folders of the test and returns it with its home folder. The TUI
// may not import the core's helpers (docs/SPEC.md §4.2 rule 2), so the folders are given through
// the App's environment.
func realApp(t *testing.T) (*app.App, string) {
	t.Helper()
	root := t.TempDir()
	vars := map[string]string{
		"TUBE_CONFIG_DIR": filepath.Join(root, "config"), "TUBE_STATE_DIR": filepath.Join(root, "state"),
		"XDG_RUNTIME_DIR": filepath.Join(root, "run"), "HOME": filepath.Join(root, "home"),
	}
	for _, dir := range []string{vars["XDG_RUNTIME_DIR"], vars["HOME"]} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
	a, err := app.Open(context.Background(), app.Options{Env: env})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	return a, vars["HOME"]
}

// fixtureProfile copies the profile Home of the proofs' fixture into dir, as name, and returns
// its path. Its keys were made for the proofs (scripts/testdata/README.md).
func fixtureProfile(t *testing.T, dir, name string) string {
	t.Helper()
	found, err := filepath.Glob("../../scripts/testdata/three-profiles/profiles/*/files/Home.ovpn")
	if err != nil || len(found) != 1 {
		t.Fatalf("the fixture: %v, %v", found, err)
	}
	data, err := os.ReadFile(found[0])
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGoldenAdd(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		h.key("a")
		h.golden("add-empty")
	})
	t.Run("candidates", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		m := h.m()
		m.readDir = fakeDirs(map[string][]dirEntry{"/vpn/": {{"home.ovpn", false}, {"homelab.ovpn", false}, {"work.ovpn", false}}})
		h.model = m
		h.key("a")
		h.typeText("/vpn/h")
		h.key("tab")
		h.golden("add-candidates")
		if got := h.m().add.path.value(); got != "/vpn/home" {
			t.Errorf("the path is %q", got)
		}
	})
	t.Run("an error of the parser", func(t *testing.T) {
		a, home := realApp(t)
		bad := filepath.Join(home, "bad.ovpn")
		if err := os.WriteFile(bad, []byte("client\nremote 127.0.0.1 1194\ncomp-lzo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		h := newHarness(t, a, 100, 30)
		h.key("a")
		h.typeText("~/bad.ovpn")
		h.key("enter")
		h.has(`The profile uses the option "comp-lzo" on line 3`)
		h.golden("add-error")
		h.key("left")
		if strings.Contains(h.screen(), "comp-lzo") {
			t.Error("the error stayed after a key")
		}
	})
}

func TestAddFlow(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.key("a")
	h.typeText("/tmp/x.ovpn")
	h.key("enter")
	if got := f.Calls(); !slices.Equal(got, []string{"AddProfile(/tmp/x.ovpn, , false)"}) {
		t.Errorf("Calls = %q", got)
	}
	if h.m().page != pageProfiles {
		t.Error("the add page is still open")
	}
	h.has("Added x.")
	t.Run("with a name", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.key("a")
		h.paste("/tmp/y.ovpn")
		h.keys("down")
		h.typeText("Office")
		h.key("enter")
		if got := f.Calls(); !slices.Equal(got, []string{"AddProfile(/tmp/y.ovpn, Office, false)"}) {
			t.Errorf("Calls = %q", got)
		}
		h.has("Added Office.")
	})
}

func TestAddWithScriptsFlow(t *testing.T) {
	runsScripts := errors.New("ProfileRunsScripts", "the profile runs scripts", "options", "up, down")
	for _, answer := range []string{"y", "n"} {
		t.Run(answer, func(t *testing.T) {
			f := apptest.Sample()
			f.Fail = map[string]error{"AddProfile": runsScripts}
			h := newHarness(t, f, 100, 30)
			h.key("a")
			h.typeText("/tmp/s.ovpn")
			h.key("enter")
			h.has("This profile runs scripts (up, down).")
			h.key(answer)
			calls := f.Calls()
			if answer == "y" && (len(calls) != 2 || calls[1] != "AddProfile(/tmp/s.ovpn, , true)") {
				t.Errorf("Calls = %q", calls)
			}
			if answer == "n" && len(calls) != 1 {
				t.Errorf("Calls = %q", calls)
			}
		})
	}
}

func TestAddReal(t *testing.T) {
	a, _ := realApp(t)
	path := fixtureProfile(t, t.TempDir(), "Lab.ovpn")
	h := newHarness(t, a, 100, 30)
	h.key("a")
	h.typeText(path)
	h.key("enter")
	h.has("Added Lab.")
	snap, err := a.Snapshot(context.Background())
	if err != nil || len(snap.Profiles) != 1 {
		t.Fatalf("the snapshot is %+v, %v", snap, err)
	}
	if h.selectedID() != snap.Profiles[0].ID {
		t.Errorf("the new profile is not selected")
	}
}

func TestEscGoesBack(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.key("a")
	h.typeText("/tmp/x.ovpn")
	h.key("esc")
	if h.m().page != pageProfiles || len(f.Calls()) != 0 {
		t.Errorf("esc: page %v, calls %q", h.m().page, f.Calls())
	}
}

func TestDeleteFlow(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.keys("d", "enter")
	if len(f.Calls()) != 0 {
		t.Errorf("enter deleted: %q", f.Calls())
	}
	h.keys("d", "y")
	if got := f.Calls(); !slices.Equal(got, []string{"DeleteProfile(id-home)"}) {
		t.Errorf("Calls = %q", got)
	}
	h.has("Deleted Home.")
	e := newHarness(t, apptest.Empty(), 100, 30)
	e.key("d")
	if e.m().ask != nil {
		t.Error("d on an empty list asked")
	}
}
