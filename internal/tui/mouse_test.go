// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// called says whether the fake got a call that begins with prefix.
func called(f *apptest.Fake, prefix string) bool {
	return slices.ContainsFunc(f.Calls(), func(c string) bool { return strings.HasPrefix(c, prefix) })
}

func TestTheMouseIsOn(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	if got := h.model.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("the mouse mode is %v", got)
	}
}

func TestClickSelectsAndDoubleClickRuns(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.clickOn("work-backup")
	if got := h.selectedID(); got != "id-work-backup" {
		t.Errorf("a click selected %q", got)
	}
	// A second click, a second later, is not a double click: nothing runs.
	h.clickOn("backup.example.com")
	if len(f.Calls()) != 0 || h.m().page != pageProfiles {
		t.Fatalf("two slow clicks ran something: %q", f.Calls())
	}
	h.doubleClickOn("backup.example.com")
	if !called(f, "Connect(id-work-backup") {
		t.Errorf("a double click did not connect: %q", f.Calls())
	}
}

func TestADoubleClickNeedsTheSameLine(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	x, y := h.find("work-backup")
	h.click(x, y)
	h.clock.Advance(100 * time.Millisecond)
	h.send(tea.MouseClickMsg{X: x, Y: y - 1, Button: tea.MouseLeft})
	if len(f.Calls()) != 0 {
		t.Errorf("two clicks on two rows ran something: %q", f.Calls())
	}
	if got := h.selectedID(); got != "id-work" {
		t.Errorf("the second click selected %q", got)
	}
	// Three fast clicks are one double click and the first click of another.
	h = newHarness(t, f, 100, 30)
	h.doubleClickOn("Work")
	if h.m().page != pageLogin {
		t.Fatalf("a double click on a profile with a password opened the page %v", h.m().page)
	}
}

