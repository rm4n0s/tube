// SPDX-License-Identifier: GPL-2.0-only

package tui

// list is the state of a list on the screen: how many rows, the selected one, and the first
// visible one.
type list struct {
	n, sel, top int
	// page is how many rows pgup and pgdown move; the last window's height.
	page int
}

// setLen sets the number of rows and keeps the selection inside.
func (l *list) setLen(n int) {
	l.n = max(n, 0)
	l.sel = min(l.sel, max(l.n-1, 0))
	l.top = min(l.top, l.sel)
}

// selected is the selected row; 0 for an empty list.
func (l *list) selected() int { return l.sel }

// selectIndex selects the row i, kept inside the list.
func (l *list) selectIndex(i int) {
	l.sel = min(max(i, 0), max(l.n-1, 0))
}

// key moves the selection for up/k, down/j, pgup, pgdown, home and end, and says whether it used
// the key.
func (l *list) key(name string) bool {
	page := max(l.page, 1)
	switch name {
	case "up", "k":
		l.selectIndex(l.sel - 1)
	case "down", "j":
		l.selectIndex(l.sel + 1)
	case "pgup":
		l.selectIndex(l.sel - page)
	case "pgdown":
		l.selectIndex(l.sel + page)
	case "home":
		l.selectIndex(0)
	case "end":
		l.selectIndex(l.n - 1)
	default:
		return false
	}
	return true
}

// window returns the rows to draw in height lines, first included and last not, moved so that
// the selected row is visible.
func (l *list) window(height int) (first, last int) {
	height = max(height, 1)
	l.page = height
	if l.sel < l.top {
		l.top = l.sel
	}
	if l.sel >= l.top+height {
		l.top = l.sel - height + 1
	}
	l.top = max(min(l.top, l.n-height), 0)
	return l.top, min(l.top+height, l.n)
}
