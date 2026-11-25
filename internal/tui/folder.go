// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rm4n0s/errors"
)

// The list of a folder on the add page (docs/SPEC.md §13.3).
const (
	// maxFolderRead is the number of names that are read of one folder, at most, and
	// maxFolderEntries the number that the list keeps.
	maxFolderRead    = 10000
	maxFolderEntries = 2000
	// minFolderBox is the smallest box of the list: its two edges and two rows.
	minFolderBox = 4
	// parentName is the row of the folder above.
	parentName    = ".."
	profileSuffix = ".ovpn"
	crumbGap      = " › "
	folderReading = "Reading…"
	folderFailed  = "This folder cannot be read."
	folderNothing = "No folders and no .ovpn files here."
	folderEmpty   = "This folder is empty."
)

// entry is a name in a folder.
type entry struct {
	name string
	dir  bool
}

// folder is the state of the list of a folder.
type folder struct {
	// dir is the folder that is shown, an absolute path; want is the one that is being read, or
	// "".
	dir, want string
	entries   []entry
	// more says that the folder has more names than the list keeps; failed that it cannot be
	// read.
	more, failed bool
	// all shows the hidden names and the files that are not .ovpn too.
	all  bool
	list list
}

// folderMsg is a folder that was read.
type folderMsg struct {
	dir          string
	entries      []entry
	more, failed bool
	// selectName is the name to select, or "".
	selectName string
	// fill says that the folder goes into the File field: the user opened it in the list.
	fill bool
}

// defaultReadDir reads the names of a folder, at most maxFolderRead of them.
func defaultReadDir(dir string) ([]os.DirEntry, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	entries, err := f.ReadDir(maxFolderRead)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return entries, err
}

// listFolder reads the folder dir for the list: the folder above first, then the folders, then
// the files, each sorted by name without case. Without all, a hidden name and a file that is not
// an .ovpn file are left out. A name that is not safe to draw is always left out: it could not
// be written in the File field. ok is false for a folder that cannot be read.
func listFolder(dir string, all bool, readDir func(string) ([]os.DirEntry, error)) (entries []entry, more, ok bool) {
	if dir != "/" {
		entries = append(entries, entry{name: parentName, dir: true})
	}
	read, err := readDir(dir)
	if err != nil {
		return entries, false, false
	}
	var found []entry
	for _, e := range read {
		name := e.Name()
		if name == "" || name != clean(name) || (!all && strings.HasPrefix(name, ".")) {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			// A link to a folder is a folder; a link that leads nowhere is listed as a file.
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = info.IsDir()
			}
		}
		if !isDir && !all && !strings.HasSuffix(strings.ToLower(name), profileSuffix) {
			continue
		}
		found = append(found, entry{name: name, dir: isDir})
	}
	sort.Slice(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.dir != b.dir {
			return a.dir
		}
		if la, lb := strings.ToLower(a.name), strings.ToLower(b.name); la != lb {
			return la < lb
		}
		return a.name < b.name
	})
	if room := maxFolderEntries - len(entries); len(found) > room {
		found, more = found[:room], true
	}
	return append(entries, found...), more, true
}

// readFolder is the command that reads a folder for the list.
func (m Model) readFolder(dir string, all bool, selectName string, fill bool) tea.Cmd {
	readDir := m.readDir
	return func() tea.Msg {
		entries, more, ok := listFolder(dir, all, readDir)
		return folderMsg{dir: dir, entries: entries, more: more, failed: !ok, selectName: selectName, fill: fill}
	}
}

// folderOf is the folder that a path in the File field is in, as an absolute path, and the name
// after it: "~/vpn/wo" is the folder vpn of the home folder, and "wo".
func (m Model) folderOf(input string) (dir, name string) {
	if input == "~" {
		input = "~/"
	}
	if i := strings.LastIndex(input, "/"); i >= 0 {
		dir, name = input[:i+1], input[i+1:]
	} else {
		name = input
	}
	switch rest, inHome := strings.CutPrefix(dir, "~/"); {
	case inHome && m.home != "":
		dir = filepath.Join(m.home, rest)
	case !strings.HasPrefix(dir, "/"):
		dir = filepath.Join(m.cwd, dir)
	}
	return filepath.Clean(dir), name
}

// showPath is an absolute path as the File field writes it: with "~" for the home folder.
func (m Model) showPath(path string) string {
	if m.home != "" && m.home != "/" {
		if path == m.home {
			return "~"
		}
		if rest, ok := strings.CutPrefix(path, strings.TrimSuffix(m.home, "/")+"/"); ok {
			return "~/" + rest
		}
	}
	return path
}

// showFolder is a folder as the File field writes it: with a "/" at its end.
func (m Model) showFolder(dir string) string {
	return strings.TrimSuffix(m.showPath(dir), "/") + "/"
}

