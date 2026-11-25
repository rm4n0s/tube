// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

func TestTooSmall(t *testing.T) {
	for _, size := range [][2]int{{39, 10}, {40, 9}} {
		h := newHarness(t, apptest.Sample(), size[0], size[1])
		if got := h.screen(); got != tooSmall {
			t.Errorf("%dx%d shows %q", size[0], size[1], got)
		}
	}
	h := newHarness(t, apptest.Sample(), 40, 10)
	h.has("Tube")
}

func TestEveryLineFits(t *testing.T) {
	for _, w := range []int{40, 60, 100, 137} {
		for _, hgt := range []int{10, 30, 50} {
			for _, page := range []string{"profiles", "help", "add", "question"} {
				h := newHarness(t, apptest.Sample(), w, hgt)
				switch page {
				case "help":
					h.key("?")
				case "add":
					h.key("a")
					h.typeText(strings.Repeat("/a-long-folder-name", 10))
				case "question":
					h.key("d")
				}
				lines := strings.Split(h.model.View().Content, "\n")
				if len(lines) != hgt {
					t.Errorf("%dx%d %s: %d lines", w, hgt, page, len(lines))
				}
				for i, line := range lines {
					if lipgloss.Width(line) > w {
						t.Errorf("%dx%d %s: the line %d is %d cells wide", w, hgt, page, i, lipgloss.Width(line))
					}
				}
			}
		}
	}
}

func TestGoldenHelp(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	h.key("?")
	h.golden("help")
	h.key("x")
	if h.m().page != pageProfiles {
		t.Error("a key did not close the help page")
	}
}
