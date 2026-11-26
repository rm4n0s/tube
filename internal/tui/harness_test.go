// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
	"github.com/rm4n0s/tube/internal/core/clock"
)

// updateGoldens rewrites the golden files instead of comparing them.
var updateGoldens = flag.Bool("update-goldens", false, "rewrite the golden files")

// harness drives the model as the program would: a message, then every message that the
// returned commands produce.
type harness struct {
	t     *testing.T
	model tea.Model
	clock *clock.Fake
	quit  bool
	// across runs every command in a goroutine of its own, as the program does; seen are the
	// messages that the commands produced since it was set.
	across bool
	seen   []tea.Msg
}

// The folders of the harness: no test reads the folders of the machine through the add page,
// unless it sets the model's readDir itself.
const (
	sampleHome = "/home/ada"
	sampleCwd  = "/home/ada/Downloads"
)

// sampleDirs are the folders that the add page of the harness shows.
func sampleDirs() map[string][]dirEntry {
	return map[string][]dirEntry{
		"/":                           {{"home", true}, {"etc", true}},
		"/home":                       {{"ada", true}},
		"/home/ada":                   {{"Downloads", true}, {"Documents", true}, {".config", true}, {"notes.txt", false}},
		sampleCwd:                     {{"work.ovpn", false}, {"vpn", true}, {"Home.ovpn", false}, {"readme.txt", false}, {".old.ovpn", false}, {"archive", true}},
		"/etc":                        {},
		"/home/ada/Downloads/vpn":     {{"lab.ovpn", false}},
		"/home/ada/Downloads/archive": {},
	}
}

// newHarness makes a model over the app, on the fake clock of the sample, sizes it and reads
// its snapshot.
func newHarness(t *testing.T, a app.Application, width, height int) *harness {
	t.Helper()
	clk := clock.NewFake(apptest.SampleTime)
	m := NewModel(context.Background(), a, clk)
	m.home, m.cwd, m.readDir = sampleHome, sampleCwd, fakeDirs(sampleDirs())
	// No test reads the clipboard of the machine: there is no display and no program for it,
	// unless a test gives its own.
	m.getenv = func(string) string { return "" }
	m.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	m.readClip = func(context.Context, []string) string {
		t.Error("the program of the clipboard was run")
		return ""
	}
	m.writeClip = func(context.Context, []string, string) bool {
		t.Error("the program that takes the clipboard was run")
		return false
	}
	h := &harness{t: t, model: m, clock: clk}
	h.resize(width, height)
	h.run(h.model.Init())
	return h
}

// run runs a command and sends what it produces.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd != nil {
		h.send(h.exec(cmd))
	}
}

// exec runs a command and returns its message: in the goroutine of the test or, with across, in
// a goroutine of its own, as the program runs a command.
func (h *harness) exec(cmd tea.Cmd) tea.Msg {
	if !h.across {
		return cmd()
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	msg := <-out
	h.seen = append(h.seen, msg)
	return msg
}

// send sends a message, then every message that the commands it returns produce.
func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		msg, queue = queue[0], queue[1:]
		switch msg := msg.(type) {
		case nil:
			continue
		case tea.QuitMsg:
			h.quit = true
			continue
		case tea.BatchMsg:
			for _, cmd := range msg {
				if cmd != nil {
					queue = append(queue, h.exec(cmd))
				}
			}
			continue
		}
		var cmd tea.Cmd
		h.model, cmd = h.model.Update(msg)
		if cmd != nil {
			queue = append(queue, h.exec(cmd))
		}
	}
}

