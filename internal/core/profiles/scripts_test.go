// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

func TestAddWithScripts(t *testing.T) {
	// setup writes a profile that runs two scripts; the second would make the marker file.
	setup := func(t *testing.T) (*fixture, string, string, []byte) {
		t.Helper()
		f := newFixture(t)
		marker := filepath.Join(t.TempDir(), "ran")
		path := f.write("scripts.ovpn", profilestest.Options{Extra: []string{
			"script-security 2",
			"up /bin/true",
			`down "/bin/sh -c 'touch ` + marker + `'"`,
		}})
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return f, path, marker, original
	}
	noMarker := func(t *testing.T, marker string) {
		t.Helper()
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Error("a script ran: the marker file exists")
		}
	}

	t.Run("without the option it is refused", func(t *testing.T) {
		f, path, marker, original := setup(t)
		_, err := f.store.Add(f.ctx, path, "x", AddOptions{})
		meta := errtest.Route(t, err, "Store.Add.ProfileRunsScripts")
		if meta["options"] != "up, down" {
			t.Errorf("metadata = %v", meta)
		}
		if len(f.folders()) != 0 {
			t.Errorf("the store has %v", f.folders())
		}
		sameBytes(t, path, original)
		noMarker(t, marker)
	})
	t.Run("with the option the lines are disabled", func(t *testing.T) {
		f, path, marker, original := setup(t)
		p, err := f.store.Add(f.ctx, path, "x", AddOptions{WithoutScripts: true})
		if err != nil {
			t.Fatal(err)
		}
		stored, err := f.store.Text(f.ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := bytes.Replace(original, []byte("\nup /bin/true\n"), []byte("\n# tube: disabled: up /bin/true\n"), 1)
		want = bytes.Replace(want, []byte("\ndown "), []byte("\n# tube: disabled: down "), 1)
		if !bytes.Equal(stored, want) {
			t.Errorf("the stored text:\n%s\nwant:\n%s", stored, want)
		}
		sameBytes(t, path, original)
		noMarker(t, marker)
	})
}

func TestWithoutScriptsLimits(t *testing.T) {
	pki := profilestest.NewPKI(t)
	t.Run("too many scripts", func(t *testing.T) {
		f := newFixture(t)
		lines := []string{"script-security 2"}
		for range MaxScriptLines + 1 {
			lines = append(lines, "up /bin/true")
		}
		_, err := f.store.Add(f.ctx, f.write("x.ovpn", profilestest.Options{Extra: lines}), "x", AddOptions{WithoutScripts: true})
		errtest.Route(t, err, "Store.Add->withoutScripts.ProfileTooManyScripts")
	})
	t.Run("exactly the most scripts", func(t *testing.T) {
		f := newFixture(t)
		var lines []string
		for range MaxScriptLines {
			lines = append(lines, "up /bin/true")
		}
		p, err := f.store.Add(f.ctx, f.write("x.ovpn", profilestest.Options{Extra: lines}), "x", AddOptions{WithoutScripts: true})
		if err != nil {
			t.Fatal(err)
		}
		text, err := f.store.Text(f.ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(text), disabledPrefix); n != MaxScriptLines {
			t.Errorf("%d lines disabled, want %d", n, MaxScriptLines)
		}
	})
	t.Run("every script option", func(t *testing.T) {
		var lines []string
		for option := range scriptOptions {
			lines = append(lines, option+" /bin/true")
		}
		dir := t.TempDir()
		profilestest.WriteProfile(t, dir, "x.ovpn", pki, profilestest.Options{Extra: lines})
		text, _, disabled, err := withoutScripts(dir, "x.ovpn")
		if err != nil {
			t.Fatal(err)
		}
		if len(disabled) != len(scriptOptions) {
			t.Errorf("disabled %v, want every one of %d", disabled, len(scriptOptions))
		}
		if n := strings.Count(string(text), disabledPrefix); n != len(scriptOptions) {
			t.Errorf("%d lines disabled", n)
		}
	})
	t.Run("another unsupported option is not disabled", func(t *testing.T) {
		dir := t.TempDir()
		profilestest.WriteProfile(t, dir, "x.ovpn", pki, profilestest.Options{Extra: []string{"up /bin/true", "comp-lzo"}})
		_, _, _, err := withoutScripts(dir, "x.ovpn")
		if got := ProblemOf(err); got.Kind != ProblemUnsupported || got.Option != "comp-lzo" {
			t.Errorf("ProblemOf = %+v (err %v)", got, err)
		}
	})
	t.Run("a profile without scripts is its own text", func(t *testing.T) {
		dir := t.TempDir()
		path := profilestest.WriteProfile(t, dir, "x.ovpn", pki, profilestest.Options{})
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text, _, disabled, err := withoutScripts(dir, "x.ovpn")
		if err != nil || len(disabled) != 0 || !bytes.Equal(text, original) {
			t.Errorf("withoutScripts = %d bytes, %v, %v", len(text), disabled, err)
		}
	})
}
