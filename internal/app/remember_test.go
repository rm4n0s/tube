// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/storage"
)

func TestWhatAConnectRemembers(t *testing.T) {
	const secret = "hunter2-must-never-be-logged"
	tests := []struct {
		name         string
		had          string
		remember     Remember
		wantUser     string
		wantPassword string
		wantChanged  bool
	}{
		{"nothing, as a script", "", Remember{}, "", "", false},
		{"nothing, and what was remembered stays", "old", Remember{}, "", "old", false},
		{"the user name", "", Remember{User: true}, "alice", "", true},
		{"the user name, and the old password stays", "old", Remember{User: true}, "alice", "old", true},
		{"the password, and the user name with it", "", Remember{Password: true}, "alice", secret, true},
		{"both", "old", Remember{User: true, Password: true}, "alice", secret, true},
		{"forget the password", "old", Remember{Forget: true}, "", "", true},
		{"forget, with nothing remembered", "", Remember{Forget: true}, "", "", false},
		{"the user name, and forget the password", "old", Remember{User: true, Forget: true}, "alice", "", true},
		{"keeping wins over forgetting", "old", Remember{Password: true, Forget: true}, "alice", secret, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, dirs := open(t)
			ctx := context.Background()
			p := addProfile(t, a, "Work", profilestest.Options{Password: true})
			if tt.had != "" {
				if err := a.store.SetPassword(ctx, p.ID, tt.had); err != nil {
					t.Fatal(err)
				}
			}
			p, err := a.store.Get(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := a.remember(ctx, p, &Credentials{Username: "alice", Password: secret}, tt.remember); got != tt.wantChanged {
				t.Errorf("remember said %v, want %v", got, tt.wantChanged)
			}
			after, err := a.store.Get(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			password, err := a.store.Password(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Username != tt.wantUser || password != tt.wantPassword || after.HasPassword != (tt.wantPassword != "") {
				t.Errorf("remembered: user name %q, password %q, has one %v; want %q, %q", after.Username, password, after.HasPassword, tt.wantUser, tt.wantPassword)
			}
			if strings.Contains(logText(t, dirs), secret) {
				t.Error("the password is in the log")
			}
			// The snapshot says that there is one, and never which.
			snap, err := a.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := snap.Profiles[0]; got.HasPassword != (tt.wantPassword != "") || got.Username != tt.wantUser {
				t.Errorf("the snapshot's profile is %+v", got)
			}
		})
	}
}

func TestRememberString(t *testing.T) {
	tests := []struct {
		r    Remember
		want string
	}{
		{Remember{}, "keep nothing"},
		{Remember{User: true}, "keep the user name"},
		{Remember{Password: true}, "keep the password"},
		{Remember{User: true, Password: true, Forget: true}, "keep the password"},
		{Remember{Forget: true}, "forget the password"},
		{Remember{User: true, Forget: true}, "keep the user name, forget the password"},
	}
	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("%+v is %q, want %q", tt.r, got, tt.want)
		}
	}
}

