// SPDX-License-Identifier: GPL-2.0-only

package tui

import "testing"

func TestListKeys(t *testing.T) {
	tests := []struct {
		start int
		key   string
		want  int
		used  bool
	}{
		{0, "up", 0, true}, {0, "k", 0, true}, {0, "down", 1, true}, {0, "j", 1, true},
		{5, "up", 4, true}, {5, "down", 6, true}, {5, "pgup", 2, true}, {5, "pgdown", 8, true},
		{9, "down", 9, true}, {9, "pgdown", 9, true}, {1, "pgup", 0, true},
		{5, "home", 0, true}, {5, "end", 9, true}, {5, "x", 5, false},
	}
	for _, tt := range tests {
		l := list{}
		l.setLen(10)
		l.window(3)
		l.selectIndex(tt.start)
		used := l.key(tt.key)
		if l.selected() != tt.want || used != tt.used {
			t.Errorf("%s from %d: %d (used %v), want %d", tt.key, tt.start, l.selected(), used, tt.want)
		}
	}
}

func TestListSetLen(t *testing.T) {
	l := list{}
	l.setLen(10)
	l.selectIndex(8)
	l.setLen(3)
	if l.selected() != 2 {
		t.Errorf("the selection is %d", l.selected())
	}
	l.setLen(0)
	if l.selected() != 0 || l.key("down") && l.selected() != 0 {
		t.Errorf("an empty list selects %d", l.selected())
	}
	if first, last := l.window(5); first != 0 || last != 0 {
		t.Errorf("the window of an empty list is %d..%d", first, last)
	}
}

func TestListWindow(t *testing.T) {
	l := list{}
	l.setLen(100)
	for _, sel := range []int{0, 5, 50, 99, 42, 0} {
		l.selectIndex(sel)
		first, last := l.window(10)
		if sel < first || sel >= last || last-first != 10 {
			t.Errorf("the selection %d is not in the window %d..%d", sel, first, last)
		}
	}
	l.setLen(4)
	if first, last := l.window(10); first != 0 || last != 4 {
		t.Errorf("a short list's window is %d..%d", first, last)
	}
}
