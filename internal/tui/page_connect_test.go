// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app/apptest"
	"github.com/rm4n0s/tube/internal/core/clock"
)

// select moves the selection to the profile with the id.
func (h *harness) selectProfile(id string) {
	h.t.Helper()
	h.key("home")
	for range len(h.m().snap.Profiles) {
		if h.selectedID() == id {
			return
		}
		h.key("down")
	}
	h.t.Fatalf("no profile %s", id)
}

// held presses a key whose command blocks: the command runs in a goroutine, and the message
// it produces comes on the channel, for h.send.
func (h *harness) held(name string) <-chan tea.Msg {
	h.t.Helper()
	m, cmd := h.model.Update(keyMsg(name))
	h.model = m
	out := make(chan tea.Msg, 1)
	if cmd == nil {
		h.t.Fatalf("the key %s returned no command", name)
	}
	go func() { out <- cmd() }()
	return out
}

// withClock gives the model a fake clock of the test.
func (h *harness) withClock() *clock.Fake {
	clk := clock.NewFake(apptest.SampleTime)
	m := h.m()
	m.clock = clk
	h.model = m
	return clk
}

func TestGoldenLogin(t *testing.T) {
	t.Run("remembered", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		h.selectProfile("id-work")
		h.key("enter")
		h.golden("login-remembered")
		if h.m().login.focus != focusPassword {
			t.Error("the password field does not have the focus")
		}
	})
	t.Run("empty", func(t *testing.T) {
		f := apptest.Sample()
		f.Snap.Profiles[2].Username = ""
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-work")
		h.key("enter")
		h.golden("login-empty")
		if h.m().login.focus != focusUser {
			t.Error("the user name field does not have the focus")
		}
	})
}

func TestGoldenConnecting(t *testing.T) {
	f := apptest.Sample()
	f.Block = map[string]chan struct{}{"Connect": make(chan struct{})}
	h := newHarness(t, f, 100, 30)
	clk := h.withClock()
	h.selectProfile("id-home")
	done := h.held("enter")
	h.golden("connecting-0")
	clk.Advance(12 * time.Second)
	h.send(tickMsg{})
	h.golden("connecting-12")
	close(f.Block["Connect"])
	h.send(<-done)
	if !h.quit || !h.m().Result().Run {
		t.Error("the TUI did not quit to run")
	}
}

func TestThePasswordIsNeverDrawn(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	h.selectProfile("id-work")
	h.key("enter")
	h.typeText("hun")
	three := h.m().View().Content
	h.typeText("ter2")
	seven := h.m().View().Content
	if strings.Contains(seven, "hunter2") || strings.Contains(seven, "hun") || strings.Contains(seven, "*") {
		t.Error("the password is drawn")
	}
	if three != seven {
		t.Error("the screen changes with the length of the password")
	}
	h.paste("pasted-secret")
	if strings.Contains(h.m().View().Content, "pasted-secret") {
		t.Error("a pasted password is drawn")
	}
}

func TestConnectFlow(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-home")
	h.key("enter")
	if got := f.Calls(); !slices.Equal(got, []string{"Connect(id-home, nil, false)"}) {
		t.Errorf("Calls = %q", got)
	}
	if !h.quit || !h.m().Result().Run {
		t.Error("the TUI did not quit to run")
	}
	t.Run("from the profile's page", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.open("id-home")
		h.key("enter")
		if got := f.Calls(); !slices.Equal(got, []string{"Connect(id-home, nil, false)"}) {
			t.Errorf("Calls = %q", got)
		}
	})
}

func TestLoginFlow(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"remember", nil, "Connect(id-work, ada, ***, true)"},
		{"do not remember", []string{"tab", "space", "shift+tab"}, "Connect(id-work, ada, ***, false)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := apptest.Sample()
			h := newHarness(t, f, 100, 30)
			h.selectProfile("id-work")
			h.key("enter")
			h.keys(tt.keys...)
			h.typeText("hunter2")
			h.key("enter")
			if got := f.Calls(); !slices.Equal(got, []string{tt.want}) {
				t.Errorf("Calls = %q", got)
			}
			if !h.m().Result().Run {
				t.Error("the TUI did not quit to run")
			}
		})
	}
	t.Run("empty fields", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-work")
		h.key("enter")
		h.key("enter")
		h.has(needBoth)
		if len(f.Calls()) != 0 {
			t.Errorf("Calls = %q", f.Calls())
		}
	})
	t.Run("esc", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-work")
		h.key("enter")
		h.typeText("hunter2")
		h.key("esc")
		if h.m().page != pageProfiles || h.m().login != nil {
			t.Error("esc did not go back to the list")
		}
		h.key("enter")
		if h.m().login.pass.value() != "" {
			t.Error("the password was kept")
		}
	})
}