func TestForgetPassword(t *testing.T) {
	a, _ := open(t)
	ctx := context.Background()
	p := addProfile(t, a, "Work", profilestest.Options{Password: true})
	if err := a.store.SetPassword(ctx, p.ID, "hunter2"); err != nil {
		t.Fatal(err)
	}
	found, err := a.FindProfile(ctx, "Work")
	if err != nil || !found.HasPassword || !found.NeedsPassword {
		t.Fatalf("FindProfile = %+v, %v", found, err)
	}
	changes := 0
	cancel := a.Subscribe(func() { changes++ })
	defer cancel()
	if err := a.ForgetPassword(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if found, err := a.FindProfile(ctx, "Work"); err != nil || found.HasPassword {
		t.Errorf("after forgetting, FindProfile = %+v, %v", found, err)
	}
	if changes != 1 {
		t.Errorf("the subscribers were told %d times", changes)
	}
	errtest.Route(t, a.ForgetPassword(ctx, "nothing"), "App.ForgetPassword->Store.SetPassword->Store.profileDir.ProfileIDInvalid")
}

func TestOwnProfileProgram(t *testing.T) {
	tests := []struct {
		argv0, want string
		ok          bool
	}{
		{"firefox", "firefox", true},
		{"/usr/bin/firefox", "firefox", true},
		{"/opt/Firefox", "firefox", true},
		{"firefox-esr", "firefox", true},
		{"firefox-bin", "firefox", true},
		{"thunderbird", "thunderbird", true},
		{"/usr/bin/thunderbird-beta", "thunderbird", true},
		{"firefoxy", "", false},
		{"my-firefox", "", false},
		{"chromium", "", false},
		{"ping", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		if got, ok := ownProfileProgram(tt.argv0); got != tt.want || ok != tt.ok {
			t.Errorf("ownProfileProgram(%q) = %q, %v; want %q, %v", tt.argv0, got, ok, tt.want, tt.ok)
		}
	}
}

func TestChoosesProfile(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"https://example.com"}, false},
		{[]string{"--private-window", "https://example.com/-P"}, false},
		{[]string{"--new-window", "-url", "about:blank"}, false},
		{[]string{"-P", "work"}, true},
		{[]string{"-p", "work"}, true},
		{[]string{"--P", "work"}, true},
		{[]string{"--profile", "/tmp/x"}, true},
		{[]string{"-profile", "/tmp/x"}, true},
		{[]string{"--profile=/tmp/x"}, true},
		{[]string{"--Profile", "/tmp/x"}, true},
		{[]string{"-ProfileManager"}, true},
		{[]string{"--no-remote", "https://example.com", "-P", "work"}, true},
		{[]string{"--profiler"}, false},
		{[]string{"P"}, false},
	}
	for _, tt := range tests {
		if got := choosesProfile(tt.args); got != tt.want {
			t.Errorf("choosesProfile(%q) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestApplicationArgv(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		argv []string
		// want is the argv, with "<dir>" for the folder of Tube's own profile; program is the
		// name of that folder's program, or "" when the argv is not changed.
		want    []string
		program string
	}{
		{"firefox gets a profile of its own", []string{"firefox"}, []string{"firefox", "--no-remote", "--profile", "<dir>"}, "firefox"},
		{"its arguments stay, after the profile", []string{"/usr/bin/firefox", "--private-window", "https://example.com"}, []string{"/usr/bin/firefox", "--no-remote", "--profile", "<dir>", "--private-window", "https://example.com"}, "firefox"},
		{"thunderbird too, in a folder of its own", []string{"thunderbird"}, []string{"thunderbird", "--no-remote", "--profile", "<dir>"}, "thunderbird"},
		{"a profile that the user chose is the user's", []string{"firefox", "-P", "work"}, []string{"firefox", "-P", "work"}, ""},
		{"another program is not touched", []string{"ping", "-c", "1", "example.com"}, []string{"ping", "-c", "1", "example.com"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, dirs := open(t, tt.argv...)
			argv, note, err := a.applicationArgv(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(dirs.State, tt.program+"-profile")
			want := slices.Clone(tt.want)
			for i, arg := range want {
				if arg == "<dir>" {
					want[i] = dir
				}
			}
			if !slices.Equal(argv, want) {
				t.Errorf("the argv is %q, want %q", argv, want)
			}
			if (note != "") != (tt.program != "") || (note != "" && !strings.HasPrefix(note, tt.program+" runs on a profile of its own")) {
				t.Errorf("the note is %q", note)
			}
			if tt.program == "" {
				return
			}
			// The folder is made, and only the user can look into it: it will hold cookies.
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() || info.Mode().Perm() != storage.DirMode {
				t.Errorf("the folder of the profile: %v, %v", info, err)
			}
			// The user's argv is not changed: the TUI shows it.
			if !slices.Equal(a.Target(), tt.argv) {
				t.Errorf("the target became %q", a.Target())
			}
		})
	}
	t.Run("a folder that cannot be made stops the start", func(t *testing.T) {
		// A program of the test with the name of Firefox: the real one is never started.
		fake := filepath.Join(t.TempDir(), "firefox")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		a, dirs := open(t, fake)
		// A file where the folder belongs: no user can make the folder there.
		if err := os.WriteFile(filepath.Join(dirs.State, "firefox-profile"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		argv, _, err := a.applicationArgv(ctx)
		errtest.Route(t, err, "App.applicationArgv->EnsureDir.StorageNotADir")
		if argv != nil {
			t.Errorf("an argv came with the error: %q", argv)
		}
		// And the application is not started on the user's own profile: the start fails, and
		// the tunnel is closed.
		tun := pingTunnel(fakeNetwork)
		a.setTunnel(tun, a.profile)
		errtest.Route(t, a.Start(ctx), "App.takeTunnel->App.applicationArgv->EnsureDir.StorageNotADir")
		if !tun.Closed() {
			t.Error("the tunnel was not closed")
		}
	})
}
