// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// open opens the page of the profile with the id, as e on its row does.
func (h *harness) open(id string) {
	h.t.Helper()
	for range len(h.m().snap.Profiles) {
		if h.selectedID() == id {
			h.key("e")
			return
		}
		h.key("down")
	}
	h.key("home")
	for range len(h.m().snap.Profiles) {
		if h.selectedID() == id {
			h.key("e")
			return
		}
		h.key("down")
	}
	h.t.Fatalf("no profile %s", id)
}

// withEditor gives the model an editor that is found, and an execEditor of the test.
func (h *harness) withEditor(run func(argv []string, id, path string) tea.Cmd) {
	m := h.m()
	m.getenv = func(name string) string {
		if name == "EDITOR" {
			return "my-editor --wait"
		}
		return ""
	}
	m.lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	m.execEditor = run
	h.model = m
}

func TestGoldenProfilePage(t *testing.T) {
	many := apptest.Sample()
	for i := range 12 {
		many.Snap.Profiles[3].Remotes = append(many.Snap.Profiles[3].Remotes, "vpn"+strconv.Itoa(i)+".example.com:1194 udp")
	}
	tests := []struct {
		name string
		f    *apptest.Fake
		id   string
	}{
		{"profile-work", apptest.Sample(), "id-work"},
		{"profile-nothing-set", apptest.Sample(), "id-work-backup"},
		{"profile-broken", apptest.Sample(), "0123456789abcdef"},
		{"profile-many-servers", many, "id-work-backup"},
	}
	for _, tt := range tests {
		for _, size := range [][2]int{{100, 30}, {60, 16}} {
			name := tt.name + "-" + strconv.Itoa(size[0])
			t.Run(name, func(t *testing.T) {
				h := newHarness(t, tt.f, size[0], size[1])
				h.open(tt.id)
				h.golden(name)
			})
		}
	}
}

func TestABrokenProfileTakesOnlyDeleteAndBack(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.open("0123456789abcdef")
	h.keys("r", "u", "n", "t")
	if len(f.Calls()) != 0 || h.m().prof.open != noField || strings.Contains(h.screen(), "rename") {
		t.Errorf("calls %q, field %v", f.Calls(), h.m().prof.open)
	}
	h.keys("d", "y")
	if got := f.Calls(); !slices.Equal(got, []string{"DeleteProfile(0123456789abcdef)"}) {
		t.Errorf("Calls = %q", got)
	}
	if h.m().page != pageProfiles {
		t.Error("the page stayed open after the delete")
	}
	h.open("id-home")
	h.key("esc")
	if h.m().page != pageProfiles {
		t.Error("esc did not go back")
	}
}

// fieldTests are the fields of the page: the key, the text it starts with, and the call.
var fieldTests = []struct {
	name, key, filled, call string
}{
	{"rename", "r", "Work", "RenameProfile(id-work, Work2)"},
	{"username", "u", "ada", "SetUsername(id-work, ada2)"},
	{"dns", "n", "10.0.0.53", "SetDNS(id-work, 10.0.0.532)"},
}

func TestRename(t *testing.T)   { testField(t, fieldTests[0]) }
func TestUsername(t *testing.T) { testField(t, fieldTests[1]) }
func TestDNS(t *testing.T)      { testField(t, fieldTests[2]) }

func testField(t *testing.T, tt struct{ name, key, filled, call string }) {
	t.Run("saved", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.open("id-work")
		h.key(tt.key)
		if got := h.m().prof.field.value(); got != tt.filled {
			t.Errorf("the field starts with %q", got)
		}
		h.typeText("2")
		h.key("enter")
		if got := f.Calls(); !slices.Equal(got, []string{tt.call}) {
			t.Errorf("Calls = %q", got)
		}
		if h.m().prof.open != noField {
			t.Error("the field stayed open")
		}
		h.has(saved)
	})
	t.Run("esc", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.open("id-work")
		h.keys(tt.key, "x", "esc")
		if len(f.Calls()) != 0 || h.m().prof.open != noField || h.m().page != pageProfile {
			t.Errorf("esc: calls %q, field %v, page %v", f.Calls(), h.m().prof.open, h.m().page)
		}
	})
	t.Run("an error", func(t *testing.T) {
		f := apptest.Sample()
		method, _, _ := strings.Cut(tt.call, "(")
		f.Fail = map[string]error{method: errors.New("ProfileNameInvalid", "the name is not allowed")}
		h := newHarness(t, f, 100, 30)
		h.open("id-work")
		h.key(tt.key)
		h.typeText("2")
		h.key("enter")
		h.has("Error: the name is not allowed (ProfileNameInvalid)")
		if h.m().prof.open == noField || h.m().prof.field.value() != tt.filled+"2" {
			t.Error("the field did not stay open with its text")
		}
	})
}

