// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// tagged is an error with a Tag and metadata, as the code that fails makes it.
func tagged(tag string, metadata ...any) error {
	e := &errors.Error{Tag: tag, Message: "the message of " + tag, Metadata: map[string]any{}}
	for i := 0; i+1 < len(metadata); i += 2 {
		e.Metadata[metadata[i].(string)] = metadata[i+1]
	}
	return e
}

// realErrors are errors made the real way: a profile the parser refuses, a name that is taken.
func realErrors(t *testing.T) (unsupported, taken error) {
	t.Helper()
	dir := t.TempDir()
	pki := profilestest.NewPKI(t)
	profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Extra: []string{"comp-lzo"}})
	_, _, unsupported = profiles.Load(dir, "a.ovpn")
	store := profiles.NewStore(storagetest.Isolate(t).Config, clock.System())
	ctx := context.Background()
	path := profilestest.WriteProfile(t, t.TempDir(), "b.ovpn", pki, profilestest.Options{})
	if _, err := store.Add(ctx, path, "Work", profiles.AddOptions{}); err != nil {
		t.Fatal(err)
	}
	home, err := store.Add(ctx, path, "Home", profiles.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	taken = store.Rename(ctx, home.ID, "work")
	return unsupported, taken
}

func TestExplain(t *testing.T) {
	unsupported, taken := realErrors(t)
	p := profiles.ProblemOf(unsupported)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"an option that is not supported, from the parser", unsupported,
			`The profile uses the option "comp-lzo" on line ` + strconv.Itoa(p.Line) + `, which Tube does not support: ` + p.Reason + `. (UnsupportedOption)`},
		{"an option that is not supported", tagged("UnsupportedOption", "option", "fragment", "line", 7, "reason", "it is not needed"),
			`The profile uses the option "fragment" on line 7, which Tube does not support: it is not needed. (UnsupportedOption)`},
		{"an option that is not supported, without a line and a reason", tagged("UnsupportedOption", "option", "fragment"),
			`The profile uses the option "fragment", which Tube does not support. (UnsupportedOption)`},
		{"a bad option", tagged("BadOption", "option", "keepalive", "line", 3, "reason", "it takes two numbers"),
			`The option "keepalive" on line 3 of the profile is not valid: it takes two numbers. (BadOption)`},
		{"a missing option", tagged("MissingOption", "option", "ca"), `The profile has no "ca". (MissingOption)`},
		{"a file", tagged("FileUnavailable", "option", "tls-crypt"),
			`A file that the profile names with "tls-crypt" cannot be used. Put it next to the profile, or inside the profile. (FileUnavailable)`},
		{"a syntax error", tagged("ProfileSyntax", "line", 4), `The profile cannot be read: a mistake on line 4. (ProfileSyntax)`},
		{"too large", tagged("ProfileTooLarge"), `The profile is too large. (ProfileTooLarge)`},
		{"unreadable", tagged("ProfileReadFailed"), `The profile cannot be read. (ProfileReadFailed)`},
		{"a wrong password", tagged("AuthFailed"), `The VPN server refused the user name or the password. (AuthFailed)`},
		{"no answer", tagged("PingTimeout"), `The VPN server did not answer. (PingTimeout)`},
		{"a certificate", tagged("TLSHandshakeFailed"), `The certificate of the VPN server was not accepted. (TLSHandshakeFailed)`},
		{"the server ended", tagged("ServerHalt"), `The VPN server closed the connection. (ServerHalt)`},
		{"not supported by the server", tagged("UnsupportedPushedOption"), `The VPN server asks for something that Tube does not support. (UnsupportedPushedOption)`},
		{"cancelled", context.Canceled, `Cancelled.`},
		{"credentials", tagged(vpn.TagCredentialsNeeded), `This profile needs a user name and a password. (VPNCredentialsNeeded)`},
		{"scripts", tagged(profiles.TagProfileRunsScripts, "options", "up, down"), `This profile runs scripts (up, down). Tube never runs them. (ProfileRunsScripts)`},
		{"a name that is taken", taken, `Another profile is already called "work". (ProfileNameTaken)`},
		{"an empty name", tagged(profiles.TagProfileNameEmpty), `A profile needs a name. (ProfileNameEmpty)`},
		{"a long name", tagged(profiles.TagProfileNameTooLong), `The name is too long (64 characters at most). (ProfileNameTooLong)`},
		{"a large file", tagged(profiles.TagProfileFileTooLarge, "file", "big.key"), `The file "big.key" of the profile is too large. (ProfileFileTooLarge)`},
		{"a DNS server", tagged(profiles.TagProfileDNSInvalid, "value", "nope"), `"nope" is not an IP address. (ProfileDNSInvalid)`},
		{"too many DNS servers", tagged(profiles.TagProfileDNSTooMany), `A profile can have 4 DNS servers at most. (ProfileDNSTooMany)`},
		{"too many DNS servers, stored", tagged(profiles.TagProfileTooManyDNS), `A profile can have 4 DNS servers at most. (ProfileTooManyDNS)`},
		{"no such profile", tagged(TagProfileNotFound, "name", "nothing"), `There is no profile called "nothing". (AppProfileNotFound)`},
		{"an ambiguous name", tagged(TagProfileAmbiguous, "name", "Home"),
			`More than one profile is called "Home" when the case is ignored. Write the name exactly. (AppProfileAmbiguous)`},
		{"no user namespace", tagged(sandbox.TagUserNamespaceDenied),
			`This machine does not let an ordinary user make a user namespace, which Tube needs. See docs/GUIDE.md ("What Tube needs"). (SandboxUserNamespaceDenied)`},
		{"no TUN", tagged(sandbox.TagTunUnavailable), `Tube cannot open /dev/net/tun. (SandboxTunUnavailable)`},
		{"not in PATH", tagged(sandbox.TagLookPathFailed, "name", "firefx"), `The program "firefx" was not found. (SandboxLookPathFailed)`},
		{"not found when started", tagged(sandbox.TagAppNotFound), `The program was not found. (SandboxAppNotFound)`},
		{"not executable", tagged(sandbox.TagAppNotExecutable), `The program cannot be executed. (SandboxAppNotExecutable)`},
		{"another error of Tube", tagged("SomethingElse"), `the message of SomethingElse (SomethingElse)`},
		{"an error that is not Tube's", context.DeadlineExceeded, `Cancelled.`},
		{"a plain error", strings.NewReader("").UnreadByte(), `strings.Reader.UnreadByte: at beginning of string`},
		{"nil", nil, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Explain(tt.err); got != tt.want {
				t.Errorf("Explain =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestExplainKeepsSecretsOut(t *testing.T) {
	for _, tag := range []string{"AuthFailed", "UnsupportedOption", TagProfileNotFound, sandbox.TagLookPathFailed, vpn.TagCredentialsNeeded} {
		err := &errors.Error{Tag: tag, Message: "the password is hunter2", Metadata: map[string]any{"password": "hunter2", "name": "x"}}
		if got := Explain(err); strings.Contains(got, "hunter2") {
			t.Errorf("Explain of %s has the secret: %q", tag, got)
		}
	}
}

func TestExplainMakesTextSafe(t *testing.T) {
	got := Explain(tagged(TagProfileNotFound, "name", "evil\x1b[2J"))
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("Explain has an escape: %q", got)
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{tagged(sandbox.TagLookPathFailed), 127},
		{tagged(sandbox.TagAppNotFound), 127},
		{tagged(sandbox.TagAppNotExecutable), 126},
		{tagged(TagProfileNotFound), 125},
		{context.Canceled, 125},
	}
	for _, tt := range tests {
		if got := ExitCode(tt.err); got != tt.want {
			t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}
