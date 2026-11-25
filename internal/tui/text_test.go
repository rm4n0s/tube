// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestFit(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"abc", 5, "abc  "},
		{"abcde", 5, "abcde"},
		{"abcdef", 5, "abcd…"},
		{"日本語", 5, "日本…"},
		{"日本語", 6, "日本語"},
		{"abc", 0, ""},
		{"abc", 1, "…"},
		{"", 2, "  "},
	}
	for _, tt := range tests {
		got := fit(tt.s, tt.width)
		if got != tt.want || (tt.width > 0 && lipgloss.Width(got) != tt.width) {
			t.Errorf("fit(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestClean(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"a\x1b[31mb", "a [31mb"},
		{"a\nb\tc\r", "a b c "},
		{"a\xffb", "a\ufffdb"},
		{"a\u0085b\u202ec", "a b c"},
	}
	for _, tt := range tests {
		if got := clean(tt.in); got != tt.want {
			t.Errorf("clean(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		then time.Time
		want string
	}{
		{time.Time{}, "never"},
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(time.Minute), "just now"},
		{now.Add(-5 * time.Minute), "5 min ago"},
		{now.Add(-90 * time.Minute), "1 hour ago"},
		{now.Add(-3 * time.Hour), "3 hours ago"},
		{now.Add(-30 * time.Hour), "1 day ago"},
		{now.Add(-2 * 24 * time.Hour), "2 days ago"},
		{now.Add(-29 * 24 * time.Hour), "29 days ago"},
		{now.Add(-23 * 24 * time.Hour * 1), "23 days ago"},
		{time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC), "2026-10-02"},
	}
	for _, tt := range tests {
		if got := ago(now, tt.then); got != tt.want {
			t.Errorf("ago(%v) = %q, want %q", tt.then, got, tt.want)
		}
	}
}

func TestWrap(t *testing.T) {
	got := wrap("one two three four", 9)
	want := []string{"one two", "three", "four"}
	if len(got) != len(want) {
		t.Fatalf("wrap = %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("wrap = %q, want %q", got, want)
		}
	}
	if got := wrap("", 10); got != nil {
		t.Errorf("wrap of nothing = %q", got)
	}
}
