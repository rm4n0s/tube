// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// remembering is the sample whose profile Work has a remembered password.
func remembering() *apptest.Fake {
	f := apptest.Sample()
	f.Snap.Profiles[2].HasPassword = true
	return f
}

// refusedLogin is the error of a login that the server refused. app.IsAuthFailed is true for
// the Tag of gopenvpn-conn's refusal (the TUI may not import the library).
func refusedLogin() error {
	return errors.New("AuthFailed", "the server refused the credentials")
}

func TestARememberedPasswordConnectsAtOnce(t *testing.T) {
	f := remembering()
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.key("enter")
	if got := f.Calls(); !slices.Equal(got, []string{"Connect(id-work, nil, keep nothing)", "Start()"}) {
		t.Errorf("Calls = %q", got)
	}
	if h.m().page != pageRun {
		t.Errorf("the page is %v", h.m().page)
	}
	// From the profile's page too, and with a double click.
	for _, keys := range [][]string{{"e", "enter"}, nil} {
		f := remembering()
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-work")
		if keys == nil {
			h.doubleClickOn("vpn1.example.com")
		} else {
			h.keys(keys...)
		}
		if !called(f, "Connect(id-work, nil, keep nothing)") || h.m().login != nil {
			t.Errorf("with %q: Calls = %q", keys, f.Calls())
		}
	}
}

func TestARememberedPasswordThatIsRefused(t *testing.T) {
	for _, from := range []string{"the list", "the profile's page"} {
		t.Run(from, func(t *testing.T) {
			f := remembering()
			f.Fail = map[string]error{"Connect": refusedLogin()}
			h := newHarness(t, f, 100, 30)
			h.selectProfile("id-work")
			if from == "the profile's page" {
				h.key("e")
			}
			h.key("enter")
			// The login page asks for another password, and says why.
			m := h.m()
			if m.page != pageLogin || m.login == nil || m.login.user.value() != "ada" || m.login.focus != focusPassword || !m.login.keep {
				t.Fatalf("after the refusal: the page %v, the login %+v", m.page, m.login)
			}
			h.has("Error: The VPN server refused the user name or the password.")
			f.Fail = nil
			h.typeText("new-password")
			h.key("enter")
			if got := f.Calls(); !slices.Equal(got, []string{"Connect(id-work, nil, keep nothing)", "Connect(id-work, ada, ***, keep the password)", "Start()"}) {
				t.Errorf("Calls = %q", got)
			}
		})
	}
	// Esc on that login page goes back to where the connect was started.
	f := remembering()
	f.Fail = map[string]error{"Connect": refusedLogin()}
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.keys("e", "enter", "esc")
	if h.m().page != pageProfile {
		t.Errorf("esc went to the page %v", h.m().page)
	}
	// Another error of a remembered password is an error on the list, as before.
	o := remembering()
	o.Fail = map[string]error{"Connect": errors.New("ConnectTimeout", "the server did not answer")}
	g := newHarness(t, o, 100, 30)
	g.selectProfile("id-work")
	g.key("enter")
	if g.m().page != pageProfiles || g.m().login != nil {
		t.Errorf("after another error the page is %v", g.m().page)
	}
}

func TestARememberedPasswordOnTheNetworkTab(t *testing.T) {
	f := apptest.Running()
	f.Snap.Profiles[2].HasPassword = true
	f.Run.ProfileID, f.Run.ProfileName = "id-home", "Home"
	h := runHarness(t, f, 100, 30)
	h.keys("tab", "tab")
	h.selectProfile("id-work")
	h.key("enter")
	if got := calls(f); !slices.Equal(got, []string{"Switch(id-work, nil, keep nothing)"}) {
		t.Fatalf("Calls = %q", got)
	}
	if h.m().page != pageRun {
		t.Fatalf("the page is %v", h.m().page)
	}
	// Refused: the login page, for the switch.
	f.Fail = map[string]error{"Switch": refusedLogin()}
	h.key("enter")
	m := h.m()
	if m.page != pageLogin || m.login == nil || !m.login.forSwitch || m.login.back != pageRun {
		t.Fatalf("after the refusal: the page %v, the login %+v", m.page, m.login)
	}
	h.has("[ Use ]")
	h.has("Error: The VPN server refused the user name or the password.")
	f.Fail = nil
	h.typeText("new")
	h.key("enter")
	if got := calls(f); got[len(got)-1] != "Switch(id-work, ada, ***, keep the password)" {
		t.Errorf("Calls = %q", got)
	}
}

