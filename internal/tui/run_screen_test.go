// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// runHarness is a harness on the run screen of the fake: enter on Home connects and starts.
func runHarness(t *testing.T, f *apptest.Fake, width, height int) *harness {
	t.Helper()
	h := newHarness(t, f, width, height)
	h.selectProfile("id-home")
	h.key("enter")
	if h.m().page != pageRun {
		t.Fatalf("the run screen is not shown: page %v, %q", h.m().page, h.screen())
	}
	return h
}

// changed tells the model that the app changed, as the subscription does.
func (h *harness) changed() {
	h.t.Helper()
	h.send(changedMsg{})
}

func TestGoldenStatus(t *testing.T) {
	tests := []struct {
		name   string
		change func(*app.RunState)
	}{
		{"connected", func(*app.RunState) {}},
		{"reconnecting", func(rs *app.RunState) {
			rs.State, rs.Since = "reconnecting", apptest.SampleTime.Add(-7*time.Second)
			rs.Down.Rate, rs.Up.Rate = 0, 0
		}},
		{"disconnected", func(rs *app.RunState) { rs.State, rs.Down.Rate, rs.Up.Rate = "disconnected", 0, 0 }},
		{"refused", func(rs *app.RunState) { rs.State, rs.Down.Rate, rs.Up.Rate = "refused", 0, 0 }},
		{"ended", func(rs *app.RunState) { rs.AppRunning, rs.Exit = false, 1 }},
		{"many-events", func(rs *app.RunState) {
			for i := range 30 {
				rs.Events = append(rs.Events, app.Event{Time: apptest.SampleTime.Add(time.Duration(i) * time.Second), Text: "event " + strconv.Itoa(i)})
			}
		}},
	}
	for _, tt := range tests {
		for _, size := range [][2]int{{100, 30}, {60, 16}} {
			name := "status-" + tt.name + "-" + strconv.Itoa(size[0])
			t.Run(name, func(t *testing.T) {
				f := apptest.Running()
				tt.change(&f.Run)
				h := runHarness(t, f, size[0], size[1])
				h.golden(name)
			})
		}
	}
}

func TestGoldenOutput(t *testing.T) {
	t.Run("the end", func(t *testing.T) {
		h := runHarness(t, apptest.Running(), 100, 30)
		h.key("tab")
		h.golden("output-end")
		h.has("firefox: line 40 of the output")
		if strings.Contains(h.screen(), "line 16 of") {
			t.Error("more lines than fit")
		}
	})
	t.Run("scrolled back", func(t *testing.T) {
		h := runHarness(t, apptest.Running(), 100, 30)
		h.keys("tab", "pgup")
		h.golden("output-scrolled")
		h.has("scrolled back · end: follow")
	})
	t.Run("empty", func(t *testing.T) {
		f := apptest.Running()
		f.Out = nil
		h := runHarness(t, f, 100, 30)
		h.key("tab")
		h.golden("output-empty")
	})
	t.Run("a long line", func(t *testing.T) {
		f := apptest.Running()
		f.Out = append(f.Out, strings.Repeat("wide ", 40))
		h := runHarness(t, f, 100, 30)
		h.key("tab")
		h.golden("output-long-line")
		if !strings.Contains(h.screen(), "wide…") {
			t.Error("the long line does not end in …")
		}
	})
}

func TestGoldenRunHeader(t *testing.T) {
	for _, w := range []int{100, 60, 40} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			h := runHarness(t, apptest.Running(), w, 12)
			h.golden("run-header-" + strconv.Itoa(w))
		})
	}
}

func TestTabs(t *testing.T) {
	h := runHarness(t, apptest.Running(), 100, 30)
	want := []string{"[Status]  Output  Network", "Status  [Output]  Network", "Status  Output  [Network]", "[Status]  Output  Network"}
	for i, w := range want {
		if i > 0 {
			h.key("tab")
		}
		h.has(w)
	}
	h.key("shift+tab")
	h.has("Status  Output  [Network]")
}

func TestTheStartFlow(t *testing.T) {
	f := apptest.Sample()
	h := runHarness(t, f, 100, 30)
	if got := f.Calls(); !slices.Equal(got, []string{"Connect(id-home, nil, keep nothing)", "Start()"}) {
		t.Errorf("Calls = %q", got)
	}
	h.has("[Status]")
	if cols, rows := f.Size(); cols != 100 || rows != 24 {
		t.Errorf("Size = %d, %d", cols, rows)
	}
	t.Run("Start fails", func(t *testing.T) {
		f := apptest.Sample()
		f.Fail = map[string]error{"Start": errors.New("SandboxAppNotFound", "the program was not found")}
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-home")
		h.key("enter")
		if h.m().page != pageProfiles || h.m().Result().Started {
			t.Errorf("page %v, started %v", h.m().page, h.m().Result().Started)
		}
		h.has("Error: ")
	})
}

func TestKeysGoToTheApplication(t *testing.T) {
	f := apptest.Running()
	h := runHarness(t, f, 100, 30)
	h.typeText("x")
	if len(f.Typed()) != 0 {
		t.Errorf("the Status tab sent %q", f.Typed())
	}
	h.key("tab")
	h.typeText("ls")
	h.keys("enter", "ctrl+c", "backspace", "ctrl+d", "ctrl+u", "esc", "space")
	if got := string(f.Typed()); got != "ls\r\x03\x7f\x04\x15\x1b " {
		t.Errorf("Typed = %q", got)
	}
	h.key("pgup")
	h.key("tab")
	if got := string(f.Typed()); got != "ls\r\x03\x7f\x04\x15\x1b " {
		t.Errorf("pgup or tab was sent: %q", got)
	}
	if h.m().run.tab != tabNetwork || h.quit {
		t.Error("tab did not change the tab")
	}
}

