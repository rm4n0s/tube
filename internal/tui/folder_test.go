// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// names are the names of the entries, folders with a "/".
func names(entries []entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.name
		if e.dir {
			out[i] += "/"
		}
	}
	return out
}

func TestListFolder(t *testing.T) {
	dirs := sampleDirs()
	dirs["/hostile"] = []dirEntry{{"a\x1bb.ovpn", false}, {"ok.ovpn", false}, {"B.OVPN", false}, {"a.ovpn", false}, {"Zeta", true}, {"alpha", true}, {"", false}}
	var big []dirEntry
	for i := range maxFolderEntries + 10 {
		big = append(big, dirEntry{fmt.Sprintf("p%05d.ovpn", i), false})
	}
	dirs["/big"] = big
	readDir := fakeDirs(dirs)
	tests := []struct {
		name, dir string
		all       bool
		want      []string
		more, ok  bool
	}{
		{"folders first, then the profiles", sampleCwd, false, []string{"../", "archive/", "vpn/", "Home.ovpn", "work.ovpn"}, false, true},
		{"all files", sampleCwd, true, []string{"../", "archive/", "vpn/", ".old.ovpn", "Home.ovpn", "readme.txt", "work.ovpn"}, false, true},
		{"the root has no folder above", "/", false, []string{"etc/", "home/"}, false, true},
		{"a hidden folder", sampleHome, true, []string{"../", ".config/", "Documents/", "Downloads/", "notes.txt"}, false, true},
		{"an empty folder", "/etc", false, []string{"../"}, false, true},
		{"a folder that cannot be read", "/root", false, []string{"../"}, false, false},
		{"a name that is not safe, and the case", "/hostile", false, []string{"../", "alpha/", "Zeta/", "a.ovpn", "B.OVPN", "ok.ovpn"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, more, ok := listFolder(tt.dir, tt.all, readDir)
			if got := names(entries); !slices.Equal(got, tt.want) || more != tt.more || ok != tt.ok {
				t.Errorf("listFolder(%q, %v) = %q, %v, %v; want %q, %v, %v", tt.dir, tt.all, got, more, ok, tt.want, tt.more, tt.ok)
			}
		})
	}
	t.Run("more names than the list keeps", func(t *testing.T) {
		entries, more, ok := listFolder("/big", false, readDir)
		if len(entries) != maxFolderEntries || !more || !ok {
			t.Errorf("listFolder kept %d names, more %v, ok %v", len(entries), more, ok)
		}
	})
}

func TestFolderOf(t *testing.T) {
	m := Model{home: sampleHome, cwd: sampleCwd}
	tests := []struct{ input, dir, name string }{
		{"", sampleCwd, ""},
		{"wo", sampleCwd, "wo"},
		{"vpn/la", sampleCwd + "/vpn", "la"},
		{"~", sampleHome, ""},
		{"~/", sampleHome, ""},
		{"~/vpn/wo", sampleHome + "/vpn", "wo"},
		{"/etc/", "/etc", ""},
		{"/etc", "/", "etc"},
		{"/", "/", ""},
		{"../x", sampleHome, "x"},
		{"~user/x", sampleCwd + "/~user", "x"},
	}
	for _, tt := range tests {
		if dir, name := m.folderOf(tt.input); dir != tt.dir || name != tt.name {
			t.Errorf("folderOf(%q) = %q, %q; want %q, %q", tt.input, dir, name, tt.dir, tt.name)
		}
	}
	// Without a home folder, "~/" is a folder of the working folder.
	if dir, _ := (Model{cwd: "/w"}).folderOf("~/x"); dir != "/w/~" {
		t.Errorf("without a home folder, the folder of ~/x is %q", dir)
	}
}

