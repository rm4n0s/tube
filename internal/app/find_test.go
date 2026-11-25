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
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

// moved is a profile file of the tests under the name in the folder.
func moved(t *testing.T, dir, name string, opts profilestest.Options) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.Rename(profileFile(t, "p.ovpn", opts), path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAddProfileWithDNS(t *testing.T) {
	a, _ := open(t)
	n := counter(t, a)
	ctx := context.Background()
	path := profileFile(t, "lab.ovpn", profilestest.Options{})
	tests := []struct {
		name, dns string
		want      []string
		route     string
	}{
		{"none", "", nil, ""},
		{"two", "9.9.9.9, 2620:fe::fe", []string{"9.9.9.9", "2620:fe::fe"}, ""},
		{"not an address", "9.9.9.9 dns.example", nil, "App.AddProfile->ParseDNS.ProfileDNSInvalid"},
		{"too many", "1.1.1.1 1.1.1.2 1.1.1.3 1.1.1.4 1.1.1.5", nil, "App.AddProfile->ParseDNS.ProfileDNSTooMany"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, err := a.store.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			n.Store(0)
			id, err := a.AddProfile(ctx, path, tt.name, tt.dns, false)
			if tt.route != "" {
				errtest.Route(t, err, tt.route)
				// A mistake in the DNS servers adds nothing and tells nobody.
				after, lerr := a.store.List(ctx)
				if lerr != nil || len(after) != len(before) || n.Load() != 0 {
					t.Errorf("a profile was added: %d profiles before, %d after (%v), %d calls", len(before), len(after), lerr, n.Load())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p, err := a.store.Get(ctx, id)
			if err != nil || !slices.Equal(addrStrings(p.DNS), tt.want) {
				t.Errorf("the DNS servers are %v, %v, want %v", p.DNS, err, tt.want)
			}
			if n.Load() != 1 {
				t.Errorf("the subscriber was called %d times", n.Load())
			}
		})
	}
}

func TestFindOrAddProfile(t *testing.T) {
	a, _ := open(t)
	ctx := context.Background()
	work := addProfile(t, a, "Work", profilestest.Options{Password: true})
	if err := a.store.SetUsername(ctx, work.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDNS(ctx, work.ID, "9.9.9.9"); err != nil {
		t.Fatal(err)
	}
	dotted := addProfile(t, a, "de.example.udp", profilestest.Options{})
	dir := t.TempDir()
	lab := moved(t, dir, "Lab.ovpn", profilestest.Options{})
	moved(t, dir, "Work.ovpn", profilestest.Options{})
	upper := moved(t, dir, "Loud.OVPN", profilestest.Options{})
	noExt := moved(t, dir, "plain", profilestest.Options{})
	needs := moved(t, dir, "Login.ovpn", profilestest.Options{Password: true})
	moved(t, dir, "Bare.ovpn", profilestest.Options{})
	home, _ := os.LookupEnv("HOME")
	moved(t, home, "Den.ovpn", profilestest.Options{})
	if err := os.Mkdir(filepath.Join(dir, "folder.ovpn"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	tests := []struct {
		name, value string
		// wantID is the stored profile that is found; wantName is the name of the one that is
		// added.
		wantID, wantName string
		route            string
	}{
		{"a name", "Work", work.ID, "", ""},
		{"a name, the case ignored", "work", work.ID, "", ""},
		{"a name written with .ovpn", "Work.ovpn", work.ID, "", ""},
		{"a name with dots is not cut", "de.example.udp", dotted.ID, "", ""},
		{"a file whose profile is stored already", filepath.Join(dir, "Work.ovpn"), work.ID, "", ""},
		{"a file", lab, "", "Lab", ""},
		{"the same file again is the stored profile", lab, "", "", ""},
		{"a file without .ovpn written", filepath.Join(dir, "Loud"), "", "", "App.FindOrAddProfile.AppNoProfileOrFile"},
		{"a file with .OVPN", upper, "", "Loud", ""},
		{"a file that has no .ovpn", noExt, "", "plain", ""},
		{"a file in this folder", "Login.ovpn", "", "Login", ""},
		{"a file in this folder, only its name", "Bare", "", "Bare", ""},
		{"only the name again is the stored profile", "Bare", "", "", ""},
		{"a file of the home folder, without .ovpn", "~/Den", "", "Den", ""},
		{"a folder", filepath.Join(dir, "folder.ovpn"), "", "", "App.FindOrAddProfile.AppNoProfileOrFile"},
		{"nothing", "nothing", "", "", "App.FindOrAddProfile.AppNoProfileOrFile"},
		{"a file that is not there", filepath.Join(dir, "missing.ovpn"), "", "", "App.FindOrAddProfile.AppNoProfileOrFile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := a.FindOrAddProfile(ctx, tt.value, false)
			if tt.route != "" {
				meta := errtest.Route(t, err, tt.route)
				if meta["name"] != tt.value {
					t.Errorf("metadata = %v", meta)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.wantID != "":
				if found.ID != tt.wantID || found.Added {
					t.Errorf("found %+v, want the stored %s", found, tt.wantID)
				}
			case tt.wantName != "":
				if found.Name != tt.wantName || !found.Added {
					t.Errorf("found %+v, want %s added", found, tt.wantName)
				}
				if p, err := a.store.Get(ctx, found.ID); err != nil || p.Name != tt.wantName {
					t.Errorf("the stored profile is %+v, %v", p, err)
				}
			default:
				if found.Added {
					t.Errorf("the file was added a second time: %+v", found)
				}
			}
		})
	}
	t.Run("a long path is told whole, and made safe", func(t *testing.T) {
		long := filepath.Join(dir, strings.Repeat("a", 120), "missing\x1b[31m")
		_, err := a.FindOrAddProfile(ctx, long, false)
		meta := errtest.Route(t, err, "App.FindOrAddProfile.AppNoProfileOrFile")
		name, _ := meta["name"].(string)
		if !strings.Contains(name, "missing") || strings.ContainsRune(name, 0x1b) {
			t.Errorf("the path in the error is %q", name)
		}
		if strings.Contains(Explain(err), "\x1b") {
			t.Errorf("the explanation has the escape: %q", Explain(err))
		}
	})
	t.Run("what is found says what the profile has", func(t *testing.T) {
		found, err := a.FindOrAddProfile(ctx, "Work", false)
		if err != nil || !found.NeedsPassword || found.HasPassword || found.Username != "alice" || !found.HasDNS {
			t.Errorf("Work is %+v, %v", found, err)
		}
		found, err = a.FindOrAddProfile(ctx, needs, false)
		if err != nil || !found.NeedsPassword || found.HasPassword || found.Username != "" || found.HasDNS || found.Added {
			t.Errorf("Login is %+v, %v", found, err)
		}
	})
	t.Run("a stored profile that cannot be used is not replaced by its file", func(t *testing.T) {
		broken := addProfile(t, a, "Broken", profilestest.Options{})
		if err := os.WriteFile(filepath.Join(broken.FilesDir, broken.File), []byte("remote \"x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := a.FindOrAddProfile(ctx, moved(t, dir, "Broken.ovpn", profilestest.Options{}), false)
		errtest.Route(t, err, "App.FindOrAddProfile->App.FindProfile.AppProfileBroken")
	})
	t.Run("a file that runs scripts", func(t *testing.T) {
		path := moved(t, dir, "Scripts.ovpn", profilestest.Options{Extra: []string{"script-security 2", "up /bin/up.sh"}})
		_, err := a.FindOrAddProfile(ctx, path, false)
		if options, ok := IsRunsScripts(err); !ok || options != "up" {
			t.Fatalf("IsRunsScripts = %q, %v (err %v)", options, ok, err)
		}
		if _, err := a.FindProfile(ctx, "Scripts"); !isNoProfile(err) {
			t.Errorf("the profile was added: %v", err)
		}
		found, err := a.FindOrAddProfile(ctx, path, true)
		if err != nil || !found.Added || found.Name != "Scripts" {
			t.Errorf("without the scripts: %+v, %v", found, err)
		}
	})
	t.Run("a file that is not a profile", func(t *testing.T) {
		path := filepath.Join(dir, "notes.ovpn")
		if err := os.WriteFile(path, []byte("remote \"x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := a.FindOrAddProfile(ctx, path, false)
		errtest.Route(t, err, "App.FindOrAddProfile->App.AddProfile->Store.Add")
	})
}

func TestUseDNS(t *testing.T) {
	own := profiles.Profile{ID: "0123456789abcdef", Name: "work"}
	tests := []struct {
		name string
		// own are the DNS servers of the profile; use is what UseDNS gets, when set.
		own   string
		use   string
		set   bool
		want  []string
		route string
	}{
		{"nothing is asked: the ones of the VPN", "", "", false, []string{"10.9.0.1"}, ""},
		{"nothing is asked: the ones of the profile", "1.1.1.1", "", false, []string{"1.1.1.1"}, ""},
		{"for this run", "", "9.9.9.9, 149.112.112.112", true, []string{"9.9.9.9", "149.112.112.112"}, ""},
		{"in place of the profile's", "1.1.1.1", "9.9.9.9", true, []string{"9.9.9.9"}, ""},
		{"empty: the ones of the VPN, not the profile's", "1.1.1.1", "", true, []string{"10.9.0.1"}, ""},
		{"a mistake", "1.1.1.1", "dns.example", true, nil, "App.UseDNS->ParseDNS.ProfileDNSInvalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openRun(t, testBinary(t))
			p := own
			var err error
			if p.DNS, err = profiles.ParseDNS(tt.own); err != nil {
				t.Fatal(err)
			}
			s.a.setTunnel(s.tun, p)
			if tt.set {
				err := s.a.UseDNS(tt.use)
				if tt.route != "" {
					errtest.Route(t, err, tt.route)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			newPinger(t)
			if err := s.a.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := s.a.RunState().DNS; !slices.Equal(got, tt.want) {
				t.Errorf("the application's DNS servers are %v, want %v", got, tt.want)
			}
		})
	}
}