func TestOtherButtonsDoNothing(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	x, y := h.find("work-backup")
	for _, button := range []tea.MouseButton{tea.MouseRight, tea.MouseMiddle} {
		h.send(tea.MouseClickMsg{X: x, Y: y, Button: button})
	}
	h.send(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.send(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	if got := h.selectedID(); got != "id-home" {
		t.Errorf("the selected profile is %q", got)
	}
	// A click outside every line and every part does nothing either.
	h.click(99, 20)
	h.click(500, 500)
	if got := h.selectedID(); got != "id-home" {
		t.Errorf("the selected profile is %q", got)
	}
}

func TestAClickOnAKeyPressesIt(t *testing.T) {
	tests := []struct {
		name, on string
		check    func(t *testing.T, h *harness, f *apptest.Fake)
	}{
		{"add", "a add", func(t *testing.T, h *harness, _ *apptest.Fake) {
			if h.m().page != pageAdd {
				t.Errorf("the page is %v", h.m().page)
			}
		}},
		{"the word of a key", "add", func(t *testing.T, h *harness, _ *apptest.Fake) {
			if h.m().page != pageAdd {
				t.Errorf("the page is %v", h.m().page)
			}
		}},
		{"open", "e open", func(t *testing.T, h *harness, _ *apptest.Fake) {
			if h.m().page != pageProfile {
				t.Errorf("the page is %v", h.m().page)
			}
		}},
		{"delete", "d delete", func(t *testing.T, h *harness, f *apptest.Fake) {
			if h.m().ask == nil {
				t.Fatal("no question")
			}
			h.clickOn("[y] Delete")
			if !called(f, "DeleteProfile(id-home)") {
				t.Errorf("Calls = %q", f.Calls())
			}
		}},
		{"keys", "? keys", func(t *testing.T, h *harness, _ *apptest.Fake) {
			if h.m().page != pageHelp {
				t.Fatalf("the page is %v", h.m().page)
			}
			h.click(50, 20)
			if h.m().page != pageProfiles {
				t.Errorf("a click did not close the help page")
			}
		}},
		{"quit", "q quit", func(t *testing.T, h *harness, _ *apptest.Fake) {
			if !h.quit {
				t.Error("it did not quit")
			}
		}},
		{"run", "enter run", func(t *testing.T, _ *harness, f *apptest.Fake) {
			if !called(f, "Connect(id-home") {
				t.Errorf("Calls = %q", f.Calls())
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := apptest.Sample()
			h := newHarness(t, f, 100, 30)
			h.clickOn(tt.on)
			tt.check(t, h, f)
		})
	}
}

func TestClickKey(t *testing.T) {
	tests := []struct{ word, want string }{
		{"enter", "enter"}, {"pgdn", "pgdown"}, {"shift+tab", "shift+tab"}, {"a", "a"}, {"?", "?"},
		{"↑↓", ""}, {"jk", ""}, {"other", ""}, {"keys", ""}, {"", ""},
	}
	for _, tt := range tests {
		if got := clickKey(tt.word); got != tt.want {
			t.Errorf("clickKey(%q) = %q, want %q", tt.word, got, tt.want)
		}
	}
	// Every key that a click presses has the name that Bubble Tea gives it.
	for _, pb := range bindings(true) {
		for _, b := range pb.bindings {
			for _, word := range strings.Fields(b.keys) {
				if name := clickKey(word); name != "" && keyPress(name).String() != name {
					t.Errorf("the key %q of %s is pressed as %q", word, pb.title, keyPress(name).String())
				}
			}
		}
	}
}

func TestAQuestionCoversThePage(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.key("d")
	h.clickOn("work-backup")
	h.clickOn("Server")
	h.wheel(3)
	if h.m().ask == nil || h.selectedID() != "id-home" || h.m().page != pageProfiles {
		t.Fatalf("a click under a question did something: selected %q, page %v", h.selectedID(), h.m().page)
	}
	h.clickOn("[n] Cancel")
	if h.m().ask != nil || len(f.Calls()) != 0 {
		t.Errorf("Cancel: the question is %v, the calls %q", h.m().ask, f.Calls())
	}
}

func TestTheWheelScrollsTheList(t *testing.T) {
	h := newHarness(t, many(100), 100, 30)
	h.key("home")
	h.wheel(2)
	m := h.m()
	if m.list.top != 2*wheelStep || m.list.selected() != 2*wheelStep {
		t.Errorf("after two notches the first row is %d and the selected one %d", m.list.top, m.list.selected())
	}
	h.has("profile 006")
	if strings.Contains(h.screen(), "profile 005") {
		t.Error("the list did not scroll")
	}
	h.wheel(-1)
	m = h.m()
	if m.list.top != wheelStep || m.list.selected() != 2*wheelStep {
		t.Errorf("after a notch back the first row is %d and the selected one %d", m.list.top, m.list.selected())
	}
	h.wheel(100)
	m = h.m()
	if last := 100 - m.listHeight(); m.list.top != last || m.list.selected() != last {
		t.Errorf("at the end the first row is %d and the selected one %d", m.list.top, m.list.selected())
	}
	h.wheel(-100)
	if m = h.m(); m.list.top != 0 || m.list.selected() != m.listHeight()-1 {
		t.Errorf("at the start the first row is %d and the selected one %d", m.list.top, m.list.selected())
	}
	// A short list does not move.
	s := newHarness(t, apptest.Sample(), 100, 30)
	s.wheel(5)
	if s.m().list.top != 0 || s.selectedID() != "id-home" {
		t.Errorf("a short list scrolled")
	}
}

func TestTheTabsAreClicked(t *testing.T) {
	f := apptest.Running()
	h := runHarness(t, f, 100, 30)
	h.clickOn("Network")
	if h.m().run.tab != tabNetwork || h.selectedID() != "id-work" {
		t.Fatalf("the tab is %v, the selected profile %q", h.m().run.tab, h.selectedID())
	}
	h.doubleClickOn("home.example.net")
	if !called(f, "Switch(id-home") {
		t.Errorf("a double click did not switch: %q", f.Calls())
	}
	h.clickOn("x disconnect")
	if h.m().ask == nil {
		t.Fatal("no question")
	}
	h.clickOn("[y] Disconnect")
	if !called(f, "Disconnect()") {
		t.Errorf("Calls = %q", f.Calls())
	}
	h.clickOn("Output")
	if h.m().run.tab != tabOutput {
		t.Fatalf("the tab is %v", h.m().run.tab)
	}
	h.clickOn("Status")
	if h.m().run.tab != tabStatus {
		t.Fatalf("the tab is %v", h.m().run.tab)
	}
	h.clickOn("q quit")
	if h.m().ask == nil {
		t.Error("q did not ask")
	}
}

func TestTheWheelScrollsTheOutput(t *testing.T) {
	f := apptest.Running()
	h := runHarness(t, f, 100, 30)
	h.clickOn("Output")
	h.has("firefox: line 40 of the output")
	h.wheel(-1)
	h.has("scrolled back")
	h.has("firefox: line 37 of the output")
	if strings.Contains(h.screen(), "line 38 of") {
		t.Error("the output did not scroll by three lines")
	}
	// A click in the output goes nowhere: the application gets keys only.
	h.clickOn("firefox: line 30")
	if len(f.Typed()) != 0 {
		t.Errorf("a click was typed: %q", f.Typed())
	}
	h.wheel(1)
	h.has("keys go to firefox")
	h.wheel(-1)
	h.clickOn("end")
	h.has("keys go to firefox")
	h.clickOn("pgup")
	h.has("scrolled back")
}

func TestTheLoginPageIsClicked(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.key("enter")
	h.clickOn("[x] remember")
	if l := h.m().login; l.remember || l.focus != focusRemember {
		t.Errorf("remember is %v, the focus %d", l.remember, l.focus)
	}
	x, y := h.find("ada")
	h.click(x+1, y)
	if l := h.m().login; l.focus != focusUser || l.user.cursor != 1 {
		t.Errorf("the focus is %d, the cursor %d", l.focus, l.user.cursor)
	}
	// The password field shows a star for each character: a click on it takes the focus and
	// puts the cursor on the star that was clicked.
	h.click(x+5, y+1)
	h.typeText("secret")
	if l := h.m().login; l.focus != focusPassword || l.pass.value() != "secret" {
		t.Errorf("the focus is %d", l.focus)
	}
	h.click(x+2, y+1)
	if l := h.m().login; l.pass.cursor != 2 {
		t.Errorf("a click put the cursor of the password at %d", l.pass.cursor)
	}
	h.key("end")
	h.clickOn("[ Connect ]")
	if !called(f, "Connect(id-work, ada, ***, false)") {
		t.Errorf("Calls = %q", f.Calls())
	}
	c := newHarness(t, apptest.Sample(), 100, 30)
	c.selectProfile("id-work")
	c.key("enter")
	c.clickOn("[ Cancel ]")
	if c.m().page != pageProfiles {
		t.Errorf("Cancel went to the page %v", c.m().page)
	}
}

func TestAClickInAField(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		cursor   int
		width, x int
		secret   bool
		want     int
	}{
		{name: "on a character", text: "hello", cursor: 5, width: 10, x: 2, want: 2},
		{name: "after the text", text: "hello", cursor: 0, width: 10, x: 8, want: 5},
		{name: "on the first cell", text: "hello", cursor: 3, width: 10, x: 0, want: 0},
		{name: "a wide character", text: "東京x", cursor: 0, width: 10, x: 3, want: 1},
		{name: "a text that is scrolled", text: "0123456789abcdef", cursor: 16, width: 8, x: 0, want: 9},
		{name: "a secret", text: "hunter2", cursor: 7, width: 10, x: 2, secret: true, want: 2},
		{name: "a secret with wide characters", text: "東京x", cursor: 0, width: 10, x: 2, secret: true, want: 2},
		{name: "an empty field", text: "", cursor: 0, width: 10, x: 4, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := field{text: []rune(tt.text), cursor: tt.cursor, secret: tt.secret}
			f.click(tt.x, tt.width)
			if f.cursor != tt.want {
				t.Errorf("the cursor is at %d, want %d", f.cursor, tt.want)
			}
		})
	}
}

func TestTheFieldOfAProfileIsClicked(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.selectProfile("id-work")
	h.key("e")
	h.clickOn("r rename")
	if h.m().prof.open != renameField {
		t.Fatalf("the open field is %v", h.m().prof.open)
	}
	x, y := h.find("New name")
	h.click(x+1, y+1)
	h.typeText("-")
	h.clickOn("enter save")
	if !called(f, "RenameProfile(id-work, W-ork)") {
		t.Errorf("Calls = %q", f.Calls())
	}
	h.clickOn("esc back")
	if h.m().page != pageProfiles {
		t.Errorf("the page is %v", h.m().page)
	}
}

func TestTheStylesFollowTheTerminal(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	if m := h.m(); !m.dark || !m.colors || m.styles.selected.GetReverse() {
		t.Fatalf("the first styles are not for a dark terminal with colors")
	}
	h.send(tea.BackgroundColorMsg{Color: color.White})
	if h.m().dark {
		t.Error("a white background is taken for a dark one")
	}
	if got, dark := h.m().styles.title.GetForeground(), newStyles(true, true).title.GetForeground(); got == dark {
		t.Error("the colors of a light terminal are the ones of a dark one")
	}
	h.send(tea.BackgroundColorMsg{Color: color.Black})
	if !h.m().dark {
		t.Error("a black background is taken for a light one")
	}
	// The profiles of colorprofile, by their numbers: NoTTY (1) and ASCII (2) have no colors, so
	// what is selected is reversed; ANSI (3), ANSI256 (4) and TrueColor (5) have.
	profiles := []struct {
		msg    tea.ColorProfileMsg
		colors bool
	}{
		{tea.ColorProfileMsg{Profile: 1}, false}, {tea.ColorProfileMsg{Profile: 3}, true},
		{tea.ColorProfileMsg{Profile: 2}, false}, {tea.ColorProfileMsg{Profile: 5}, true},
		{tea.ColorProfileMsg{Profile: 4}, true},
	}
	for _, p := range profiles {
		h.send(p.msg)
		if m := h.m(); m.colors != p.colors || m.styles.selected.GetReverse() == p.colors {
			t.Errorf("with the profile %v: colors %v, reversed %v", p.msg.Profile, m.colors, m.styles.selected.GetReverse())
		}
	}
}

func TestLinesAndTheirParts(t *testing.T) {
	var got []string
	say := func(s string) action {
		return func(m Model, _ click) (Model, tea.Cmd) {
			got = append(got, s)
			return m, nil
		}
	}
	l := text("ab").button("cd", say("cd")).add("  ").append(text("x").button("東", say("wide"))).whole(say("line")).indent(2)
	tests := []struct {
		x    int
		want string
		at   int
	}{
		{0, "", 0}, {1, "", 0}, {2, "line", 0}, {4, "cd", 0}, {5, "cd", 1}, {6, "line", 4}, {9, "wide", 0}, {10, "wide", 1}, {11, "line", 9}, {40, "line", 38},
	}
	for _, tt := range tests {
		got = nil
		do, at := l.at(tt.x)
		if do != nil {
			do(Model{}, click{})
		}
		if (tt.want == "") != (do == nil) || (do != nil && (got[0] != tt.want || at != tt.at)) {
			t.Errorf("the cell %d does %q at %d, want %q at %d", tt.x, got, at, tt.want, tt.at)
		}
	}
	if do, _ := l.plain().at(4); do != nil {
		t.Error("a plain line can be clicked")
	}
	if l.w != 11 {
		t.Errorf("the line is %d cells wide", l.w)
	}
}