// crumbs are the folders on the way to dir, from the home folder ("~") when dir is in it and
// from the root when not: what each is called, and its path.
func (m Model) crumbs(dir string) (names, paths []string) {
	shown := m.showPath(dir)
	base := "/"
	names, paths = []string{"/"}, []string{"/"}
	if shown == "~" || strings.HasPrefix(shown, "~/") {
		base = m.home
		names, paths = []string{"~"}, []string{m.home}
		shown = strings.TrimPrefix(shown, "~")
	}
	for _, part := range strings.Split(shown, "/") {
		if part == "" {
			continue
		}
		base = filepath.Join(base, part)
		names, paths = append(names, part), append(paths, base)
	}
	return names, paths
}

// browse opens the folder dir in the list; its path goes into the File field when it was read.
// selectName is the name to select there, or "".
func (m Model) browse(dir, selectName string) (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	p.br.want = dir
	return m, m.readFolder(dir, p.br.all, selectName, true)
}

// follow shows the folder of the path in the File field, and selects the name that the path
// begins: the list follows what is typed.
func (m Model) follow() (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	b := &p.br
	dir, name := m.folderOf(p.path.value())
	switch {
	case dir == b.want:
		return m, nil
	case dir == b.dir && b.want == "":
		b.selectPrefix(name)
		return m, nil
	}
	b.want = dir
	return m, m.readFolder(dir, b.all, "", false)
}

// folderRead takes a folder that was read, when it is the one that is waited for.
func (m Model) folderRead(msg folderMsg) Model {
	if m.add == nil || msg.dir != m.add.br.want {
		return m
	}
	p := *m.add
	m.add = &p
	b := &p.br
	b.want, b.dir, b.entries, b.more, b.failed = "", msg.dir, msg.entries, msg.more, msg.failed
	b.list = list{}
	b.list.setLen(len(b.entries))
	if len(b.entries) > 1 && b.entries[0].name == parentName {
		// The first name of the folder, not the folder above.
		b.list.selectIndex(1)
	}
	if msg.fill {
		p.path.set(m.showFolder(msg.dir))
	}
	for i, e := range b.entries {
		if msg.selectName != "" && e.name == msg.selectName {
			b.list.selectIndex(i)
		}
	}
	if dir, name := m.folderOf(p.path.value()); msg.selectName == "" && dir == b.dir {
		b.selectPrefix(name)
	}
	return m
}

// selectPrefix selects the first name of the folder that begins with prefix, with the same case
// when there is one. An empty prefix selects nothing.
func (b *folder) selectPrefix(prefix string) {
	if prefix == "" {
		return
	}
	lower := strings.ToLower(prefix)
	other := -1
	for i, e := range b.entries {
		switch {
		case e.name == parentName:
		case strings.HasPrefix(e.name, prefix):
			b.list.selectIndex(i)
			return
		case other < 0 && strings.HasPrefix(strings.ToLower(e.name), lower):
			other = i
		}
	}
	if other >= 0 {
		b.list.selectIndex(other)
	}
}

// openEntry opens the row i of the list: a folder is shown, and a file goes into the File field.
// toName then gives the focus to the Name field: the file was chosen with enter or a double
// click.
func (m Model) openEntry(i int, toName bool) (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	b := &p.br
	if i < 0 || i >= len(b.entries) {
		return m, nil
	}
	e := b.entries[i]
	switch {
	case e.name == parentName:
		return m.folderUp()
	case e.dir:
		return m.browse(filepath.Join(b.dir, e.name), "")
	}
	p.path.set(m.showPath(filepath.Join(b.dir, e.name)))
	if toName {
		p.focus = addFocusName
	}
	return m, nil
}

// folderUp opens the folder above, with the folder that was shown selected.
func (m Model) folderUp() (Model, tea.Cmd) {
	dir := m.add.br.dir
	if dir == "/" || dir == "" {
		return m, nil
	}
	return m.browse(filepath.Dir(dir), filepath.Base(dir))
}

// toggleAll shows, or hides, the hidden names and the files that are not .ovpn.
func (m Model) toggleAll() (Model, tea.Cmd) {
	p := *m.add
	m.add = &p
	b := &p.br
	b.all = !b.all
	selected := ""
	if i := b.list.selected(); i < len(b.entries) {
		selected = b.entries[i].name
	}
	b.want = b.dir
	return m, m.readFolder(b.dir, b.all, selected, false)
}

// folderKey handles a key on the list of the folder. Text that is typed goes to the File field,
// which the list follows: a "/" or a "~" starts a new path there.
func (m Model) folderKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	p := m.add
	b := &p.br
	switch name := msg.String(); name {
	case "enter":
		return m.openEntry(b.list.selected(), true)
	case "right":
		if i := b.list.selected(); i < len(b.entries) && b.entries[i].dir {
			return m.openEntry(i, false)
		}
	case "backspace", "left":
		return m.folderUp()
	case "tab":
		p.focus = m.addStop(p.focus, 1)
	case "up", "down", "pgup", "pgdown", "home", "end":
		b.list.key(name)
	default:
		if msg.Text == "" {
			return m, nil
		}
		p.focus = addFocusPath
		if msg.Text == "/" || msg.Text == "~" {
			p.path.clear()
		}
		p.path.key(msg)
		return m.follow()
	}
	return m, nil
}

