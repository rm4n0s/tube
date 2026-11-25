// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// networkHarness is a harness on the Network tab of the fake.
func networkHarness(t *testing.T, f *apptest.Fake, width, height int) *harness {
	t.Helper()
	h := runHarness(t, f, width, height)
	h.keys("tab", "tab")
	if h.m().run.tab != tabNetwork {
		t.Fatal("the Network tab is not shown")
	}
	return h
}

// calls are the fake's calls after Connect and Start.
func calls(f *apptest.Fake) []string { return f.Calls()[2:] }

func TestGoldenNetwork(t *testing.T) {
	tests := []struct {
		name   string
		change func(*app.RunState)
	}{
		{"connected", func(*app.RunState) {}},
		{"reconnecting", func(rs *app.RunState) { rs.State = "reconnecting" }},
		{"disconnected", func(rs *app.RunState) { rs.State = "disconnected" }},
		{"refused", func(rs *app.RunState) { rs.State = "refused" }},
	}
	for _, tt := range tests {
		for _, size := range [][2]int{{100, 30}, {60, 16}} {
			name := "network-" + tt.name + "-" + strconv.Itoa(size[0])
			t.Run(name, func(t *testing.T) {
				f := apptest.Running()
				tt.change(&f.Run)
				h := networkHarness(t, f, size[0], size[1])
				h.golden(name)
			})
		}
	}
	t.Run("switching", func(t *testing.T) {
		f := apptest.Running()
		f.Block = map[string]chan struct{}{"Switch": make(chan struct{})}
		h := networkHarness(t, f, 100, 30)
		h.selectProfile("id-home")
		done := h.held("enter")
		h.golden("network-switching-100")
		h.has("Connecting to Home… 0 s   esc cancels")
		close(f.Block["Switch"])
		h.send(<-done)
	})
}

func TestTheSelectionStartsOnTheProfileInUse(t *testing.T) {
	h := networkHarness(t, apptest.Running(), 100, 30)
	if h.selectedID() != "id-work" {
		t.Errorf("the selection is on %s", h.selectedID())
	}
	h.has("firefox uses Work.")
	h.has(">*Work")
	h.has("in use")
}

func TestSwitchFlow(t *testing.T) {
	f := apptest.Running()
	h := networkHarness(t, f, 100, 30)
	h.selectProfile("id-home")
	h.key("enter")
	if got := calls(f); !slices.Equal(got, []string{"Switch(id-home, nil, false)"}) {
		t.Errorf("Calls = %q", got)
	}
	h.has("Now using Home.")
}

func TestSwitchWithALogin(t *testing.T) {
	f := apptest.Running()
	f.Run.ProfileID, f.Run.ProfileName = "id-home", "Home"
	h := networkHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.key("enter")
	if h.m().page != pageLogin || h.m().login.user.value() != "ada" {
		t.Fatalf("the login page is not shown: %v", h.m().page)
	}
	h.typeText("hunter2")
	if strings.Contains(h.m().View().Content, "hunter2") {
		t.Error("the password is drawn")
	}
	h.key("enter")
	if got := calls(f); !slices.Equal(got, []string{"Switch(id-work, ada, ***, true)"}) {
		t.Errorf("Calls = %q", got)
	}
	if h.m().page != pageRun || h.m().run.tab != tabNetwork {
		t.Error("the Network tab is not shown again")
	}
	t.Run("esc", func(t *testing.T) {
		f := apptest.Running()
		f.Run.ProfileID = "id-home"
		h := networkHarness(t, f, 100, 30)
		h.selectProfile("id-work")
		h.keys("enter", "esc")
		if h.m().page != pageRun || h.m().run.tab != tabNetwork || len(calls(f)) != 0 {
			t.Errorf("page %v, calls %q", h.m().page, calls(f))
		}
	})
}

