// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
)

// step is a prompt of the terminal and what the user answers to it.
type step = [2]string

// The prompts, as the tests wait for them.
const (
	passwordPrompt = "Password: "
	saveAll        = "Save the user name, the password and the DNS servers with the profile? [Y/n] "
	saveLogin      = "Save the user name and the password with the profile? [Y/n] "
	saveDNS        = "Save the DNS servers with the profile? [Y/n] "
)

// checkDNS is the check of the tests: an answer with a letter in it is refused, as the app
// refuses it.
func checkDNS(text string) error {
	if strings.ContainsAny(text, "abcdefghijklmnopqrstuvwxyz") {
		return &errors.Error{Tag: "ProfileDNSInvalid", Metadata: map[string]any{"value": text}}
	}
	return nil
}

func TestSettle(t *testing.T) {
	login := app.Found{ID: "id", Name: "work", NeedsPassword: true}
	remembered := app.Found{ID: "id", Name: "work", NeedsPassword: true, HasPassword: true, Username: "ada"}
	open := app.Found{ID: "id", Name: "home"}
	creds := func(user, password string) *app.Credentials {
		return &app.Credentials{Username: user, Password: password}
	}
	auth := authFile(t, "filed\nfrom-file\n", 0o600)
	tests := []struct {
		name  string
		f     flags
		found app.Found
		// steps are what the terminal asks and what the user answers; nothing is asked when
		// there are none.
		steps []step
		want  plan
	}{
		// Nothing is missing: nothing is asked, and nothing is saved without --save.
		{"no login, DNS of its own", flags{}, app.Found{HasDNS: true}, nil, plan{}},
		{"no login, no DNS: the ones of the VPN, without a question", flags{}, open, nil, plan{}},
		{"the remembered login", flags{}, remembered, nil, plan{}},
		{"the remembered login, its user name given", flags{username: "ada"}, remembered, nil, plan{}},
		{"--auth-file", flags{authFile: auth}, login, nil, plan{creds: creds("filed", "from-file")}},
		{"--auth-file wins over the remembered login", flags{authFile: auth}, remembered, nil, plan{creds: creds("filed", "from-file")}},
		{"--username and --password", flags{username: "bob", password: "pw"}, login, nil, plan{creds: creds("bob", "pw")}},
		{"they win over the remembered login", flags{username: "bob", password: "pw"}, remembered, nil, plan{creds: creds("bob", "pw")}},
		{"--password with the remembered user name", flags{password: "pw"}, remembered, nil, plan{creds: creds("ada", "pw")}},
		{"--save", flags{username: "bob", password: "pw", save: true}, login, nil, plan{creds: creds("bob", "pw"), save: true}},
		{"--dns", flags{dns: "9.9.9.9", dnsGiven: true}, open, nil, plan{dns: "9.9.9.9", dnsSet: true}},
		{"an empty --dns", flags{dnsGiven: true, save: true}, app.Found{HasDNS: true}, nil, plan{dnsSet: true, save: true}},
		{"a login that the profile does not ask for is not used", flags{username: "bob", password: "pw"}, open, nil, plan{}},
		{"a new profile, all given", flags{username: "bob", password: "pw", dns: "", dnsGiven: true},
			app.Found{NeedsPassword: true, Added: true}, nil, plan{creds: creds("bob", "pw"), dnsSet: true}},

		// What is missing is asked, one by one, and then whether to save it.
		{"everything", flags{}, login,
			[]step{{userPrompt, "bob\n"}, {passwordPrompt, "pw\n"}, {dnsPrompt, "9.9.9.9, 1.1.1.1\n"}, {saveAll, "\n"}},
			plan{creds: creds("bob", "pw"), dns: "9.9.9.9, 1.1.1.1", dnsSet: true, save: true}},
		{"not saved", flags{}, login,
			[]step{{userPrompt, "bob\n"}, {passwordPrompt, "pw\n"}, {dnsPrompt, "9.9.9.9\n"}, {saveAll, "n\n"}},
			plan{creds: creds("bob", "pw"), dns: "9.9.9.9", dnsSet: true}},
		{"no DNS: the ones of the VPN", flags{}, login,
			[]step{{userPrompt, "bob\n"}, {passwordPrompt, "pw\n"}, {dnsPrompt, "\n"}, {saveLogin, "yes\n"}},
			plan{creds: creds("bob", "pw"), save: true}},
		{"the user name is remembered", flags{}, app.Found{NeedsPassword: true, Username: "ada", HasDNS: true},
			[]step{{passwordPrompt, "pw\n"}, {saveLogin, "N\n"}}, plan{creds: creds("ada", "pw")}},
		{"--username: the password is asked", flags{username: "bob"}, app.Found{NeedsPassword: true, HasDNS: true},
			[]step{{passwordPrompt, "pw\n"}, {saveLogin, "y\n"}}, plan{creds: creds("bob", "pw"), save: true}},
		{"another user name than the remembered one: its password is asked", flags{username: "bob"}, remembered,
			[]step{{passwordPrompt, "pw\n"}, {dnsPrompt, "\n"}, {saveLogin, "n\n"}}, plan{creds: creds("bob", "pw")}},
		{"--password: the user name is asked", flags{password: "pw"}, app.Found{NeedsPassword: true, HasDNS: true},
			[]step{{userPrompt, "bob\n"}, {saveLogin, "\n"}}, plan{creds: creds("bob", "pw"), save: true}},
		{"--dns: only the login is asked", flags{dns: "9.9.9.9", dnsGiven: true}, login,
			[]step{{userPrompt, "bob\n"}, {passwordPrompt, "pw\n"}, {saveAll, "\n"}},
			plan{creds: creds("bob", "pw"), dns: "9.9.9.9", dnsSet: true, save: true}},
		{"--save: no question about it", flags{save: true}, login,
			[]step{{userPrompt, "bob\n"}, {passwordPrompt, "pw\n"}, {dnsPrompt, "9.9.9.9\n"}},
			plan{creds: creds("bob", "pw"), dns: "9.9.9.9", dnsSet: true, save: true}},
		{"a new profile without a login", flags{}, app.Found{Added: true},
			[]step{{dnsPrompt, "9.9.9.9\n"}, {saveDNS, "\n"}}, plan{dns: "9.9.9.9", dnsSet: true, save: true}},
		{"a new profile without a login, no DNS: nothing to save", flags{}, app.Found{Added: true},
			[]step{{dnsPrompt, "  \n"}}, plan{}},
		{"a new profile, the login given", flags{authFile: auth}, app.Found{NeedsPassword: true, Added: true},
			[]step{{dnsPrompt, "\n"}, {saveLogin, "n\n"}}, plan{creds: creds("filed", "from-file")}},
		{"a wrong DNS server is asked for again", flags{}, app.Found{Added: true},
			[]step{{dnsPrompt, "dns.example\n"}, {`"dns.example" is not an IP address. (ProfileDNSInvalid)` + "\r\n" + dnsPrompt, "9.9.9.9\n"}, {saveDNS, "n\n"}},
			plan{dns: "9.9.9.9", dnsSet: true}},
		{"an answer that is not yes or no is asked for again", flags{}, app.Found{Added: true},
			[]step{{dnsPrompt, "9.9.9.9\n"}, {saveDNS, "maybe\n"}, {yesOrNo[:len(yesOrNo)-1] + "\r\n" + saveDNS, "n\n"}},
			plan{dns: "9.9.9.9", dnsSet: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			master, slave := terminal(t)
			shown := user(t, master, tt.steps...)
			c := cli{tty: slave.Name()}
			got, err := c.settle(tt.f, tt.found, checkDNS)
			if err != nil {
				t.Fatal(err)
			}
			if (got.creds == nil) != (tt.want.creds == nil) || (got.creds != nil && *got.creds != *tt.want.creds) {
				t.Errorf("the credentials are not the wanted ones (given: %v, wanted: %v)", got.creds != nil, tt.want.creds != nil)
			}
			if got.dns != tt.want.dns || got.dnsSet != tt.want.dnsSet || got.save != tt.want.save {
				t.Errorf("DNS %q (set %v), save %v; want %q (set %v), save %v", got.dns, got.dnsSet, got.save, tt.want.dns, tt.want.dnsSet, tt.want.save)
			}
			screen := <-shown
			if len(tt.steps) == 0 && screen != "" {
				t.Errorf("the terminal was asked: %q", screen)
			}
			for _, s := range tt.steps {
				if !strings.Contains(screen, s[0]) {
					t.Errorf("the terminal did not show %q:\n%q", s[0], screen)
				}
			}
			// A password is never shown, whoever gave it.
			if strings.Contains(screen, "pw") || strings.Contains(screen, "from-file") {
				t.Errorf("the terminal showed the password: %q", screen)
			}
		})
	}
}

