// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

func TestClipboardCommand(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		installed []string
		want      string
	}{
		{"wayland", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-paste", "xclip", "xsel"}, "wl-paste --no-newline --type text"},
		{"wayland and x11: the program of wayland first", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, []string{"wl-paste", "xclip"}, "wl-paste --no-newline --type text"},
		{"wayland without its program: the one of x11", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, []string{"xclip"}, "xclip -selection clipboard -out"},
		{"x11 with xsel only", map[string]string{"DISPLAY": ":0"}, []string{"xsel"}, "xsel --clipboard --output"},
		{"x11: wl-paste is of no use", map[string]string{"DISPLAY": ":0"}, []string{"wl-paste"}, ""},
		{"no display", nil, []string{"wl-paste", "xclip", "xsel"}, ""},
		{"no program", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(name string) string { return tt.env[name] }
			lookPath := func(name string) (string, error) {
				if slices.Contains(tt.installed, name) {
					return "/usr/bin/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			if got := strings.Join(clipboardCommand(getenv, lookPath), " "); got != tt.want {
				t.Errorf("clipboardCommand = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunClipboard(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	tests := []struct {
		name, script string
		timeout      time.Duration
		want         string
	}{
		{"what the program prints", "printf 'a secret\\nline'", time.Minute, "a secret\nline"},
		{"a program that fails printed nothing", "printf text; exit 1", time.Minute, ""},
		{"a program that prints nothing", "true", time.Minute, ""},
		{"a program that takes too long", "sleep 30", 100 * time.Millisecond, ""},
		{"a program that is not there", "", time.Minute, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()
			argv := []string{"sh", "-c", tt.script}
			if tt.script == "" {
				argv = []string{"/nonexistent/tube-test-clipboard"}
			}
			start := time.Now()
			if got := runClipboard(ctx, argv); got != tt.want {
				t.Errorf("runClipboard = %q, want %q", got, tt.want)
			}
			if time.Since(start) > 10*time.Second {
				t.Errorf("runClipboard took %v", time.Since(start))
			}
		})
	}
	t.Run("more than the limit", func(t *testing.T) {
		got := runClipboard(context.Background(), []string{"sh", "-c", "head -c 200000 /dev/zero | tr '\\0' 'x'"})
		if len(got) != maxClipboard {
			t.Errorf("runClipboard kept %d bytes, want %d", len(got), maxClipboard)
		}
	})
}

// loginHarness is a harness on the login page of the profile Work, whose user name is "ada".
func loginHarness(t *testing.T, f *apptest.Fake, width, height int) *harness {
	t.Helper()
	h := newHarness(t, f, width, height)
	h.selectProfile("id-work")
	h.key("enter")
	if h.m().page != pageLogin {
		t.Fatalf("the page is %v", h.m().page)
	}
	return h
}

// rightClickPassword clicks the right button in the password field of the login page.
func (h *harness) rightClickPassword() {
	h.t.Helper()
	x, y := h.find(passwordLabel)
	h.rightClick(x+loginLabelWidth+3, y)
}

func TestTheMenuOfAField(t *testing.T) {
	const secret = "s3cret-from-the-clipboard"
	h := loginHarness(t, apptest.Sample(), 100, 30)
	h.withClipboard(secret + "\n")
	x, y := h.find("ada")
	h.rightClick(x+1, y)
	m := h.m()
	if m.menu == nil || m.menu.text != secret || m.login.focus != focusUser || m.login.user.cursor != len("ada") {
		t.Fatalf("after a right click: the menu %+v, the focus %d, the cursor %d", m.menu, m.login.focus, m.login.user.cursor)
	}
	h.has("│ Paste │")
	h.has("enter paste   esc close")
	if raw := h.model.View().Content; strings.Contains(raw, "s3cret") {
		t.Error("the text of the clipboard is drawn")
	}
	// The menu is under the click.
	if mx, my := h.find("╭───────╮"); mx != x+1 || my != y+1 {
		t.Errorf("the menu is at %d, %d; the click was at %d, %d", mx, my, x+1, y)
	}
	h.clickOn("Paste")
	if m := h.m(); m.menu != nil || m.login.user.value() != "ada"+secret {
		t.Errorf("after Paste: the menu %v, the user name %q", m.menu, m.login.user.value())
	}
	// The password: its text goes in, and only stars are drawn.
	h.rightClickPassword()
	if m := h.m(); m.menu == nil || m.login.focus != focusPassword {
		t.Fatalf("after a right click on the password: the menu %v, the focus %d", m.menu, m.login.focus)
	}
	h.key("enter")
	if m := h.m(); m.menu != nil || m.login.pass.value() != secret || m.page != pageLogin {
		t.Errorf("after enter: the menu %v, the page %v, the password has %d characters", m.menu, m.page, len(m.login.pass.value()))
	}
	h.has(passwordLabel + "   " + strings.Repeat("*", len(secret)))
	if raw := h.model.View().Content; strings.Contains(raw, "s3cret") && !strings.Contains(raw, "ada"+secret) {
		t.Error("the password is drawn")
	}
}

func TestTheMenuCloses(t *testing.T) {
	tests := []struct {
		name string
		do   func(h *harness)
		// wantPass is the password after it; the page stays the login page.
		wantPass string
	}{
		{"esc", func(h *harness) { h.key("esc") }, ""},
		{"another key is typed", func(h *harness) { h.typeText("x") }, "x"},
		{"a click beside it", func(h *harness) { h.click(60, 20) }, ""},
		{"a click on a button under it does not press the button", func(h *harness) { h.clickOn("[ Cancel ]") }, ""},
		{"a right click beside it", func(h *harness) { h.rightClick(60, 20) }, ""},
		{"a paste of the terminal", func(h *harness) { h.paste("typed") }, "typed"},
		{"the wheel does nothing, then esc", func(h *harness) { h.wheel(2); h.key("esc") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := loginHarness(t, apptest.Sample(), 100, 30)
			h.withClipboard("clip")
			h.rightClickPassword()
			if h.m().menu == nil {
				t.Fatal("no menu")
			}
			tt.do(h)
			if m := h.m(); m.menu != nil || m.page != pageLogin || m.login.pass.value() != tt.wantPass {
				t.Errorf("the menu %v, the page %v, the password %q", m.menu, m.page, m.login.pass.value())
			}
		})
	}
}

func TestNoMenuWithoutText(t *testing.T) {
	for _, text := range []string{"", "\n", "  \t ", "\x1b\x07"} {
		h := loginHarness(t, apptest.Sample(), 100, 30)
		h.withClipboard(text)
		h.rightClickPassword()
		if h.m().menu != nil {
			t.Errorf("a menu for the clipboard %q", text)
		}
		h.has(noClipboardText)
	}
	// A right click that is not on a field asks for nothing.
	h := loginHarness(t, apptest.Sample(), 100, 30)
	m := h.m()
	m.getenv = func(string) string { return "wayland-test" }
	m.lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	h.model = m
	for _, on := range []string{"[ Connect ]", "[x] remember", "The profile", "Tube"} {
		x, y := h.find(on)
		h.rightClick(x, y)
		if m := h.m(); m.menu != nil || m.clip != nil {
			t.Errorf("a right click on %q asked for the clipboard", on)
		}
	}
	// Under a question no menu opens.
	q := newHarness(t, apptest.Sample(), 100, 30)
	q.withClipboard("clip")
	q.key("d")
	x, y := q.find("[y] Delete")
	q.rightClick(x, y)
	if m := q.m(); m.menu != nil || m.ask == nil {
		t.Errorf("under a question: the menu %v", m.menu)
	}
}

func TestTheTerminalIsAskedForTheClipboard(t *testing.T) {
	// The harness has no display: the terminal is asked, and answers with a message.
	h := loginHarness(t, apptest.Sample(), 100, 30)
	h.send(tea.ClipboardMsg{Content: "nobody asked"})
	if h.m().menu != nil {
		t.Fatal("a clipboard that nobody asked for opened a menu")
	}
	h.rightClickPassword()
	if m := h.m(); m.clip == nil || m.menu != nil {
		t.Fatalf("after the right click: waiting %v, the menu %v", m.clip, m.menu)
	}
	h.send(tea.ClipboardMsg{Content: "from-the-terminal"})
	if m := h.m(); m.menu == nil || m.menu.text != "from-the-terminal" || m.clip != nil {
		t.Fatalf("after the answer: the menu %+v", m.menu)
	}
	h.clickOn("Paste")
	if got := h.m().login.pass.value(); got != "from-the-terminal" {
		t.Errorf("the password is %q", got)
	}
	// A terminal that does not answer: after two seconds the message says so, and a late
	// answer opens nothing.
	h.rightClickPassword()
	h.clock.Advance(time.Second)
	h.send(tickMsg{})
	if h.m().clip == nil {
		t.Fatal("the wait ended after a second")
	}
	h.clock.Advance(time.Second)
	h.send(tickMsg{})
	if m := h.m(); m.clip != nil || m.menu != nil {
		t.Fatalf("after two seconds: waiting %v, the menu %v", m.clip, m.menu)
	}
	h.has(noClipboard)
	h.send(tea.ClipboardMsg{Content: "late"})
	if h.m().menu != nil {
		t.Error("a late answer opened a menu")
	}
	// A key ends the wait too.
	h.rightClickPassword()
	h.key("left")
	h.send(tea.ClipboardMsg{Content: "late"})
	if m := h.m(); m.clip != nil || m.menu != nil {
		t.Error("an answer after a key opened a menu")
	}
}

func TestTheMenuOfTheOtherFields(t *testing.T) {
	t.Run("the File and the Name of the add page", func(t *testing.T) {
		f := sampleIn(sampleCwd)
		h := newHarness(t, f, 100, 30)
		h.key("a")
		h.withClipboard("~/Downloads/vpn/lab.ovpn")
		x, y := h.find("File")
		h.rightClick(x+addLabelWidth+4, y)
		h.clickOn("Paste")
		h.wantFolder(sampleCwd+"/vpn", "lab.ovpn", "~/Downloads/vpn/lab.ovpn", addFocusPath)
		h.withClipboard("Lab")
		h.rightClick(x+addLabelWidth+4, y+1)
		if h.m().add.focus != addFocusName || h.m().menu == nil {
			t.Fatalf("the focus is %d, the menu %v", h.m().add.focus, h.m().menu)
		}
		h.key("enter")
		h.key("enter")
		if got := f.Calls(); !slices.Equal(got, []string{"AddProfile(~/Downloads/vpn/lab.ovpn, Lab, , false)"}) {
			t.Errorf("Calls = %q", got)
		}
	})
	t.Run("the field of a profile", func(t *testing.T) {
		f := apptest.Sample()
		h := newHarness(t, f, 100, 30)
		h.selectProfile("id-work")
		h.keys("e", "u")
		h.withClipboard("-lovelace")
		// The field is the last line with the name: the line of the fact has it too.
		x, y := 0, 0
		for i, l := range strings.Split(h.screen(), "\n") {
			if j := strings.Index(l, "ada"); j >= 0 {
				x, y = j, i
			}
		}
		h.rightClick(x, y)
		h.clickOn("Paste")
		h.key("enter")
		if !called(f, "SetUsername(id-work, ada-lovelace)") {
			t.Errorf("Calls = %q", f.Calls())
		}
	})
	t.Run("a page that is busy has no menu", func(t *testing.T) {
		f := sampleIn(sampleCwd)
		f.Block = map[string]chan struct{}{"AddProfile": make(chan struct{})}
		h := newHarness(t, f, 100, 30)
		h.key("a")
		m := h.m()
		m, _ = m.addSubmit()
		h.model = m
		h.withClipboard("x")
		x, y := h.find("File")
		h.rightClick(x+addLabelWidth+4, y)
		if m := h.m(); m.menu != nil || m.clip != nil {
			t.Error("a busy page asked for the clipboard")
		}
	})
}

func TestTheMenuFitsTheScreen(t *testing.T) {
	for _, size := range [][2]int{{40, 10}, {60, 16}, {100, 30}} {
		w, hgt := size[0], size[1]
		h := loginHarness(t, apptest.Sample(), w, hgt)
		h.withClipboard("clip")
		_, y := h.find(passwordLabel)
		for _, at := range [][2]int{{w - 1, y}, {loginLabelWidth + 3, y}, {w - 3, y - 1}} {
			h.rightClick(at[0], at[1])
			if h.m().menu == nil {
				t.Fatalf("%dx%d: no menu for the click at %v:\n%s", w, hgt, at, h.screen())
			}
			lines := strings.Split(h.model.View().Content, "\n")
			if len(lines) != hgt {
				t.Errorf("%dx%d: %d lines", w, hgt, len(lines))
			}
			for i, l := range lines {
				if lipgloss.Width(l) != w {
					t.Errorf("%dx%d: with the menu at %v the line %d is %d cells wide", w, hgt, at, i, lipgloss.Width(l))
				}
			}
			for _, part := range []string{"╭───────╮", "│ Paste │", "╰───────╯"} {
				h.has(part)
			}
			h.clickOn("Paste")
			if h.m().menu != nil {
				t.Errorf("%dx%d: Paste was not clicked at %v", w, hgt, at)
			}
		}
	}
}

func TestTheHeaderKeepsTheRates(t *testing.T) {
	long := "xx-yyy.prod.vpnvendor.comvpnvendor_openvpn_udp"
	tests := []struct {
		width      int
		want, have string
	}{
		{160, long + "   connected 00:12:31   ↓ 1.2 MB/s  ↑ 80.0 kB/s", ""},
		{100, "…   connected 00:12:31   ↓ 1.2 MB/s  ↑ 80.0 kB/s", "firefox through xx-yyy.prod.vpnvendor."},
		{80, "…   connected 00:12:31   ↓ 1.2 MB/s  ↑ 80.0 kB/s", "firefox through xx-yyy.p…"},
		{70, "…   connected 00:12:31", "firefox through xx-yyy.prod.vpnvendor.co…"},
		{60, "…   connected 00:12:31", "firefox through xx-yyy.prod.v"},
		{40, "…   connected", "firefox through xx-…"},
	}
	for _, tt := range tests {
		f := apptest.Running()
		f.Run.ProfileName = long
		h := runHarness(t, f, tt.width, 12)
		header := strings.TrimRight(strings.Split(h.screen(), "\n")[0], " ")
		if !strings.HasSuffix(header, tt.want) || !strings.Contains(header, tt.have) || lipgloss.Width(header) > tt.width {
			t.Errorf("at %d columns the header is %q", tt.width, header)
		}
	}
	// After the end the name is cut too.
	f := apptest.Running()
	f.Run.ProfileName = long
	f.Run.AppRunning, f.Run.Exit = false, 1
	h := runHarness(t, f, 60, 12)
	if header := strings.TrimRight(strings.Split(h.screen(), "\n")[0], " "); lipgloss.Width(header) != 60 || !strings.HasSuffix(header, "…") {
		t.Errorf("after the end the header is %q", header)
	}
}

func TestGoldenMenu(t *testing.T) {
	h := loginHarness(t, apptest.Sample(), 100, 30)
	h.withClipboard("a text of the clipboard")
	h.typeText("hunter2")
	h.rightClickPassword()
	h.golden("login-menu")
	if raw := h.model.View().Content; strings.Contains(raw, "hunter2") || strings.Contains(raw, "a text of") {
		t.Error("the password or the clipboard is drawn")
	}
}