func TestShowPathAndCrumbs(t *testing.T) {
	m := Model{home: sampleHome, cwd: sampleCwd}
	tests := []struct {
		path, shown, folder string
		crumbs, paths       []string
	}{
		{sampleHome, "~", "~/", []string{"~"}, []string{sampleHome}},
		{sampleCwd + "/vpn", "~/Downloads/vpn", "~/Downloads/vpn/", []string{"~", "Downloads", "vpn"}, []string{sampleHome, sampleCwd, sampleCwd + "/vpn"}},
		{"/", "/", "/", []string{"/"}, []string{"/"}},
		{"/etc/openvpn", "/etc/openvpn", "/etc/openvpn/", []string{"/", "etc", "openvpn"}, []string{"/", "/etc", "/etc/openvpn"}},
		{"/home/adam", "/home/adam", "/home/adam/", []string{"/", "home", "adam"}, []string{"/", "/home", "/home/adam"}},
	}
	for _, tt := range tests {
		if got := m.showPath(tt.path); got != tt.shown {
			t.Errorf("showPath(%q) = %q, want %q", tt.path, got, tt.shown)
		}
		if got := m.showFolder(tt.path); got != tt.folder {
			t.Errorf("showFolder(%q) = %q, want %q", tt.path, got, tt.folder)
		}
		if crumbs, paths := m.crumbs(tt.path); !slices.Equal(crumbs, tt.crumbs) || !slices.Equal(paths, tt.paths) {
			t.Errorf("crumbs(%q) = %q, %q; want %q, %q", tt.path, crumbs, paths, tt.crumbs, tt.paths)
		}
	}
	if got := (Model{}).showPath("/home/ada/x"); got != "/home/ada/x" {
		t.Errorf("without a home folder, showPath = %q", got)
	}
}

// sampleIn is the sample whose last add was in dir: the add page starts there.
func sampleIn(dir string) *apptest.Fake {
	f := apptest.Sample()
	f.Snap.AddFolder = dir
	return f
}

// folderState is the folder of the add page, the selected name, the File field and the focus.
func (h *harness) folderState() (dir, selected, path string, focus int) {
	p := h.m().add
	if i := p.br.list.selected(); i < len(p.br.entries) {
		selected = p.br.entries[i].name
	}
	return p.br.dir, selected, p.path.value(), p.focus
}

// wantFolder fails the test unless the add page shows the folder, with the name selected, the
// File field and the focus.
func (h *harness) wantFolder(dir, selected, path string, focus int) {
	h.t.Helper()
	if d, s, p, f := h.folderState(); d != dir || s != selected || p != path || f != focus {
		h.t.Errorf("the folder is %q with %q selected, the File %q, the focus %d; want %q, %q, %q, %d", d, s, p, f, dir, selected, path, focus)
	}
}

func TestBrowseWithTheKeys(t *testing.T) {
	f := sampleIn(sampleCwd)
	h := newHarness(t, f, 100, 30)
	h.key("a")
	h.wantFolder(sampleCwd, "archive", "", addFocusFolder)
	h.keys("down", "enter")
	h.wantFolder(sampleCwd+"/vpn", "lab.ovpn", "~/Downloads/vpn/", addFocusFolder)
	h.key("backspace")
	h.wantFolder(sampleCwd, "vpn", "~/Downloads/", addFocusFolder)
	h.keys("right", "left", "left")
	h.wantFolder(sampleHome, "Downloads", "~/", addFocusFolder)
	h.keys("up", "up", "enter")
	h.wantFolder("/home", "ada", "/home/", addFocusFolder)
	h.keys("home", "enter")
	h.wantFolder("/", "home", "/", addFocusFolder)
	h.keys("backspace", "left")
	h.wantFolder("/", "home", "/", addFocusFolder)
	h.keys("enter", "enter", "down", "enter", "pgdown")
	h.wantFolder(sampleCwd, "work.ovpn", "~/Downloads/", addFocusFolder)
	// The right arrow opens folders only; enter on a file chooses it and goes to the Name.
	h.key("right")
	h.wantFolder(sampleCwd, "work.ovpn", "~/Downloads/", addFocusFolder)
	h.key("enter")
	h.wantFolder(sampleCwd, "work.ovpn", "~/Downloads/work.ovpn", addFocusName)
	h.has("Name   work")
	h.typeText("Office")
	h.key("enter")
	if got := f.Calls(); !slices.Equal(got, []string{"AddProfile(~/Downloads/work.ovpn, Office, false)"}) {
		t.Errorf("Calls = %q", got)
	}
	h.has("Added Office.")
}

