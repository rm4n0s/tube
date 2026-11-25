// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

func TestThePasswordIsRemembered(t *testing.T) {
	f := newFixture(t)
	p := f.add("work", profilestest.Options{Password: true})
	if p.HasPassword {
		t.Fatal("a new profile has a remembered password")
	}
	if got, err := f.store.Password(f.ctx, p.ID); err != nil || got != "" {
		t.Fatalf("Password = %q, %v before one is set", got, err)
	}
	const secret = "hunter2 with spaces, and é"
	if err := f.store.SetPassword(f.ctx, p.ID, secret); err != nil {
		t.Fatal(err)
	}
	if got, err := f.store.Password(f.ctx, p.ID); err != nil || got != secret {
		t.Errorf("Password = %q, %v", got, err)
	}
	got, err := f.store.Get(f.ctx, p.ID)
	if err != nil || !got.HasPassword || !got.NeedsPassword {
		t.Errorf("the profile is %+v, %v", got, err)
	}
	// The file is one that only the user can read, next to meta.json and not among the files
	// of the profile.
	path := filepath.Join(f.store.dir, p.ID, passwordFile)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the password file: %v, %v", info, err)
	}
	if len(got.Files) != 0 {
		t.Errorf("the password is among the files of the profile: %q", got.Files)
	}
	// A new password replaces the old one; an empty one forgets it; forgetting twice is fine.
	if err := f.store.SetPassword(f.ctx, p.ID, "another"); err != nil {
		t.Fatal(err)
	}
	if got, err := f.store.Password(f.ctx, p.ID); err != nil || got != "another" {
		t.Errorf("Password = %q, %v", got, err)
	}
	for range 2 {
		if err := f.store.SetPassword(f.ctx, p.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the password file is still there: %v", err)
	}
	if got, err := f.store.Get(f.ctx, p.ID); err != nil || got.HasPassword {
		t.Errorf("after forgetting, the profile is %+v, %v", got, err)
	}
	// A deleted profile takes its password with it.
	if err := f.store.SetPassword(f.ctx, p.ID, secret); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Delete(f.ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the password outlived its profile: %v", err)
	}
}

func TestThePasswordFails(t *testing.T) {
	f := newFixture(t)
	p := f.add("work", profilestest.Options{Password: true})
	const secret = "a-secret-that-must-not-be-in-an-error"
	tests := []struct {
		name  string
		do    func() error
		route string
	}{
		{"too long", func() error { return f.store.SetPassword(f.ctx, p.ID, strings.Repeat("x", MaxPasswordLen+1)) }, "Store.SetPassword.ProfilePasswordTooLong"},
		{"an id that is none", func() error { return f.store.SetPassword(f.ctx, "../x", secret) }, "Store.SetPassword->Store.profileDir.ProfileIDInvalid"},
		{"no such profile", func() error { return f.store.SetPassword(f.ctx, "0123456789abcdef", secret) }, "Store.SetPassword->Store.profileDir.ProfileNotFound"},
		{"reading with an id that is none", func() error { _, err := f.store.Password(f.ctx, "../x"); return err }, "Store.Password->Store.profileDir.ProfileIDInvalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.do()
			errtest.Route(t, err, tt.route)
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the password is in the error: %v", err)
			}
		})
	}
	// The longest password that is remembered.
	longest := strings.Repeat("y", MaxPasswordLen)
	if err := f.store.SetPassword(f.ctx, p.ID, longest); err != nil {
		t.Fatal(err)
	}
	if got, err := f.store.Password(f.ctx, p.ID); err != nil || got != longest {
		t.Errorf("the longest password came back with %d bytes, %v", len(got), err)
	}
	// A folder where the file belongs: it is no password, it cannot be read as one, and it
	// cannot be forgotten while something is in it.
	path := filepath.Join(f.store.dir, p.ID, passwordFile)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := f.store.Get(f.ctx, p.ID); err != nil || got.HasPassword {
		t.Errorf("a folder is taken for a password: %+v, %v", got, err)
	}
	errtest.Route(t, f.store.SetPassword(f.ctx, p.ID, ""), "Store.SetPassword.ProfilePasswordForgetFailed")
	if _, err := f.store.Password(f.ctx, p.ID); err == nil {
		t.Error("a folder was read as a password")
	}
	// An empty file is no password either.
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := f.store.Get(f.ctx, p.ID); err != nil || got.HasPassword {
		t.Errorf("an empty file is taken for a password: %+v, %v", got, err)
	}
}
