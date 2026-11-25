// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package main

import (
	"os"
	"os/exec"
	"path/filepath"
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

// startInTerminal builds the program, starts a server with its profile "test", and runs the
// program with the arguments in a pseudo-terminal of 100x30.
func startInTerminal(t *testing.T, args ...string) (*inTerminal, *vpntest.Server) {
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
	e, server := setup(t, bin, vpntest.ServerOptions{})
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
	return &inTerminal{cmd: cmd, master: master, screen: readScreen(master)}, server
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
