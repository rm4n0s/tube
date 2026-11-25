// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/rm4n0s/tube/internal/core/pty"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// screenReader reads the master side of a pseudo-terminal into a buffer that the test polls.
type screenReader struct {
	buf  syncBuffer
	done chan struct{}
}

// readScreen reads the master side until it fails (the slave side is closed everywhere).
func readScreen(master *os.File) *screenReader {
	r := &screenReader{done: make(chan struct{})}
	go func() {
		defer close(r.done)
		chunk := make([]byte, 4096)
		for {
			n, err := master.Read(chunk)
			if n > 0 {
				// A syncBuffer's Write does not fail.
				_, _ = r.buf.Write(chunk[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return r
}

// waitFor waits until the text, without escape sequences, has want.
func (r *screenReader) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(ansi.Strip(r.buf.String()), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no %q on the screen:\n%s", want, ansi.Strip(r.buf.String()))
}

// inTerminal is the program running in a pseudo-terminal of the test.
type inTerminal struct {
	cmd    *exec.Cmd
	master *os.File
	screen *screenReader
}

// buildTube builds the program for a test against the server, which is skipped where there is
// no server.
func buildTube(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("openvpn"); err != nil {
		t.Skip("openvpn is not installed")
	}
	if os.Getenv("TUBE_NS") != "1" {
		t.Skip("not inside the namespace of scripts/vpn-ns.sh")
	}
	bin := filepath.Join(t.TempDir(), "tube")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// startInTerminal builds the program, starts a server with its profile "test", and runs the
// program with the arguments in a pseudo-terminal of 100x30.
func startInTerminal(t *testing.T, args ...string) (*inTerminal, *vpntest.Server) {
	t.Helper()
	e, server := setup(t, buildTube(t), vpntest.ServerOptions{})
	return e.inTerminal(t, args...), server
}

// inTerminal runs the program with the arguments in a pseudo-terminal of 100x30, which is its
// terminal: what it asks, it asks there.
func (e e2e) inTerminal(t *testing.T, args ...string) *inTerminal {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := master.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := pty.SetSize(master, 100, 30); err != nil {
		t.Fatal(err)
	}
	cmd := e.command(t, args...)
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := slave.Close(); err != nil {
		t.Fatal(err)
	}
	return &inTerminal{cmd: cmd, master: master, screen: readScreen(master)}
}

// press writes keys to the program's terminal.
func (p *inTerminal) press(t *testing.T, keys string) {
	t.Helper()
	if _, err := p.master.WriteString(keys); err != nil {
		t.Fatal(err)
	}
}

// wait waits for the end of the program, 10 seconds at most, and returns its exit status.
func (p *inTerminal) wait(t *testing.T, server *vpntest.Server) int {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The status says how it ended.
		_ = p.cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("tube did not end; the screen:\n%s\nthe server's log:\n%s", ansi.Strip(p.screen.buf.String()), server.Log())
	}
	select {
	case <-p.screen.done:
	case <-time.After(10 * time.Second):
		t.Error("the pseudo-terminal stayed open")
	}
	return p.cmd.ProcessState.ExitCode()
}

func TestTheTUIRunsPing(t *testing.T) {
	p, server := startInTerminal(t, "ping", "-c", "2", "-W", "2", "10.8.0.1")
	p.screen.waitFor(t, "test")
	p.press(t, "\r")
	p.screen.waitFor(t, "[Status]")
	p.press(t, "\t")
	p.screen.waitFor(t, "bytes from "+server.TunnelAddr().String())
	p.screen.waitFor(t, "ended with status 0")
	p.press(t, "q")
	if status := p.wait(t, server); status != 0 {
		t.Errorf("the status is %d; the screen:\n%s", status, ansi.Strip(p.screen.buf.String()))
	}
}

func TestQuitStopsTheApplication(t *testing.T) {
	p, server := startInTerminal(t, "sleep", "300")
	p.screen.waitFor(t, "test")
	p.press(t, "\r")
	p.screen.waitFor(t, "[Status]")
	p.press(t, "q")
	p.screen.waitFor(t, "Stop it and quit?")
	p.press(t, "y")
	if status := p.wait(t, server); status != 143 {
		t.Errorf("the status is %d; the screen:\n%s", status, ansi.Strip(p.screen.buf.String()))
	}
	if left := processesWith("sleep\x00300"); len(left) > 0 {
		t.Errorf("the application is left: %v", left)
	}
}

// waitForAfter waits until what was read after the first mark bytes, without escape sequences,
// has want.
func (r *screenReader) waitForAfter(t *testing.T, mark int, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(ansi.Strip(r.buf.String()[mark:]), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no %q on the screen after byte %d:\n%s", want, mark, ansi.Strip(r.buf.String()[mark:]))
}

// seqPattern finds the sequence numbers of ping's replies.
var seqPattern = regexp.MustCompile(`bytes from [0-9.]+: icmp_seq=([0-9]+)`)

// lastReply is the largest sequence number of a reply on the screen so far, or 0.
func (r *screenReader) lastReply() int {
	last := 0
	for _, m := range seqPattern.FindAllStringSubmatch(ansi.Strip(r.buf.String()), -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > last {
			last = n
		}
	}
	return last
}

// waitForReply waits until a reply with a sequence number above after is on the screen.
func (r *screenReader) waitForReply(t *testing.T, after int) int {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if n := r.lastReply(); n > after {
			return n
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no reply after %d on the screen:\n%s", after, ansi.Strip(r.buf.String()))
	return 0
}

func TestDisconnectInTheTUI(t *testing.T) {
	p, server := startInTerminal(t, "ping", "-c", "30", "-i", "0.5", "-W", "1", "10.8.0.1")
	p.screen.waitFor(t, "test")
	p.press(t, "\r")
	p.screen.waitFor(t, "[Status]")
	p.press(t, "\t")
	p.screen.waitForReply(t, 0)
	// The Network tab: x, y.
	p.press(t, "\t")
	p.screen.waitFor(t, "ping uses test.")
	p.press(t, "x")
	p.screen.waitFor(t, "Disconnect?")
	p.press(t, "y")
	p.screen.waitFor(t, "has no network: disconnected.")
	// Back on the Output tab (shift+tab), no reply comes for three seconds.
	mark := len(p.screen.buf.String())
	p.press(t, "\x1b[Z")
	p.screen.waitForAfter(t, mark, "[Output]")
	time.Sleep(500 * time.Millisecond)
	before := p.screen.lastReply()
	time.Sleep(3 * time.Second)
	if after := p.screen.lastReply(); after != before {
		t.Fatalf("replies came after the disconnect: %d, then %d", before, after)
	}
	// The Network tab again: enter on the profile connects it.
	p.press(t, "\t")
	p.screen.waitFor(t, "has no network: disconnected.")
	p.press(t, "\r")
	p.screen.waitFor(t, "Now using test.")
	p.press(t, "\x1b[Z")
	p.screen.waitForReply(t, before)
	p.press(t, "\t")
	p.press(t, "q")
	p.screen.waitFor(t, "Stop it and quit?")
	p.press(t, "y")
	p.wait(t, server)
}