// clickEntry is the click on the row i of the list: it selects the row and, for a file, writes
// it in the File field. A double click opens the row.
func (m Model) clickEntry(i int) action {
	return func(m Model, c click) (Model, tea.Cmd) {
		p := *m.add
		m.add = &p
		p.err, p.focus = "", addFocusFolder
		p.br.list.selectIndex(i)
		if c.double {
			return m.openEntry(i, true)
		}
		if e := p.br.entries[i]; !e.dir {
			p.path.set(m.showPath(filepath.Join(p.br.dir, e.name)))
		}
		return m, nil
	}
}

// clickCrumb is the click on a folder of the path above the list: it opens that folder.
func (m Model) clickCrumb(dir string) action {
	return func(m Model, _ click) (Model, tea.Cmd) {
		p := *m.add
		m.add = &p
		p.err, p.focus = "", addFocusFolder
		return m.browse(dir, "")
	}
}

// folderBox draws the list of the folder in a box of width cells and height lines: the path on
// the upper edge, each of its folders a button; the rows; and on the lower edge, which rows of
// how many are shown, when not all are.
func (m Model) folderBox(width, height int) []line {
	p := m.add
	b := p.br
	st := m.styles
	focused := p.focus == addFocusFolder
	edge := st.border.Render
	if focused {
		edge = st.borderOn.Render
	}
	inner := width - 4
	lines := make([]line, 0, height)
	lines = append(lines, m.folderTop(width, edge))
	lst := b.list
	rows := height - 2
	first, last := lst.window(rows)
	for i := first; i < last; i++ {
		e := b.entries[i]
		selected := i == b.list.selected()
		name := e.name
		if e.dir {
			name += "/"
		}
		row := fit(selectedOrNot(selected)+name, inner)
		switch {
		case selected && focused:
			row = st.selected.Render(row)
		case selected:
			row = st.selectedOff.Render(row)
		case e.dir:
			row = st.folder.Render(row)
		case strings.HasSuffix(strings.ToLower(e.name), profileSuffix):
			row = st.profile.Render(row)
		}
		lines = append(lines, text(edge("│")+" ").append(text(row).whole(m.clickEntry(i))).add(" "+edge("│")))
	}
	note := ""
	switch real := len(b.entries) > 1 || (len(b.entries) == 1 && b.entries[0].name != parentName); {
	case b.dir == "" || (b.want != "" && !real):
		note = st.dim.Render(folderReading)
	case b.failed:
		note = st.err.Render(folderFailed)
	case !real && b.all:
		note = st.dim.Render(folderEmpty)
	case !real:
		note = st.dim.Render(folderNothing)
	}
	for len(lines) < height-1 {
		lines = append(lines, text(edge("│")+" "+fit("  "+note, inner)+" "+edge("│")))
		note = ""
	}
	position := ""
	if n := len(b.entries); n > rows || b.more {
		position = " " + strconv.Itoa(first+1) + "-" + strconv.Itoa(last) + " of "
		if b.more {
			position += "the first "
		}
		position += strconv.Itoa(n) + " "
	}
	if len(position) > width-4 {
		position = ""
	}
	bottom := edge("╰"+strings.Repeat("─", width-3-len(position))) + st.dim.Render(position) + edge("─╯")
	return append(lines, text(bottom))
}

// folderTop is the upper edge of the box: the path of the folder, each folder on the way a
// button that opens it. When the path is too long, its first folders are left out.
func (m Model) folderTop(width int, edge func(...string) string) line {
	st := m.styles
	var names, paths []string
	if m.add.br.dir != "" {
		names, paths = m.crumbs(m.add.br.dir)
	}
	room := width - 6
	path := func(cut bool) line {
		var l line
		if cut {
			l = l.add(st.dim.Render(ellipsis))
		}
		for i, name := range names {
			if i > 0 || cut {
				l = l.add(st.dim.Render(crumbGap))
			}
			// The path is the working folder's, or one that was typed: its names are not
			// trusted to be safe to draw.
			name = clean(name)
			if len(names) == 1 && l.w+lipgloss.Width(name) > room {
				name = strings.TrimRight(fit(name, max(room-l.w, 1)), " ")
			}
			l = l.button(st.title.Render(name), m.clickCrumb(paths[i]))
		}
		return l
	}
	l := path(false)
	for len(names) > 1 && l.w > room {
		names, paths = names[1:], paths[1:]
		l = path(true)
	}
	return text(edge("╭─") + " ").append(l).add(" " + edge(strings.Repeat("─", max(width-5-l.w, 0))+"╮"))
}
