// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// reverseOn is how the selection is drawn: reversed.
const reverseOn = "\x1b[7m"

// copies is what the program that takes the clipboard of a test got.
type copies struct{ texts []string }

// withCopy gives the model a display and a program that takes the clipboard: what it gets is
// kept, and it works unless fail is set.
func (h *harness) withCopy(fail bool) *copies {
	h.t.Helper()
	c := &copies{}
	m := h.m()
	m.getenv = func(name string) string {
		if name == "WAYLAND_DISPLAY" {
			return "wayland-test"
		}
		return ""
	}
	m.lookPath = func(name string) (string, error) {
		if name == "wl-copy" {
			return "/usr/bin/wl-copy", nil
		}
		return "", exec.ErrNotFound
	}
	m.writeClip = func(_ context.Context, argv []string, text string) bool {
		if strings.Join(argv, " ") != "wl-copy --type text/plain" {
			h.t.Errorf("the program that takes the clipboard is %q", argv)
		}
		c.texts = append(c.texts, text)
		return !fail
	}
	h.model = m
	return c
}

// press, move and release are the left button pressed on a cell, moved to one while it is
// held, and let go.
func (h *harness) press(x, y int) {
	h.t.Helper()
	h.clock.Advance(time.Second)
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func (h *harness) move(x, y int) {
	h.t.Helper()
	h.send(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func (h *harness) release(x, y int) {
	h.t.Helper()
	h.send(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// drag presses on a cell, moves to another and lets go there.
func (h *harness) drag(x0, y0, x1, y1 int) {
	h.t.Helper()
	h.press(x0, y0)
	h.move(x1, y1)
	h.release(x1, y1)
}

// dragOver selects a text of the screen: from its first cell to its last.
func (h *harness) dragOver(text string) {
	h.t.Helper()
	x, y := h.find(text)
	h.drag(x, y, x+len([]rune(text))-1, y)
}

func TestSelectionSpan(t *testing.T) {
	down := selection{from: cell{5, 2}, to: cell{3, 4}}
	up := selection{from: cell{3, 4}, to: cell{5, 2}}
	one := selection{from: cell{7, 1}, to: cell{2, 1}}
	tests := []struct {
		name        string
		sel         selection
		y           int
		left, right int
		ok          bool
	}{
		{"above", down, 1, 0, 0, false},
		{"the first line: from the cell to the edge", down, 2, 5, 20, true},
		{"a line between: all of it", down, 3, 0, 20, true},
		{"the last line: up to the cell, with it", down, 4, 0, 4, true},
		{"below", down, 5, 0, 0, false},
		{"backwards is the same: the first line", up, 2, 5, 20, true},
		{"backwards is the same: the last line", up, 4, 0, 4, true},
		{"one line, backwards", one, 1, 2, 8, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left, right, ok := tt.sel.span(tt.y, 20)
			if ok != tt.ok || (ok && (left != tt.left || right != tt.right)) {
				t.Errorf("span = %d, %d, %v; want %d, %d, %v", left, right, ok, tt.left, tt.right, tt.ok)
			}
		})
	}
}

func TestCopyCommand(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		installed []string
		want      string
	}{
		{"wayland", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-copy", "xclip", "xsel"}, "wl-copy --type text/plain"},
		{"wayland without its program: the one of x11", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, []string{"xclip"}, "xclip -selection clipboard -in"},
		{"x11 with xsel only", map[string]string{"DISPLAY": ":0"}, []string{"xsel"}, "xsel --clipboard --input"},
		{"the program that prints the clipboard is of no use", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-paste"}, ""},
		{"no display", nil, []string{"wl-copy", "xclip", "xsel"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(name string) string { return tt.env[name] }
			lookPath := func(name string) (string, error) {
				if slices.Contains(tt.installed, name) {
					return "/usr/bin/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			if got := strings.Join(programOf(copyPrograms, getenv, lookPath), " "); got != tt.want {
				t.Errorf("the program is %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteClipboard(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	out := filepath.Join(t.TempDir(), "clipboard")
	tests := []struct {
		name    string
		argv    []string
		timeout time.Duration
		want    bool
	}{
		{"the program gets the text", []string{"sh", "-c", "cat > " + out}, time.Minute, true},
		{"a program that fails", []string{"sh", "-c", "cat > /dev/null; exit 1"}, time.Minute, false},
		{"a program that takes too long", []string{"sh", "-c", "sleep 30"}, 100 * time.Millisecond, false},
		{"a program that is not there", []string{"/nonexistent/tube-test-copy"}, time.Minute, false},
	}
	const text = "two lines\nof 東京"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()
			start := time.Now()
			if got := writeClipboard(ctx, tt.argv, text); got != tt.want {
				t.Errorf("writeClipboard = %v, want %v", got, tt.want)
			}
			if time.Since(start) > 10*time.Second {
				t.Errorf("writeClipboard took %v", time.Since(start))
			}
		})
	}
	if got, err := os.ReadFile(out); err != nil || string(got) != text {
		t.Errorf("the program got %q, %v", got, err)
	}
}

func TestSelectAndCopy(t *testing.T) {
	t.Run("a part of a line", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		h.dragOver("vpn1.example.com:1194")
		if !slices.Equal(c.texts, []string{"vpn1.example.com:1194"}) {
			t.Errorf("copied %q", c.texts)
		}
		h.has("Copied 21 characters.")
		if h.m().sel != nil || h.m().press != nil {
			t.Error("the selection stayed after the button was let go")
		}
	})
	t.Run("backwards", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		x, y := h.find("backup.example.com")
		h.drag(x+len("backup.example.com")-1, y, x, y)
		if !slices.Equal(c.texts, []string{"backup.example.com"}) {
			t.Errorf("copied %q", c.texts)
		}
	})
	t.Run("lines, without the spaces at their ends", func(t *testing.T) {
		h := runHarness(t, apptest.Running(), 100, 30)
		c := h.withCopy(false)
		x0, y0 := h.find("10.8.0.2/24")
		x1, y1 := h.find("1500")
		h.drag(x0, y0, x1+3, y1)
		if want := "10.8.0.2/24\nDNS servers   10.0.0.53\nMTU           1500"; len(c.texts) != 1 || c.texts[0] != want {
			t.Errorf("copied %q, want %q", c.texts, want)
		}
	})
	t.Run("the output of the application", func(t *testing.T) {
		h := runHarness(t, apptest.Running(), 100, 30)
		c := h.withCopy(false)
		h.key("tab")
		x, y := h.find("firefox: line 20 of the output")
		// To beyond the right edge: the lines end where their text ends.
		h.drag(x, y, 250, y+1)
		if want := "firefox: line 20 of the output\nfirefox: line 21 of the output"; len(c.texts) != 1 || c.texts[0] != want {
			t.Errorf("copied %q, want %q", c.texts, want)
		}
		if len(f(h).Typed()) != 0 {
			t.Errorf("the mouse went to the application: %q", f(h).Typed())
		}
	})
	t.Run("wide characters", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		x, y := h.find("東京-long")
		// The second cell of the first character, to the "l": a character is selected whole.
		h.drag(x+1, y, x+5, y)
		if len(c.texts) != 1 || !strings.HasSuffix(c.texts[0], "京-l") || strings.ContainsRune(c.texts[0], '�') {
			t.Errorf("copied %q", c.texts)
		}
	})
	t.Run("the selection is drawn while the button is held", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		h.withCopy(false)
		x, y := h.find("home.example.net")
		before := h.m().View().Content
		h.press(x, y)
		if h.m().View().Content != before {
			t.Error("a press alone changed the screen")
		}
		h.move(x+3, y)
		if raw := h.m().View().Content; !strings.Contains(raw, reverseOn+"home\x1b[") {
			t.Errorf("the selection is not drawn reversed:\n%q", strings.Split(raw, "\n")[y])
		}
		if got := h.screen(); got != ansi.Strip(before) {
			t.Error("the selection changed the text of the screen")
		}
		h.release(x+3, y)
		if strings.Contains(h.m().View().Content, reverseOn+"home\x1b[") {
			t.Error("the selection is still drawn after the button was let go")
		}
	})
	t.Run("a click copies nothing", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		x, y := h.find("Work")
		h.press(x, y)
		h.release(x, y)
		// Away and back to the cell of the press is a click too.
		h.press(x, y)
		h.move(x+4, y+1)
		h.move(x, y)
		h.release(x, y)
		if len(c.texts) != 0 {
			t.Errorf("copied %q", c.texts)
		}
		if h.selectedID() != "id-work" {
			t.Errorf("the click did not select the row: %s", h.selectedID())
		}
	})
	t.Run("a press still does what a click does", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		h.dragOver("work-backup")
		if h.selectedID() != "id-work-backup" || !slices.Equal(c.texts, []string{"work-backup"}) {
			t.Errorf("selected %s, copied %q", h.selectedID(), c.texts)
		}
	})
	t.Run("empty cells copy nothing", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		h.drag(10, 15, 40, 18)
		if len(c.texts) != 0 || strings.Contains(h.screen(), "Copied") {
			t.Errorf("copied %q", c.texts)
		}
	})
	t.Run("a key ends the selection", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		x, y := h.find("home.example.net")
		h.press(x, y)
		h.move(x+8, y)
		h.key("down")
		h.release(x+8, y)
		if len(c.texts) != 0 || h.m().sel != nil {
			t.Errorf("copied %q", c.texts)
		}
	})
	t.Run("a move without a press selects nothing", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		h.move(10, 4)
		h.release(20, 4)
		if len(c.texts) != 0 || h.m().sel != nil {
			t.Errorf("copied %q", c.texts)
		}
	})
	t.Run("the text of a question", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		c := h.withCopy(false)
		h.selectProfile("id-home")
		h.key("d")
		h.dragOver("Delete")
		if !slices.Equal(c.texts, []string{"Delete"}) || h.m().ask == nil {
			t.Errorf("copied %q, the question is open: %v", c.texts, h.m().ask != nil)
		}
	})
	t.Run("a terminal that is too small", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 30, 8)
		c := h.withCopy(false)
		h.drag(0, 0, 20, 0)
		if len(c.texts) != 0 {
			t.Errorf("copied %q", c.texts)
		}
	})
}