// keyMsg is the key press of a key's name.
func keyMsg(name string) tea.KeyPressMsg {
	special := map[string]tea.KeyPressMsg{
		"enter":     {Code: tea.KeyEnter},
		"esc":       {Code: tea.KeyEscape},
		"tab":       {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"space":     {Code: tea.KeySpace, Text: " "},
		"backspace": {Code: tea.KeyBackspace},
		"delete":    {Code: tea.KeyDelete},
		"up":        {Code: tea.KeyUp},
		"down":      {Code: tea.KeyDown},
		"left":      {Code: tea.KeyLeft},
		"right":     {Code: tea.KeyRight},
		"pgup":      {Code: tea.KeyPgUp},
		"pgdown":    {Code: tea.KeyPgDown},
		"home":      {Code: tea.KeyHome},
		"end":       {Code: tea.KeyEnd},
		"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
		"ctrl+u":    {Code: 'u', Mod: tea.ModCtrl},
	}
	if k, ok := special[name]; ok {
		return k
	}
	r := []rune(name)[0]
	if r >= 'A' && r <= 'Z' {
		return tea.KeyPressMsg{Code: r, Text: name, Mod: tea.ModShift}
	}
	return tea.KeyPressMsg{Code: r, Text: name}
}

// key presses one key, by its name.
func (h *harness) key(name string) {
	h.t.Helper()
	k := keyMsg(name)
	if got := k.String(); got != name {
		h.t.Fatalf("the key %q is named %q by Bubble Tea", name, got)
	}
	h.send(k)
}

// keys presses keys, one after the other.
func (h *harness) keys(names ...string) {
	h.t.Helper()
	for _, n := range names {
		h.key(n)
	}
}

// typeText types the text, one key press for each character.
func (h *harness) typeText(text string) {
	h.t.Helper()
	for _, r := range text {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// paste pastes the text.
func (h *harness) paste(text string) {
	h.t.Helper()
	h.send(tea.PasteMsg{Content: text})
}

// resize gives the terminal a size.
func (h *harness) resize(width, height int) {
	h.t.Helper()
	h.send(tea.WindowSizeMsg{Width: width, Height: height})
}

// find is the cell of the screen where the text begins; the test fails when it is not there.
func (h *harness) find(text string) (x, y int) {
	h.t.Helper()
	for y, line := range strings.Split(h.screen(), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return lipgloss.Width(line[:i]), y
		}
	}
	h.t.Fatalf("the screen has no %q to click on:\n%s", text, h.screen())
	return 0, 0
}

// click clicks the left button on a cell. The clock goes on by a second first, so that two
// clicks are never a double click.
func (h *harness) click(x, y int) {
	h.t.Helper()
	h.clock.Advance(time.Second)
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// clickOn clicks on the first cell of a text of the screen.
func (h *harness) clickOn(text string) {
	h.t.Helper()
	x, y := h.find(text)
	h.click(x, y)
}

// doubleClickOn clicks twice, a tenth of a second apart, on the first cell of a text.
func (h *harness) doubleClickOn(text string) {
	h.t.Helper()
	x, y := h.find(text)
	h.click(x, y)
	h.clock.Advance(100 * time.Millisecond)
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// rightClick clicks the right button on a cell.
func (h *harness) rightClick(x, y int) {
	h.t.Helper()
	h.clock.Advance(time.Second)
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight})
}

// withClipboard gives the model a display and a program that prints text as the clipboard.
func (h *harness) withClipboard(text string) {
	h.t.Helper()
	m := h.m()
	m.getenv = func(name string) string {
		if name == "WAYLAND_DISPLAY" {
			return "wayland-test"
		}
		return ""
	}
	m.lookPath = func(name string) (string, error) {
		if name == "wl-paste" {
			return "/usr/bin/wl-paste", nil
		}
		return "", exec.ErrNotFound
	}
	m.readClip = func(_ context.Context, argv []string) string {
		if argv[0] != "wl-paste" {
			h.t.Errorf("the program of the clipboard is %q", argv)
		}
		return text
	}
	h.model = m
}

// wheel turns the wheel by notches: down for a number above 0, up for one below.
func (h *harness) wheel(notches int) {
	h.t.Helper()
	button := tea.MouseWheelDown
	if notches < 0 {
		button, notches = tea.MouseWheelUp, -notches
	}
	for range notches {
		h.send(tea.MouseWheelMsg{X: 1, Y: 1, Button: button})
	}
}

// m is the model.
func (h *harness) m() Model { return h.model.(Model) }

// screen is the view's content without its styles.
func (h *harness) screen() string { return ansi.Strip(h.model.View().Content) }

// golden compares the screen with the golden file of the name, or rewrites it with
// -update-goldens.
func (h *harness) golden(name string) {
	h.t.Helper()
	path := filepath.Join("testdata", "goldens", name+".txt")
	got := h.screen() + "\n"
	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			h.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatalf("the golden file %s: %v (run with -update-goldens)", path, err)
	}
	if got != string(want) {
		h.t.Errorf("the screen differs from %s:\n--- got\n%s--- want\n%s", path, got, want)
	}
}

// has fails the test unless the screen has the text.
func (h *harness) has(text string) {
	h.t.Helper()
	if !strings.Contains(h.screen(), text) {
		h.t.Errorf("the screen has no %q:\n%s", text, h.screen())
	}
}

// selectedID is the id of the selected profile.
func (h *harness) selectedID() string {
	m := h.m()
	if len(m.snap.Profiles) == 0 {
		return ""
	}
	return m.snap.Profiles[m.list.selected()].ID
}
