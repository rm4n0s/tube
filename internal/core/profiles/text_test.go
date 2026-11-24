// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

func TestText(t *testing.T) {
	f := newFixture(t)
	path := f.write("work.ovpn", profilestest.Options{})
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.store.Add(f.ctx, path, "work", AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Text(f.ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Text = %q, want the file's bytes", got)
	}
	_, err = f.store.Text(f.ctx, "0123456789abcdef")
	errtest.Route(t, err, "Store.Text->Store.usable->Store.Get->Store.profileDir.ProfileNotFound")
	t.Run("a broken profile", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(p.FilesDir, p.File), []byte("client\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.Text(f.ctx, p.ID)
		meta := errtest.Route(t, err, "Store.Text->Store.usable.ProfileBroken")
		if reason, _ := meta["reason"].(string); !strings.HasPrefix(reason, "its profile cannot be read") {
			t.Errorf("metadata = %v", meta)
		}
	})
}

func TestReplaceText(t *testing.T) {
	setup := func(t *testing.T) (*fixture, Profile, []byte) {
		t.Helper()
		f := newFixture(t)
		p := f.add("work", profilestest.Options{Separate: true})
		old, err := f.store.Text(f.ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return f, p, old
	}
	// filesOf lists the files of the profile's files folder.
	filesOf := func(t *testing.T, p Profile) []string {
		t.Helper()
		entries, err := os.ReadDir(p.FilesDir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}
	t.Run("a valid text", func(t *testing.T) {
		f, p, old := setup(t)
		text := bytes.Replace(old, []byte("remote 127.0.0.1 1194"), []byte("remote 127.0.0.2 1195"), 1)
		if err := f.store.ReplaceText(f.ctx, p.ID, text); err != nil {
			t.Fatal(err)
		}
		got, err := f.store.Get(f.ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if want := []Remote{{Network: "udp", Address: "127.0.0.2:1195"}}; !slices.Equal(got.Remotes, want) {
			t.Errorf("remotes = %+v, want %+v", got.Remotes, want)
		}
		if names := filesOf(t, p); !slices.Equal(names, []string{"ca.crt", "client.crt", "client.key", "work.ovpn"}) {
			t.Errorf("the files folder has %v", names)
		}
	})
	t.Run("a text that does not parse", func(t *testing.T) {
		f, p, old := setup(t)
		err := f.store.ReplaceText(f.ctx, p.ID, append(bytes.Clone(old), []byte("remote \"unclosed\n")...))
		if got := ProblemOf(err); got.Kind != ProblemSyntax {
			t.Errorf("ProblemOf = %+v (err %v)", got, err)
		}
		now, err := f.store.Text(f.ctx, p.ID)
		if err != nil || !bytes.Equal(now, old) {
			t.Errorf("the text changed: %v", err)
		}
		if names := filesOf(t, p); !slices.Equal(names, []string{"ca.crt", "client.crt", "client.key", "work.ovpn"}) {
			t.Errorf("the files folder has %v", names)
		}
	})
	t.Run("a text that names a file that is not stored", func(t *testing.T) {
		f, p, old := setup(t)
		err := f.store.ReplaceText(f.ctx, p.ID, bytes.Replace(old, []byte("ca ca.crt"), []byte("ca other.crt"), 1))
		if got := ProblemOf(err); got.Kind != ProblemFile {
			t.Errorf("ProblemOf = %+v (err %v)", got, err)
		}
		if now, _ := f.store.Text(f.ctx, p.ID); !bytes.Equal(now, old) {
			t.Error("the text changed")
		}
	})
	t.Run("a text that is too large", func(t *testing.T) {
		f, p, _ := setup(t)
		err := f.store.ReplaceText(f.ctx, p.ID, bytes.Repeat([]byte("#"), MaxFileBytes+1))
		errtest.Route(t, err, "Store.ReplaceText.ProfileTextTooLarge")
	})
	t.Run("a profile that is not there", func(t *testing.T) {
		f, _, old := setup(t)
		err := f.store.ReplaceText(f.ctx, "0123456789abcdef", old)
		errtest.Route(t, err, "Store.ReplaceText->Store.usable->Store.Get->Store.profileDir.ProfileNotFound")
	})
}

func TestLoadWithOverride(t *testing.T) {
	pki := profilestest.NewPKI(t)
	dir := t.TempDir()
	path := profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Separate: true})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The file on the disk is not a profile; the text given is.
	if err := os.WriteFile(path, []byte("not a profile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text := bytes.Replace(data, []byte("remote 127.0.0.1 1194"), []byte("remote 127.0.0.9 1194"), 1)
	p, files, err := loadWith(dir, "a.ovpn", text)
	if err != nil {
		t.Fatal(err)
	}
	if p.Remotes[0].Address != "127.0.0.9:1194" {
		t.Errorf("remotes = %+v: the file was read, not the text", p.Remotes)
	}
	if want := []string{"ca.crt", "client.crt", "client.key"}; !slices.Equal(files, want) {
		t.Errorf("files = %v, want %v: they are read from the folder", files, want)
	}
}
