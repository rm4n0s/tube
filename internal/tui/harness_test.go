// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	quit  bool
}

// newHarness makes a model over the app, on the fake clock of the sample, sizes it and reads
// its snapshot.
func newHarness(t *testing.T, a app.Application, width, height int) *harness {
	t.Helper()
	h := &harness{t: t, model: NewModel(context.Background(), a, clock.NewFake(apptest.SampleTime))}
	h.resize(width, height)
	h.run(h.model.Init())
	return h
}

// run runs a command and sends what it produces.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd != nil {
		h.send(cmd())
	}
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
					queue = append(queue, cmd())
				}
			}
			continue
		}
		var cmd tea.Cmd
		h.model, cmd = h.model.Update(msg)
		if cmd != nil {
			queue = append(queue, cmd())
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