func TestSettleWithoutATerminal(t *testing.T) {
	c := cli{tty: filepath.Join(t.TempDir(), "no-tty")}
	login := app.Found{NeedsPassword: true}
	for _, f := range []flags{{}, {username: "bob"}, {password: "pw"}, {dns: "9.9.9.9", dnsGiven: true, save: true}} {
		_, err := c.settle(f, login, checkDNS)
		errtest.Route(t, err, "cli.settle->openTerminal.TubeNoTerminal")
		if got := app.Explain(err); !strings.Contains(got, "--auth-file, or --username and --password") || strings.Contains(got, "pw") {
			t.Errorf("the error is %q", got)
		}
	}
	// The DNS servers alone are not worth an error: nobody is there to ask, and the ones of
	// the VPN are used.
	got, err := c.settle(flags{}, app.Found{Added: true}, checkDNS)
	if err != nil || got != (plan{}) {
		t.Errorf("a new profile without a terminal: %+v, %v", got, err)
	}
	got, err = c.settle(flags{username: "bob", password: "pw", save: true}, app.Found{NeedsPassword: true, Added: true}, checkDNS)
	if err != nil || got.creds == nil || got.dnsSet || !got.save {
		t.Errorf("a new profile with its login: DNS set %v, save %v, %v", got.dnsSet, got.save, err)
	}
}