func TestWhereTheAddPageStarts(t *testing.T) {
	gone := "/home/ada/gone"
	tests := []struct {
		name, remembered, home, cwd string
		wantDir, wantSelected       string
	}{
		{"the home folder", "", sampleHome, sampleCwd, sampleHome, "Documents"},
		{"the folder of the last add", sampleCwd + "/vpn", sampleHome, sampleCwd, sampleCwd + "/vpn", "lab.ovpn"},
		{"a remembered folder that is gone: the home folder", gone, sampleHome, sampleCwd, sampleHome, "Documents"},
		{"no home folder: the working folder", "", "", sampleCwd, sampleCwd, "archive"},
		{"a home folder that cannot be read is shown as it is", "", "/root", sampleCwd, "/root", ".."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, sampleIn(tt.remembered), 100, 30)
			m := h.m()
			m.home, m.cwd = tt.home, tt.cwd
			h.model = m
			h.key("a")
			h.wantFolder(tt.wantDir, tt.wantSelected, "", addFocusFolder)
		})
	}
	// The snapshot that comes after an add has the folder of that add: the next add starts
	// there, without a new start of Tube.
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.key("a")
	h.wantFolder(sampleHome, "Documents", "", addFocusFolder)
	h.keys("down", "enter", "end", "enter", "enter")
	if got := f.Calls(); !slices.Equal(got, []string{"AddProfile(~/Downloads/work.ovpn, , false)"}) {
		t.Fatalf("Calls = %q", got)
	}
	f.Snap.AddFolder = sampleCwd
	f.Emit()
	h.changed()
	h.key("a")
	h.wantFolder(sampleCwd, "archive", "", addFocusFolder)
}

func TestBrowseWithTheMouse(t *testing.T) {
	f := sampleIn(sampleCwd)
	h := newHarness(t, f, 100, 30)
	h.clickOn("a add")
	h.clickOn("vpn/")
	h.wantFolder(sampleCwd, "vpn", "", addFocusFolder)
	h.doubleClickOn("vpn/")
	h.wantFolder(sampleCwd+"/vpn", "lab.ovpn", "~/Downloads/vpn/", addFocusFolder)
	h.clickOn("../")
	h.wantFolder(sampleCwd+"/vpn", "..", "~/Downloads/vpn/", addFocusFolder)
	h.doubleClickOn("../")
	h.wantFolder(sampleCwd, "vpn", "~/Downloads/", addFocusFolder)
	h.clickOn("~")
	h.wantFolder(sampleHome, "Documents", "~/", addFocusFolder)
	h.doubleClickOn("Downloads/")
	h.clickOn("Home.ovpn")
	h.wantFolder(sampleCwd, "Home.ovpn", "~/Downloads/Home.ovpn", addFocusFolder)
	h.doubleClickOn("work.ovpn")
	h.wantFolder(sampleCwd, "work.ovpn", "~/Downloads/work.ovpn", addFocusName)
	x, y := h.find("~/Downloads/work.ovpn")
	h.click(x+2, y)
	if p := h.m().add; p.focus != addFocusPath || p.path.cursor != 2 {
		t.Errorf("a click in the File field: the focus is %d, the cursor %d", p.focus, p.path.cursor)
	}
	h.click(x+50, y+1)
	if p := h.m().add; p.focus != addFocusName {
		t.Errorf("a click in the Name field: the focus is %d", p.focus)
	}
	h.clickOn("[ Add ]")
	if got := f.Calls(); !slices.Equal(got, []string{"AddProfile(~/Downloads/work.ovpn, , false)"}) {
		t.Errorf("Calls = %q", got)
	}
	h.has("Added work.")
	h.key("a")
	h.clickOn("[ Cancel ]")
	if h.m().page != pageProfiles || h.m().add != nil {
		t.Errorf("Cancel left the page %v", h.m().page)
	}
}

func TestTheWheelScrollsTheFolder(t *testing.T) {
	dirs := sampleDirs()
	for i := range 60 {
		dirs[sampleCwd] = append(dirs[sampleCwd], dirEntry{fmt.Sprintf("p%02d.ovpn", i), false})
	}
	h := newHarness(t, sampleIn(sampleCwd), 100, 30)
	m := h.m()
	m.readDir = fakeDirs(dirs)
	h.model = m
	h.key("a")
	h.has(" 1-19 of 65 ")
	h.wheel(2)
	h.has(" 7-25 of 65 ")
	if p := h.m().add; p.br.list.top != 6 || p.br.list.selected() != 6 {
		t.Errorf("the first row is %d, the selected one %d", p.br.list.top, p.br.list.selected())
	}
	h.wheel(-1)
	h.has(" 4-22 of 65 ")
	h.key("end")
	h.has(" 47-65 of 65 ")
}