// f is the fake application of a harness.
func f(h *harness) *apptest.Fake { return h.m().app.(*apptest.Fake) }

func TestCopyNeverHasASecret(t *testing.T) {
	h := loginHarness(t, apptest.Sample(), 100, 30)
	c := h.withCopy(false)
	h.typeText("hunter2")
	// The whole screen, from its first cell to its last.
	h.drag(0, 0, 99, 29)
	if len(c.texts) != 1 {
		t.Fatalf("copied %q", c.texts)
	}
	if strings.Contains(c.texts[0], "hunter2") || !strings.Contains(c.texts[0], "*******") || !strings.Contains(c.texts[0], "ada") {
		t.Errorf("the copy of the login page is not the stars and the user name:\n%s", c.texts[0])
	}
}

func TestCopyWithoutAProgram(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	// No display and no program: the terminal is asked (OSC 52), and cannot say whether it did.
	x, y := h.find("home.example.net")
	h.press(x, y)
	h.move(x+len("home.example.net")-1, y)
	m, cmd := h.model.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.model = m
	if cmd == nil {
		t.Fatal("the terminal was not asked")
	}
	if got, want := cmd(), tea.SetClipboard("home.example.net")(); got != want {
		t.Errorf("the terminal was asked with %#v, want %#v", got, want)
	}
	h.has("Copied 16 characters, if your terminal allows it; else hold shift and select with the terminal.")
}

func TestCopyThatFails(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	c := h.withCopy(true)
	h.dragOver("Home")
	if len(c.texts) != 1 {
		t.Fatalf("copied %q", c.texts)
	}
	h.has("wl-copy could not copy the text. Hold shift and select with your terminal.")
	if !h.m().isError {
		t.Error("the failure is not drawn as an error")
	}
	t.Run("one character", func(t *testing.T) {
		h := newHarness(t, apptest.Sample(), 100, 30)
		h.withCopy(false)
		x, y := h.find("Home")
		h.drag(x, y+1, x+1, y)
		h.drag(x, y, x+1, y)
		h.has("Copied 2 characters.")
		if got := copied(1); got != "Copied 1 character" {
			t.Errorf("copied(1) = %q", got)
		}
	})
}

func TestTheMenuClosesForASelection(t *testing.T) {
	h := loginHarness(t, apptest.Sample(), 100, 30)
	h.withClipboard("pasted")
	h.rightClickPassword()
	if h.m().menu == nil {
		t.Fatal("the menu did not open")
	}
	x, y := h.find("ada")
	h.press(x, y)
	h.move(x+2, y)
	if h.m().menu != nil {
		t.Error("the menu stayed open under a selection")
	}
}
