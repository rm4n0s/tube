// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/rm4n0s/gopenvpn-conn/ovpn"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

func TestLoad(t *testing.T) {
	pki := profilestest.NewPKI(t)

	t.Run("inline files", func(t *testing.T) {
		dir := t.TempDir()
		profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{})
		p, files, err := Load(dir, "a.ovpn")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 0 {
			t.Errorf("files = %v, want none", files)
		}
		if len(p.Remotes) != 1 || p.Remotes[0].Address != "127.0.0.1:1194" || p.Remotes[0].Network != "udp" {
			t.Errorf("remotes = %+v", p.Remotes)
		}
	})
	t.Run("files next to it", func(t *testing.T) {
		dir := t.TempDir()
		profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Separate: true})
		_, files, err := Load(dir, "a.ovpn")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"ca.crt", "client.crt", "client.key"}; !slices.Equal(files, want) {
			t.Errorf("files = %v, want %v", files, want)
		}
	})
	t.Run("a file in a folder of its own", func(t *testing.T) {
		dir := t.TempDir()
		path := profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Separate: true})
		if err := os.Mkdir(filepath.Join(dir, "keys"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(dir, "ca.crt"), filepath.Join(dir, "keys", "ca.crt")); err != nil {
			t.Fatal(err)
		}
		replaceLine(t, path, "ca ca.crt", "ca keys/ca.crt")
		_, files, err := Load(dir, "a.ovpn")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"client.crt", "client.key", "keys/ca.crt"}; !slices.Equal(files, want) {
			t.Errorf("files = %v, want %v", files, want)
		}
	})
	t.Run("a name that climbs out of the folder", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "profile")
		path := profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Separate: true})
		// A valid CA outside: if it were read, the profile would parse.
		if err := os.WriteFile(filepath.Join(parent, "ca.crt"), pki.CA, 0o600); err != nil {
			t.Fatal(err)
		}
		replaceLine(t, path, "ca ca.crt", "ca ../ca.crt")
		_, _, err := Load(dir, "a.ovpn")
		if got := ProblemOf(err); got.Kind != ProblemFile || got.Option != "ca" {
			t.Errorf("ProblemOf = %+v (err %v), want the ca refused", got, err)
		}
	})
	t.Run("a link to a file outside the folder", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "profile")
		path := profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Separate: true})
		outside := filepath.Join(parent, "outside.crt")
		if err := os.WriteFile(outside, pki.CA, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "link.crt")); err != nil {
			t.Fatal(err)
		}
		replaceLine(t, path, "ca ca.crt", "ca link.crt")
		_, _, err := Load(dir, "a.ovpn")
		if got := ProblemOf(err); got.Kind != ProblemFile || got.Option != "ca" {
			t.Errorf("ProblemOf = %+v (err %v), want the ca refused", got, err)
		}
	})
	t.Run("a file that is too large", func(t *testing.T) {
		dir := t.TempDir()
		path := profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Separate: true})
		big := append(bytes.Clone(pki.CA), bytes.Repeat([]byte("#"), MaxFileBytes+1-len(pki.CA))...)
		if err := os.WriteFile(filepath.Join(dir, "big.crt"), big, 0o600); err != nil {
			t.Fatal(err)
		}
		replaceLine(t, path, "ca ca.crt", "ca big.crt")
		_, _, err := Load(dir, "a.ovpn")
		meta := errtest.Route(t, err, "Load->loadWith.ProfileFileTooLarge")
		if meta["file"] != "big.crt" {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("a file at the limit", func(t *testing.T) {
		dir := t.TempDir()
		path := profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Separate: true})
		// A CA file is PEM; text after the block is ignored by the parser of certificates.
		full := append(bytes.Clone(pki.CA), bytes.Repeat([]byte("#"), MaxFileBytes-len(pki.CA))...)
		if err := os.WriteFile(filepath.Join(dir, "full.crt"), full, 0o600); err != nil {
			t.Fatal(err)
		}
		replaceLine(t, path, "ca ca.crt", "ca full.crt")
		if _, _, err := Load(dir, "a.ovpn"); err != nil {
			t.Errorf("a file of exactly %d bytes: %v", MaxFileBytes, err)
		}
	})
	t.Run("too many files", func(t *testing.T) {
		dir := t.TempDir()
		// The ca option may be given again: each one adds certificates, and names a file.
		var extra []string
		for i := range MaxFiles + 1 {
			name := "ca" + strconv.Itoa(i) + ".crt"
			if err := os.WriteFile(filepath.Join(dir, name), pki.CA, 0o600); err != nil {
				t.Fatal(err)
			}
			extra = append(extra, "ca "+name)
		}
		profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Extra: extra})
		_, _, err := Load(dir, "a.ovpn")
		errtest.Route(t, err, "Load->loadWith.ProfileTooManyFiles")
	})
	t.Run("exactly the most files", func(t *testing.T) {
		dir := t.TempDir()
		var extra []string
		for i := range MaxFiles {
			name := "ca" + strconv.Itoa(i) + ".crt"
			if err := os.WriteFile(filepath.Join(dir, name), pki.CA, 0o600); err != nil {
				t.Fatal(err)
			}
			extra = append(extra, "ca "+name)
		}
		profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Extra: extra})
		_, files, err := Load(dir, "a.ovpn")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != MaxFiles {
			t.Errorf("%d files, want %d", len(files), MaxFiles)
		}
	})
	t.Run("a folder that does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		_, _, err := Load(missing, "a.ovpn")
		meta := errtest.Route(t, err, "Load->loadWith.ProfileFolderOpenFailed")
		if meta["dir"] != missing {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("a profile that does not exist", func(t *testing.T) {
		_, _, err := Load(t.TempDir(), "a.ovpn")
		if tag, ok := tagOf(err); !ok || tag != ovpn.TagReadFailed {
			t.Errorf("err = %v, want the parser's %s", err, ovpn.TagReadFailed)
		}
	})
}