func TestForgetThePassword(t *testing.T) {
	f := remembering()
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.key("e")
	h.golden("profile-remembered-100")
	h.has("Password         remembered")
	h.has("p forget password")
	h.key("p")
	if h.m().ask == nil {
		t.Fatal("no question")
	}
	h.has(`Forget the password of "Work"? Tube will ask for it the next`)
	h.key("n")
	if len(f.Calls()) != 0 {
		t.Fatalf("no forgot: %q", f.Calls())
	}
	h.clickOn("p forget password")
	h.clickOn("[y] Forget")
	if got := f.Calls(); !slices.Equal(got, []string{"ForgetPassword(id-work)"}) {
		t.Errorf("Calls = %q", got)
	}
	h.has(forgotPassword)
	// The app forgot it: the snapshot says so, the page too, and the key is gone.
	f.Snap.Profiles[2].HasPassword = false
	f.Emit()
	h.changed()
	h.has("Password         asked each time")
	if strings.Contains(h.screen(), "forget password") {
		t.Error("the key is still on the last line")
	}
	h.key("p")
	if h.m().ask != nil || len(f.Calls()) != 1 {
		t.Errorf("p with nothing remembered: the question %v, Calls %q", h.m().ask, f.Calls())
	}
	// A failure is said.
	g := remembering()
	g.Fail = map[string]error{"ForgetPassword": errors.New("ProfilePasswordForgetFailed", "it cannot be removed")}
	e := newHarness(t, g, 100, 30)
	e.selectProfile("id-work")
	e.keys("e", "p", "y")
	e.has("Error: ")
	// A profile without a password has no such line.
	n := newHarness(t, apptest.Sample(), 100, 30)
	n.selectProfile("id-home")
	n.key("e")
	if strings.Contains(n.screen(), "Password ") {
		t.Errorf("a profile with a certificate only has a line of the password:\n%s", n.screen())
	}
}

func TestTheMarksOfTheLoginPage(t *testing.T) {
	tests := []struct {
		name string
		do   func(h *harness)
		want string
	}{
		{"both are on at first", func(*harness) {}, "keep the password"},
		{"a click on the mark of the password", func(h *harness) { h.clickOn("[x] remember the password") }, "keep the user name, forget the password"},
		{"two clicks", func(h *harness) { h.clickOn("[x] remember the password"); h.clickOn("[ ] remember the password") }, "keep the password"},
		{"space on it", func(h *harness) { h.keys("tab", "tab", "space") }, "keep the user name, forget the password"},
		{"both off", func(h *harness) {
			h.clickOn("[x] remember the password")
			h.clickOn("[x] remember the user name")
		}, "forget the password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := apptest.Sample()
			h := loginHarness(t, f, 100, 30)
			h.typeText("hunter2")
			tt.do(h)
			h.clickOn("[ Connect ]")
			if want := "Connect(id-work, ada, ***, " + tt.want + ")"; !called(f, want) {
				t.Errorf("Calls = %q, want %q", f.Calls(), want)
			}
		})
	}
	// The focus goes around the two fields and the two marks.
	h := loginHarness(t, apptest.Sample(), 100, 30)
	for _, want := range []int{focusRemember, focusKeep, focusUser, focusPassword} {
		h.key("tab")
		if got := h.m().login.focus; got != want {
			t.Errorf("after tab the focus is %d, want %d", got, want)
		}
	}
	h.key("shift+tab")
	if got := h.m().login.focus; got != focusUser {
		t.Errorf("after shift+tab the focus is %d", got)
	}
}

func TestGoldenLoginSizes(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {40, 10}} {
		h := loginHarness(t, apptest.Sample(), size[0], size[1])
		h.typeText("hunter2")
		h.golden("login-" + map[int]string{60: "60", 40: "40"}[size[0]])
		// In ten rows the fields, both marks and the buttons are all there.
		for _, part := range []string{userLabel, passwordLabel + "   *******", rememberText, keepText, connectButton} {
			h.has(part)
		}
	}
}
