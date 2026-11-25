// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

func TestGoldenProfiles(t *testing.T) {
	noTarget := apptest.Sample()
	noTarget.Snap.Target = nil
	tests := []struct {
		name          string
		fake          *apptest.Fake
		width, height int
	}{
		{"sample-100x30", apptest.Sample(), 100, 30},
		{"sample-60x16", apptest.Sample(), 60, 16},
		{"sample-40x10", apptest.Sample(), 40, 10},
		{"empty-100x30", apptest.Empty(), 100, 30},
		{"no-target-100x30", noTarget, 100, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.fake, tt.width, tt.height)
			h.golden("profiles-" + tt.name)
			if tt.name == "sample-100x30" && h.selectedID() != "id-home" {
				t.Errorf("the selected profile is %q, want the latest used", h.selectedID())
			}
		})
	}
}

func TestMoving(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	steps := []struct{ key, want string }{
		{"down", "id-work"}, {"j", "id-work-backup"}, {"end", "id-long"}, {"home", "0123456789abcdef"},
		{"pgdown", "id-long"}, {"up", "id-work-backup"}, {"k", "id-work"},
	}
	for _, s := range steps {
		h.key(s.key)
		if got := h.selectedID(); got != s.want {
			t.Errorf("after %s the selected profile is %q, want %q", s.key, got, s.want)
		}
	}
}

// many is a fake with n profiles.
func many(n int) *apptest.Fake {
	f := apptest.Empty()
	for i := range n {
		f.Snap.Profiles = append(f.Snap.Profiles, app.Profile{ID: fmt.Sprintf("id-%03d", i), Name: fmt.Sprintf("profile %03d", i)})
	}
	return f
}

func TestALongList(t *testing.T) {
	h := newHarness(t, many(100), 100, 20)
	check := func() {
		t.Helper()
		if !strings.Contains(h.screen(), "> profile "+strings.TrimPrefix(h.selectedID(), "id-")) {
			t.Fatalf("the selected row %s is not on the screen:\n%s", h.selectedID(), h.screen())
		}
	}
	for range 99 {
		h.key("down")
		check()
	}
	for range 99 {
		h.key("up")
		check()
	}
	h.keys("end")
	check()
	h.keys("pgup", "pgup")
	check()
}

func TestChangedReadsTheSnapshotAgain(t *testing.T) {
	f := apptest.Sample()
	h := newHarness(t, f, 100, 30)
	h.key("down") // Work
	f.Snap.Profiles = append([]app.Profile{{ID: "id-aaa", Name: "Aaa"}}, f.Snap.Profiles...)
	h.send(changedMsg{})
	h.has("Aaa")
	if got := h.selectedID(); got != "id-work" {
		t.Errorf("the selection moved to %q", got)
	}
}

func TestASnapshotThatFails(t *testing.T) {
	f := apptest.Sample()
	f.Fail = map[string]error{"Snapshot": errors.New("DiskGone", "the disk is gone")}
	h := newHarness(t, f, 100, 30)
	h.has("Error: the disk is gone (DiskGone)")
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		h := newHarness(t, apptest.Sample(), 100, 30)
		h.key(key)
		if !h.quit || h.m().Result().Run {
			t.Errorf("%s: quit %v, run %v", key, h.quit, h.m().Result().Run)
		}
	}
}

func TestKeysOfLaterSteps(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	h.key("a")
	h.has(notBuilt)
	h.key("down")
	if strings.Contains(h.screen(), notBuilt) {
		t.Error("a key did not clear the message line")
	}
	// Text that is typed or pasted on the list does nothing.
	before := h.selectedID()
	h.typeText("zz")
	h.paste("pasted")
	if h.selectedID() != before || strings.Contains(h.screen(), "pasted") {
		t.Error("typed or pasted text changed the list")
	}
}

func TestHostileNames(t *testing.T) {
	f := apptest.Sample()
	f.Snap.Profiles[1].Name = "evil�[2J�]0;x"
	f.Snap.Now = f.Snap.Now.Add(time.Minute)
	h := newHarness(t, f, 100, 30)
	h.has("evil�[2J")
	for _, c := range []byte(h.screen()) {
		if c < 0x20 && c != '\n' {
			t.Fatalf("the screen has the byte %#x", c)
		}
	}
}