func TestFindEditor(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		found  []string
		want   []string
		wantOK bool
	}{
		{"visual", map[string]string{"VISUAL": "code -w", "EDITOR": "vim"}, []string{"code", "vim", "nano"}, []string{"code", "-w"}, true},
		{"visual not found", map[string]string{"VISUAL": "code -w", "EDITOR": "vim"}, []string{"vim", "nano"}, []string{"vim"}, true},
		{"neither set", nil, []string{"nano", "vi"}, []string{"nano"}, true},
		{"no nano", nil, []string{"vi"}, []string{"vi"}, true},
		{"nothing found", map[string]string{"EDITOR": "vim"}, nil, nil, false},
		{"an empty variable", map[string]string{"VISUAL": "  ", "EDITOR": "vim"}, []string{"vim"}, []string{"vim"}, true},
		{"no shell", map[string]string{"EDITOR": "vim; rm -rf x"}, []string{"vim;"}, []string{"vim;", "rm", "-rf", "x"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(name string) string { return tt.env[name] }
			lookPath := func(name string) (string, error) {
				if slices.Contains(tt.found, name) {
					return "/usr/bin/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			got, ok := findEditor(getenv, lookPath)
			if ok != tt.wantOK || !slices.Equal(got, tt.want) {
				t.Errorf("findEditor = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// doneAt once is an execEditor that ends at once, with the error.
func doneAtOnce(editorErr error, argvs *[][]string) func(argv []string, id, path string) tea.Cmd {
	return func(argv []string, id, path string) tea.Cmd {
		*argvs = append(*argvs, argv)
		return func() tea.Msg { return editorDoneMsg{id: id, path: path, err: editorErr} }
	}
}

func TestEditFlow(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	var argvs [][]string
	h.withEditor(doneAtOnce(nil, &argvs))
	h.open("id-work")
	h.key("t")
	if got := f.Calls(); !slices.Equal(got, []string{"BeginEdit(id-work)", "EndEdit(id-work, /edit/id-work.ovpn)"}) {
		t.Errorf("Calls = %q", got)
	}
	if len(argvs) != 1 || !slices.Equal(argvs[0], []string{"my-editor", "--wait"}) {
		t.Errorf("the editor was %q", argvs)
	}
	h.has(saved)
	t.Run("the editor fails", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.withEditor(doneAtOnce(exec.ErrNotFound, &argvs))
		h.open("id-work")
		h.key("t")
		if got := f.Calls(); len(got) != 2 || !strings.HasPrefix(got[1], "EndEdit(") {
			t.Errorf("Calls = %q", got)
		}
	})
	t.Run("BeginEdit fails", func(t *testing.T) {
		f := apptest.Sample()
		f.Fail = map[string]error{"BeginEdit": errors.New("ProfileNotFound", "no such profile")}
		h := newHarness(t, f, 100, 30)
		var argvs [][]string
		h.withEditor(doneAtOnce(nil, &argvs))
		h.open("id-work")
		h.key("t")
		if len(argvs) != 0 || len(f.Calls()) != 1 {
			t.Errorf("the editor ran: %q, %q", argvs, f.Calls())
		}
		h.has("Error: no such profile (ProfileNotFound)")
	})
}

func TestEditWithoutAnEditor(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	m := h.m()
	m.getenv = func(string) string { return "" }
	m.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	h.model = m
	h.open("id-work")
	h.key("t")
	if len(f.Calls()) != 0 {
		t.Errorf("Calls = %q", f.Calls())
	}
	h.has(noEditor)
}

// scriptEditor is an execEditor that runs a shell script of the test on the file, in place of
// giving the terminal to an editor.
func scriptEditor(t *testing.T, script string) func(argv []string, id, path string) tea.Cmd {
	t.Helper()
	file := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(file, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return func(argv []string, id, path string) tea.Cmd {
		return func() tea.Msg {
			err := exec.Command(file, path).Run()
			return editorDoneMsg{id: id, path: path, err: err}
		}
	}
}

// storedText is the profile's text in the store, read through an edit that changes nothing.
func storedText(t *testing.T, a *app.App, id string) []byte {
	t.Helper()
	ctx := context.Background()
	path, err := a.BeginEdit(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.EndEdit(ctx, id, path); err != nil {
		t.Fatal(err)
	}
	return text
}

func TestEditReal(t *testing.T) {
	tests := []struct {
		name, script, message string
		changed               bool
	}{
		{"a comment", `printf '# edited by the test\n' >> "$1"`, saved, true},
		{"garbage", `printf 'comp-lzo\n' >> "$1"`, notChangedBy, false},
		{"nothing", `true`, nothingNew, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := realApp(t)
			id, err := a.AddProfile(context.Background(), fixtureProfile(t, t.TempDir(), "Lab.ovpn"), "", "", false)
			if err != nil {
				t.Fatal(err)
			}
			before := storedText(t, a, id)
			h := newHarness(t, a, 100, 30)
			h.withEditor(scriptEditor(t, tt.script))
			h.open(id)
			h.key("t")
			h.has(tt.message)
			after := storedText(t, a, id)
			if changed := !bytes.Equal(before, after); changed != tt.changed {
				t.Errorf("the text changed: %v", changed)
			}
			if tt.changed && !bytes.HasSuffix(after, []byte("# edited by the test\n")) {
				t.Errorf("the stored text ends with %q", after[max(len(after)-40, 0):])
			}
		})
	}
}

func TestAProfileThatGoesAway(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.open("id-work")
	f.Snap.Profiles = slices.DeleteFunc(slices.Clone(f.Snap.Profiles), func(p app.Profile) bool { return p.ID == "id-work" })
	h.send(changedMsg{})
	if h.m().page != pageProfiles {
		t.Error("the page of a profile that is gone stayed open")
	}
}

func TestPasteIntoAField(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.open("id-work")
	h.key("n")
	h.key("ctrl+u")
	h.paste("1.1.1.1 9.9.9.9")
	h.key("enter")
	if got := f.Calls(); !slices.Equal(got, []string{"SetDNS(id-work, 1.1.1.1 9.9.9.9)"}) {
		t.Errorf("Calls = %q", got)
	}
}
