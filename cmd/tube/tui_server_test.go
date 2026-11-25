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

func TestTheTUIRunsPing(t *testing.T) {
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
	defer func() {
		if err := master.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := pty.SetSize(master, 100, 30); err != nil {
		t.Fatal(err)
	}
	cmd := e.command(t, "ping", "-c", "2", "-W", "2", server.TunnelAddr().String())
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := slave.Close(); err != nil {
		t.Fatal(err)
	}
	screen := readScreen(master)
	screen.waitFor(t, "test")
	if _, err := master.WriteString("\r"); err != nil {
		t.Fatal(err)
	}
	screen.waitFor(t, "bytes from "+server.TunnelAddr().String())
	if err := cmd.Wait(); err != nil {
		t.Errorf("tube ended with %v; the screen:\n%s\nthe server's log:\n%s", err, ansi.Strip(screen.buf.String()), server.Log())
	}
	select {
	case <-screen.done:
	case <-time.After(10 * time.Second):
		t.Error("the pseudo-terminal stayed open")
	}
}