func TestPaste(t *testing.T) {
	f := apptest.Running()
	h := runHarness(t, f, 100, 30)
	h.paste("on status")
	h.key("tab")
	h.paste("echo hi\n")
	if got := string(f.Typed()); got != "echo hi\n" {
		t.Errorf("Typed = %q", got)
	}
}

func TestScrolling(t *testing.T) {
	f := apptest.Running()
	h := runHarness(t, f, 100, 30)
	h.keys("tab", "pgup")
	before := h.screen()
	f.Out = append(f.Out, "a new line")
	h.changed()
	if h.screen() != before {
		t.Error("the window moved while scrolled back")
	}
	h.key("end")
	h.has("a new line")
	h.key("home")
	h.has("firefox: line 1 of the output")
	h.key("pgdown")
	h.key("pgdown")
	if !h.m().run.follow {
		t.Error("pgdown to the end does not follow")
	}
}

func TestResize(t *testing.T) {
	f := apptest.Running()
	h := runHarness(t, f, 100, 30)
	h.resize(120, 40)
	if cols, rows := f.Size(); cols != 120 || rows != 34 {
		t.Errorf("Size = %d, %d", cols, rows)
	}
}

func TestQuitWhileRunning(t *testing.T) {
	f := apptest.Running()
	h := runHarness(t, f, 100, 30)
	h.key("q")
	h.has("firefox is still running. Stop it and quit?")
	h.key("n")
	if h.quit || slices.Contains(f.Calls(), "Stop()") {
		t.Fatal("n stopped or quit")
	}
	h.keys("q", "y")
	if !slices.Contains(f.Calls(), "Stop()") || h.quit {
		t.Fatalf("calls %q, quit %v", f.Calls(), h.quit)
	}
	h.has("Stopping firefox…")
	f.Run.AppRunning, f.Run.Exit = false, 143
	h.changed()
	if !h.quit || !h.m().Result().Started {
		t.Errorf("quit %v, started %v", h.quit, h.m().Result().Started)
	}
	t.Run("a second ctrl+c", func(t *testing.T) {
		h := runHarness(t, apptest.Running(), 100, 30)
		h.keys("ctrl+c", "y")
		if h.quit {
			t.Fatal("it quit before the application ended")
		}
		h.key("ctrl+c")
		if !h.quit {
			t.Error("a second ctrl+c did not quit")
		}
	})
}

func TestTheEndOfTheApplicationQuits(t *testing.T) {
	tests := []struct {
		name string
		// keys are pressed while the application still runs.
		keys []string
	}{
		{"on the Status tab", nil},
		{"on the Output tab", []string{"tab"}},
		{"on the Network tab", []string{"tab", "tab"}},
		{"on the help page", []string{"?"}},
		{"while the question about quitting is open", []string{"q"}},
		{"after the question was answered with no", []string{"q", "n"}},
		{"while the network is disconnected", []string{"tab", "tab", "x", "y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := apptest.Running()
			h := runHarness(t, f, 100, 30)
			h.keys(tt.keys...)
			if h.quit {
				t.Fatal("it quit while the application ran")
			}
			f.Run.AppRunning, f.Run.Exit = false, 2
			h.changed()
			if res := h.m().Result(); !h.quit || !res.Started || !res.Ended {
				t.Errorf("quit %v, result %+v", h.quit, res)
			}
			if slices.Contains(f.Calls(), "Stop()") {
				t.Errorf("an application that ended was stopped: %q", f.Calls())
			}
		})
	}
	t.Run("an application that ended at once", func(t *testing.T) {
		f := apptest.Running()
		f.Run.AppRunning, f.Run.Exit = false, 127
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-home")
		h.key("enter")
		if res := h.m().Result(); !h.quit || !res.Started || !res.Ended {
			t.Errorf("quit %v, result %+v", h.quit, res)
		}
		// The last frame says how it ended, and asks for no key.
		h.has("firefox ended with status 127.")
		if strings.Contains(h.screen(), "Press q") {
			t.Errorf("the last frame asks for a key:\n%s", h.screen())
		}
	})
	t.Run("the user stopped it: it did not end by itself", func(t *testing.T) {
		f := apptest.Running()
		h := runHarness(t, f, 100, 30)
		h.keys("q", "y")
		f.Run.AppRunning, f.Run.Exit = false, 143
		h.changed()
		if res := h.m().Result(); !h.quit || !res.Started || res.Ended {
			t.Errorf("quit %v, result %+v", h.quit, res)
		}
	})
}

func TestTheOutputIsSafe(t *testing.T) {
	f := apptest.Running()
	f.Out = append(f.Out, "bell\x07 escape\x1b[2J c1\u009b1m")
	f.Run.Events = append(f.Run.Events, app.Event{Time: apptest.SampleTime, Text: "evil\x1b]0;x\x07"})
	h := runHarness(t, f, 100, 30)
	for _, tab := range []string{"status", "output"} {
		raw := sgr.ReplaceAllString(h.m().View().Content, "")
		for _, c := range []byte(raw) {
			if (c < 0x20 && c != '\n') || c == 0x7f {
				t.Fatalf("the %s tab has the byte %#x", tab, c)
			}
		}
		if strings.ContainsRune(raw, '\u009b') {
			t.Fatalf("the %s tab has a C1 control", tab)
		}
		h.key("tab")
	}
}