func TestSettleFailures(t *testing.T) {
	t.Run("an auth file that cannot be read", func(t *testing.T) {
		c := cli{tty: filepath.Join(t.TempDir(), "no-tty")}
		_, err := c.settle(flags{authFile: filepath.Join(t.TempDir(), "missing")}, app.Found{NeedsPassword: true}, checkDNS)
		errtest.Route(t, err, "cli.settle->readAuthFile.TubeAuthFileUnreadable")
	})
	// The user leaves at each of the questions.
	tests := []struct {
		name  string
		f     flags
		found app.Found
		steps []step
	}{
		{"at the user name", flags{password: "pw"}, app.Found{NeedsPassword: true}, []step{{userPrompt, ""}}},
		{"at the password", flags{}, app.Found{NeedsPassword: true, Username: "ada"}, []step{{passwordPrompt, ""}}},
		{"at the DNS servers", flags{}, app.Found{Added: true}, []step{{dnsPrompt, ""}}},
		{"at the answer about a wrong DNS server", flags{}, app.Found{Added: true}, []step{{dnsPrompt, "x\n"}, {"is not an IP address", ""}}},
		{"at the save", flags{}, app.Found{Added: true}, []step{{dnsPrompt, "9.9.9.9\n"}, {saveDNS, ""}}},
		{"at the answer that is not yes or no", flags{}, app.Found{Added: true}, []step{{dnsPrompt, "9.9.9.9\n"}, {saveDNS, "what\n"}, {"Answer y or n.", ""}}},
	}
	for _, tt := range tests {
		t.Run("the terminal ends "+tt.name, func(t *testing.T) {
			master, slave := terminal(t)
			shown := user(t, master, tt.steps...)
			c := cli{tty: slave.Name()}
			_, err := c.settle(tt.f, tt.found, checkDNS)
			errtest.Route(t, err, "cli.settle")
			if appErr, ok := errors.FromError(err); !ok || !strings.HasPrefix(appErr.Tag, "TubeTerminal") {
				t.Errorf("the error is %v", err)
			}
			<-shown
		})
	}
}