func TestTheListFollowsTheFile(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	h.key("a")
	h.typeText("/et")
	h.wantFolder("/", "etc", "/et", addFocusPath)
	h.typeText("c/")
	h.wantFolder("/etc", "..", "/etc/", addFocusPath)
	h.has(folderNothing)
	h.key("ctrl+u")
	h.wantFolder(sampleCwd, "archive", "", addFocusPath)
	h.typeText("h")
	h.wantFolder(sampleCwd, "Home.ovpn", "h", addFocusPath)
	h.typeText("x")
	h.wantFolder(sampleCwd, "Home.ovpn", "hx", addFocusPath)
	h.keys("ctrl+u")
	h.typeText("~/Do")
	h.wantFolder(sampleHome, "Documents", "~/Do", addFocusPath)
	h.typeText("w")
	h.wantFolder(sampleHome, "Downloads", "~/Dow", addFocusPath)
	h.key("tab")
	h.wantFolder(sampleCwd, "archive", "~/Downloads/", addFocusPath)
	// A pasted path is followed too, and a folder that cannot be read says so.
	h.keys("up")
	h.paste("/root/secret.ovpn")
	h.wantFolder("/root", "..", "/root/secret.ovpn", addFocusPath)
	h.has(folderFailed)
	h.paste("x")
	h.wantFolder("/root", "..", "/root/secret.ovpnx", addFocusPath)
	// In the list, text goes to the File field: "~" and "/" start a new path.
	h.keys("up")
	h.typeText("~")
	h.wantFolder(sampleHome, "Documents", "~", addFocusPath)
	h.keys("up")
	h.typeText("D")
	h.wantFolder(sampleCwd, "archive", "~D", addFocusPath)
}

func TestAllFiles(t *testing.T) {
	h := newHarness(t, sampleIn(sampleCwd), 100, 30)
	h.key("a")
	h.keys("down", "down")
	if strings.Contains(h.screen(), "readme.txt") {
		t.Fatal("a file that is not .ovpn is listed")
	}
	h.clickOn("[ ] all files")
	h.has("[x] all files")
	h.has("readme.txt")
	h.has(".old.ovpn")
	h.wantFolder(sampleCwd, "Home.ovpn", "", addFocusAll)
	h.key("space")
	h.has("[ ] all files")
	if strings.Contains(h.screen(), "readme.txt") {
		t.Error("the file is still listed")
	}
	h.wantFolder(sampleCwd, "Home.ovpn", "", addFocusAll)
	// An empty folder says what it is.
	h.keys("tab", "home", "down", "enter")
	h.wantFolder(sampleCwd+"/archive", "..", "~/Downloads/archive/", addFocusFolder)
	h.has(folderNothing)
	h.keys("shift+tab", "space")
	h.has(folderEmpty)
}

func TestTheFocusOfTheAddPage(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
		want          int
	}{
		{"it starts on the list", 100, 30, nil, addFocusFolder},
		{"tab leaves the list", 100, 30, []string{"tab"}, addFocusPath},
		{"down goes to the name", 100, 30, []string{"tab", "down"}, addFocusName},
		{"tab goes on", 100, 30, []string{"tab", "down", "tab"}, addFocusAll},
		{"and around", 100, 30, []string{"tab", "down", "tab", "tab"}, addFocusFolder},
		{"down too", 100, 30, []string{"tab", "down", "down", "down"}, addFocusFolder},
		{"up goes back", 100, 30, []string{"tab", "down", "down", "up", "up", "up"}, addFocusFolder},
		{"shift+tab goes back", 100, 30, []string{"shift+tab", "shift+tab"}, addFocusName},
		{"tab completes in the File field", 100, 30, []string{"tab", "tab"}, addFocusPath},
		{"no list: it starts on the File", 40, 10, nil, addFocusPath},
		{"no list: two fields", 40, 10, []string{"down"}, addFocusName},
		{"no list: around", 40, 10, []string{"down", "down"}, addFocusPath},
		{"no list: shift+tab", 40, 10, []string{"shift+tab"}, addFocusName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, apptest.Sample(), tt.width, tt.height)
			h.key("a")
			h.keys(tt.keys...)
			if got := h.m().add.focus; got != tt.want {
				t.Errorf("the focus is %d, want %d", got, tt.want)
			}
		})
	}
	t.Run("a terminal that gets too short for the list", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.key("a")
		h.resize(100, 10)
		if strings.Contains(h.screen(), "╭") {
			t.Fatalf("the list is drawn in 10 lines:\n%s", h.screen())
		}
		h.typeText("/tmp/x.ovpn")
		h.key("enter")
		if got := f.Calls(); !slices.Equal(got, []string{"AddProfile(/tmp/x.ovpn, , false)"}) {
			t.Errorf("Calls = %q", got)
		}
	})
}

