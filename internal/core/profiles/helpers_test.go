// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
)

// testTime is the time of the fake clock of these tests.
var testTime = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

// fixture is a store in isolated folders, a fake clock, and a PKI for the test's profiles.
type fixture struct {
	t     *testing.T
	ctx   context.Context
	store *Store
	clock *clock.Fake
	pki   *profilestest.PKI
	// src is a folder outside the store where the test writes the profiles it adds.
	src string
}

// newFixture isolates the test's folders and makes a store in them.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dirs := storagetest.Isolate(t)
	clk := clock.NewFake(testTime)
	return &fixture{
		t:     t,
		ctx:   context.Background(),
		store: NewStore(dirs.Config, clk),
		clock: clk,
		pki:   profilestest.NewPKI(t),
		src:   t.TempDir(),
	}
}

// write writes a profile into its own new folder under src and returns its path.
func (f *fixture) write(name string, opts profilestest.Options) string {
	f.t.Helper()
	dir, err := os.MkdirTemp(f.src, "p")
	if err != nil {
		f.t.Fatal(err)
	}
	return profilestest.WriteProfile(f.t, dir, name, f.pki, opts)
}

// add writes and adds a profile, and fails the test when the add fails.
func (f *fixture) add(name string, opts profilestest.Options) Profile {
	f.t.Helper()
	p, err := f.store.Add(f.ctx, f.write(name+".ovpn", opts), name)
	if err != nil {
		f.t.Fatalf("Add(%s): %v", name, err)
	}
	return p
}

// folders returns the names of the folders of the store.
func (f *fixture) folders() []string {
	f.t.Helper()
	entries, err := os.ReadDir(f.store.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// replaceLine rewrites the file, replacing the line old with the lines new.
func replaceLine(t *testing.T, path, old string, new ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	found := false
	var out []string
	for _, l := range lines {
		if l == old {
			out = append(out, new...)
			found = true
			continue
		}
		out = append(out, l)
	}
	if !found {
		t.Fatalf("%s has no line %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

// notRoot skips a test that needs the mode bits to stop it.
func notRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root is not stopped by mode bits")
	}
}

// chmod changes a mode and changes it back to 0700 when the test ends.
func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Error(err)
		}
	})
}

// sameBytes fails the test unless the file has the content.
func sameBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s changed", filepath.Base(path))
	}
}

// tagOf returns the Tag of an *errors.Error.
func tagOf(err error) (string, bool) {
	appErr, ok := errors.FromError(err)
	if !ok {
		return "", false
	}
	return appErr.Tag, true
}