func TestSwitchFails(t *testing.T) {
	t.Run("a refused login", func(t *testing.T) {
		f := apptest.Running()
		f.Run.ProfileID = "id-home"
		f.Fail = map[string]error{"Switch": errors.New("AuthFailed", "the server refused the credentials")}
		h := networkHarness(t, f, 100, 30)
		h.selectProfile("id-work")
		h.key("enter")
		h.typeText("wrong")
		h.key("enter")
		if h.m().page != pageLogin || h.m().login.pass.value() != "" || h.m().login.user.value() != "ada" {
			t.Errorf("page %v", h.m().page)
		}
		h.has("The VPN server refused the user name or the password.")
	})
	t.Run("another error", func(t *testing.T) {
		f := apptest.Running()
		f.Fail = map[string]error{"Switch": errors.New("ServerPollTimeout", "nothing answered")}
		h := networkHarness(t, f, 100, 30)
		h.selectProfile("id-home")
		h.key("enter")
		h.has("Error: The VPN server did not answer.")
		h.has("firefox uses Work.")
	})
}

func TestCancelASwitch(t *testing.T) {
	f := apptest.Running()
	f.Block = map[string]chan struct{}{"Switch": make(chan struct{})}
	h := networkHarness(t, f, 100, 30)
	h.selectProfile("id-home")
	done := h.held("enter")
	h.key("esc")
	msg := <-done
	if s, ok := msg.(switchedMsg); !ok || !errors.Is(s.err, context.Canceled) {
		t.Fatalf("the switch ended with %#v", msg)
	}
	h.send(msg)
	h.has(cancelled)
}

func TestTheProfileInUse(t *testing.T) {
	f := apptest.Running()
	h := networkHarness(t, f, 100, 30)
	h.key("enter")
	if len(calls(f)) != 0 {
		t.Errorf("Calls = %q", calls(f))
	}
	h.has("firefox already uses Work.")
}

func TestDisconnectFlow(t *testing.T) {
	f := apptest.Running()
	h := networkHarness(t, f, 100, 30)
	h.key("x")
	h.has("Disconnect? firefox keeps running without network.")
	h.key("enter")
	if len(calls(f)) != 0 {
		t.Errorf("enter disconnected: %q", calls(f))
	}
	h.keys("x", "y")
	if got := calls(f); !slices.Equal(got, []string{"Disconnect()"}) {
		t.Errorf("Calls = %q", got)
	}
	d := apptest.Running()
	d.Run.State = "disconnected"
	hd := networkHarness(t, d, 100, 30)
	hd.key("x")
	if hd.m().ask != nil {
		t.Error("x asked with nothing to disconnect")
	}
}

func TestConnectAgainAfterADisconnect(t *testing.T) {
	f := apptest.Running()
	f.Run.State = "disconnected"
	f.Run.ProfileID, f.Run.ProfileName = "id-home", "Home"
	h := networkHarness(t, f, 100, 30)
	if h.selectedID() != "id-home" {
		t.Fatalf("the selection is on %s", h.selectedID())
	}
	h.key("enter")
	if got := calls(f); !slices.Equal(got, []string{"Switch(id-home, nil, false)"}) {
		t.Errorf("Calls = %q", got)
	}
}

func TestNetworkABrokenProfile(t *testing.T) {
	f := apptest.Running()
	h := networkHarness(t, f, 100, 30)
	h.selectProfile("0123456789abcdef")
	h.key("enter")
	if len(calls(f)) != 0 {
		t.Errorf("Calls = %q", calls(f))
	}
	h.has("This profile cannot be used: its profile cannot be read")
}

func TestAfterTheEnd(t *testing.T) {
	f := apptest.Running()
	f.Run.AppRunning, f.Run.Exit = false, 0
	h := networkHarness(t, f, 100, 30)
	for _, key := range []string{"enter", "x"} {
		h.selectProfile("id-home")
		h.key(key)
		h.has("firefox has ended.")
	}
	if len(calls(f)) != 0 {
		t.Errorf("Calls = %q", calls(f))
	}
}

func TestQuitOnTheNetworkTab(t *testing.T) {
	h := networkHarness(t, apptest.Running(), 100, 30)
	h.key("q")
	h.has("firefox is still running. Stop it and quit?")
}

func TestCtrlCOnTheLoginOfASwitch(t *testing.T) {
	f := apptest.Running()
	f.Run.ProfileID = "id-home"
	h := networkHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.key("enter")
	h.key("ctrl+c")
	if h.quit || h.m().page != pageRun {
		t.Errorf("quit %v, page %v", h.quit, h.m().page)
	}
}