func TestAWrongPassword(t *testing.T) {
	f := apptest.Sample()
	// app.IsAuthFailed is true for the Tag of gopenvpn-conn's refusal (the TUI may not import
	// vpn, docs/SPEC.md §4.2 rule 2).
	f.Fail = map[string]error{"Connect": errors.New("AuthFailed", "the server refused the credentials")}
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.key("enter")
	h.typeText("wrong")
	h.key("enter")
	m := h.m()
	if m.page != pageLogin || m.login.user.value() != "ada" || m.login.pass.value() != "" || h.quit {
		t.Errorf("page %v, user %q, password kept %v, quit %v", m.page, m.login.user.value(), m.login.pass.value() != "", h.quit)
	}
	h.has("The VPN server refused the user name or the password.")
}

func TestAConnectError(t *testing.T) {
	f := apptest.Sample()
	f.Fail = map[string]error{"Connect": errors.New("ServerPollTimeout", "nothing answered")}
	h := newHarness(t, f, 100, 30)
	h.open("id-home")
	h.key("enter")
	if h.m().page != pageProfiles || h.quit {
		t.Errorf("page %v, quit %v", h.m().page, h.quit)
	}
	h.has("Error: ")
}

func TestCancel(t *testing.T) {
	for _, withLogin := range []bool{false, true} {
		name := "without a login"
		if withLogin {
			name = "after the login"
		}
		t.Run(name, func(t *testing.T) {
			f := apptest.Sample()
			f.Block = map[string]chan struct{}{"Connect": make(chan struct{})}
			h := newHarness(t, f, 100, 30)
			var done <-chan tea.Msg
			if withLogin {
				h.selectProfile("id-work")
				h.key("enter")
				h.typeText("hunter2")
				done = h.held("enter")
			} else {
				h.selectProfile("id-home")
				done = h.held("enter")
			}
			h.key("esc")
			h.has(cancelling)
			msg := <-done
			if c, ok := msg.(connectedMsg); !ok || !errors.Is(c.err, context.Canceled) {
				t.Fatalf("the connect ended with %#v", msg)
			}
			h.send(msg)
			want := pageProfiles
			if withLogin {
				want = pageLogin
			}
			if h.m().page != want || h.quit {
				t.Errorf("page %v, quit %v", h.m().page, h.quit)
			}
			h.has(cancelled)
			if withLogin && h.m().login.pass.value() != "" {
				t.Error("the password was kept")
			}
		})
	}
	t.Run("ctrl+c", func(t *testing.T) {
		f := apptest.Sample()
		f.Block = map[string]chan struct{}{"Connect": make(chan struct{})}
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-home")
		done := h.held("enter")
		h.key("ctrl+c")
		if c, ok := (<-done).(connectedMsg); !ok || !errors.Is(c.err, context.Canceled) {
			t.Error("ctrl+c did not cancel the connect")
		}
		if !h.quit || h.m().Result().Run {
			t.Error("ctrl+c did not quit without running")
		}
	})
}

func TestEnterWithoutATarget(t *testing.T) {
	f := apptest.Sample()
	f.Snap.Target = nil
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-home")
	h.key("enter")
	if h.m().page != pageProfile || len(f.Calls()) != 0 {
		t.Errorf("page %v, calls %q", h.m().page, f.Calls())
	}
	h.key("enter")
	if len(f.Calls()) != 0 {
		t.Errorf("enter on the page: %q", f.Calls())
	}
}

func TestABrokenProfile(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.selectProfile("0123456789abcdef")
	h.key("enter")
	h.has("This profile cannot be used: its profile cannot be read: a mistake on line 3")
	if len(f.Calls()) != 0 || h.m().page != pageProfiles {
		t.Errorf("page %v, calls %q", h.m().page, f.Calls())
	}
}