func TestTheAddPageIsBusy(t *testing.T) {
	f := sampleIn(sampleCwd)
	f.Block = map[string]chan struct{}{"AddProfile": make(chan struct{})}
	h := newHarness(t, f, 100, 30)
	h.key("a")
	h.keys("end")
	m := h.m()
	m, _ = m.addSubmit()
	h.model = m
	h.has(adding)
	h.clickOn("vpn/")
	h.clickOn("[ Add ]")
	h.clickOn("[ ] all files")
	h.paste("/x")
	h.typeText("y")
	h.wantFolder(sampleCwd, "work.ovpn", "", addFocusFolder)
	if h.m().add.br.all || !h.m().add.busy {
		t.Error("the page changed while the add ran")
	}
}

func TestALongFolder(t *testing.T) {
	deep := sampleHome + strings.Repeat("/a-folder-with-a-long-name", 8) + "/東京"
	for _, width := range []int{40, 60, 100} {
		h := newHarness(t, sampleIn(deep), width, 30)
		m := h.m()
		m.readDir = fakeDirs(map[string][]dirEntry{deep: {{"x.ovpn", false}}})
		h.model = m
		h.key("a")
		top := strings.Split(h.screen(), "\n")[2]
		if lipgloss.Width(top) != width || !strings.Contains(top, "… › ") || !strings.Contains(top, "東京 ") || !strings.HasSuffix(strings.TrimRight(top, " "), "─╮") {
			t.Errorf("at %d columns the upper edge is %q", width, top)
		}
		// A click on the last folder of the path that is shown opens it.
		h.clickOn("東京")
		h.wantFolder(deep, "x.ovpn", strings.Replace(deep, sampleHome, "~", 1)+"/", addFocusFolder)
	}
	// One name that is longer than the edge is cut.
	h := newHarness(t, apptest.Sample(), 40, 30)
	m := h.m()
	m.home, m.cwd = "", "/"+strings.Repeat("x", 80)
	h.model = m
	h.key("a")
	if top := strings.Split(h.screen(), "\n")[2]; lipgloss.Width(top) != 40 || !strings.Contains(top, "x… ") {
		t.Errorf("the upper edge is %q", top)
	}
}

func TestAFolderWithAnUnsafeName(t *testing.T) {
	evil := "/srv/\x1b[31mred\x07/vpn"
	h := newHarness(t, apptest.Sample(), 100, 30)
	m := h.m()
	m.home, m.cwd = "", evil
	m.readDir = fakeDirs(map[string][]dirEntry{evil: {{"x.ovpn", false}, {"\x1b]0;title\x07.ovpn", false}, {"bad\u202eevil", true}}})
	h.model = m
	h.key("a")
	raw := h.model.View().Content
	for _, bad := range []string{"\x1b[31mred", "\x07", "\x1b]0;", "\u202e"} {
		if strings.Contains(raw, bad) {
			t.Errorf("the screen has %q", bad)
		}
	}
	h.has("srv ›  [31mred  › vpn")
	if got, want := names(h.m().add.br.entries), []string{"../", "x.ovpn"}; !slices.Equal(got, want) {
		t.Errorf("the folder lists %q, want %q", got, want)
	}
}

func TestBrowseReal(t *testing.T) {
	a, _ := realApp(t)
	root := t.TempDir()
	sub := filepath.Join(root, "profiles")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	fixtureProfile(t, sub, "Lab.ovpn")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sub, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "broken.ovpn")); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, a, 100, 30)
	m := h.m()
	m.home, m.cwd, m.readDir = "", root, defaultReadDir
	h.model = m
	h.key("a")
	if got, want := names(h.m().add.br.entries), []string{"../", "link/", "profiles/", "broken.ovpn"}; !slices.Equal(got, want) {
		t.Fatalf("the folder has %q, want %q", got, want)
	}
	// Through the link to the folder, to the profile in it.
	h.doubleClickOn("link/")
	h.doubleClickOn("Lab.ovpn")
	h.clickOn("[ Add ]")
	h.has("Added Lab.")
	snap, err := a.Snapshot(context.Background())
	if err != nil || len(snap.Profiles) != 1 || snap.Profiles[0].Name != "Lab" {
		t.Fatalf("the snapshot is %+v, %v", snap, err)
	}
}

func TestDefaultReadDir(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b", "a"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := defaultReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Errorf("defaultReadDir = %d names, %v", len(entries), err)
	}
	if entries, err := defaultReadDir(filepath.Join(dir, "empty-or-missing")); err == nil {
		t.Errorf("a missing folder was read: %v", entries)
	}
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if entries, err := defaultReadDir(filepath.Join(dir, "empty")); err != nil || len(entries) != 0 {
		t.Errorf("an empty folder: %d names, %v", len(entries), err)
	}
}