func TestSaveQuestion(t *testing.T) {
	creds := &app.Credentials{}
	for _, tt := range []struct {
		pl   plan
		want string
	}{
		{plan{creds: creds, dnsSet: true}, saveAll},
		{plan{creds: creds}, saveLogin},
		{plan{dnsSet: true}, saveDNS},
	} {
		if got := saveQuestion(tt.pl) + yesNo; got != tt.want {
			t.Errorf("saveQuestion = %q, want %q", got, tt.want)
		}
	}
}

// scriptsQuestion is the question about a profile file that runs scripts.
const scriptsQuestion = "This profile runs scripts (up). Tube never runs them, and sets the DNS of the application itself. Add it without them? [y/N] "

func TestFindProfile(t *testing.T) {
	tests := []struct {
		name string
		// steps are what the user answers on the terminal; without any there is no terminal.
		steps []step
		// added says that the profile is added; else route is the route of the error.
		added bool
		route string
		said  string
	}{
		{"yes", []step{{scriptsQuestion, "y\n"}}, true, "", ""},
		{"no", []step{{scriptsQuestion, "n\n"}}, false, "ProfileRunsScripts", ""},
		{"an empty answer is no", []step{{scriptsQuestion, "\n"}}, false, "ProfileRunsScripts", ""},
		{"another answer is asked for again", []step{{scriptsQuestion, "hm\n"}, {"Answer y or n.", "yes\n"}}, true, "", ""},
		{"the terminal ends", []step{{scriptsQuestion, ""}}, false, "cli.findProfile->askYesNo->askLine->readLine.TubeTerminalClosed", ""},
		{"no terminal", nil, false, "ProfileRunsScripts",
			"tube: Run this on a terminal, where Tube asks whether to add the profile without its scripts, or add it in the TUI.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storagetest.Isolate(t)
			ctx := context.Background()
			a, err := app.Open(ctx, app.Options{Env: os.LookupEnv})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := a.Close(); err != nil {
					t.Error(err)
				}
			}()
			path := profilestest.WriteProfile(t, t.TempDir(), "Scripts.ovpn", profilestest.NewPKI(t), profilestest.Options{Extra: []string{"script-security 2", "up /bin/up.sh"}})
			var said bytes.Buffer
			c := cli{stderr: &said, tty: filepath.Join(t.TempDir(), "no-tty")}
			var shown <-chan string
			if len(tt.steps) > 0 {
				master, slave := terminal(t)
				shown = user(t, master, tt.steps...)
				c.tty = slave.Name()
			}
			found, err := c.findProfile(ctx, a, path)
			if shown != nil {
				<-shown
			}
			if said.String() != tt.said {
				t.Errorf("Tube said %q, want %q", said.String(), tt.said)
			}
			snap, serr := a.Snapshot(ctx)
			if serr != nil {
				t.Fatal(serr)
			}
			if tt.added {
				if err != nil || !found.Added || found.Name != "Scripts" || len(snap.Profiles) != 1 {
					t.Errorf("found %+v, %v; %d profiles", found, err, len(snap.Profiles))
				}
				return
			}
			errtest.Route(t, err, tt.route)
			if len(snap.Profiles) != 0 {
				t.Errorf("the profile was added: %+v", snap.Profiles)
			}
		})
	}
	t.Run("a profile without scripts asks nothing", func(t *testing.T) {
		storagetest.Isolate(t)
		ctx := context.Background()
		a, err := app.Open(ctx, app.Options{Env: os.LookupEnv})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := a.Close(); err != nil {
				t.Error(err)
			}
		}()
		path := profilestest.WriteProfile(t, t.TempDir(), "Plain.ovpn", profilestest.NewPKI(t), profilestest.Options{})
		c := cli{tty: filepath.Join(t.TempDir(), "no-tty")}
		found, err := c.findProfile(ctx, a, strings.TrimSuffix(path, ".ovpn"))
		if err != nil || !found.Added || found.Name != "Plain" {
			t.Errorf("found %+v, %v", found, err)
		}
	})
}
