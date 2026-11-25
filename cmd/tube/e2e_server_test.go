// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// e2e is the built program and the folders of one test.
type e2e struct {
	bin string
	env []string
}

// setup makes isolated folders, starts a server with the options and adds its profile as
// "test" to the folders that the program will use.
func setup(t *testing.T, bin string, opts vpntest.ServerOptions) (e2e, *vpntest.Server) {
	t.Helper()
	server := vpntest.StartServer(t, opts)
	dirs := storagetest.Isolate(t)
	store := profiles.NewStore(dirs.Config, clock.System())
	if _, err := store.Add(context.Background(), server.ProfilePath(), "test", profiles.AddOptions{}); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
		"TUBE_CONFIG_DIR=" + dirs.Config, "TUBE_STATE_DIR=" + dirs.State, "XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR"),
	}
	return e2e{bin: bin, env: env}, server
}

// dir is the folder that a variable of the program's environment names.
func (e e2e) dir(name string) string {
	for _, kv := range e.env {
		if value, ok := strings.CutPrefix(kv, name+"="); ok {
			return value
		}
	}
	return ""
}

// command is the program with the arguments, under a context of 60 seconds.
func (e e2e) command(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, e.bin, args...)
	cmd.Env = e.env
	cmd.WaitDelay = 10 * time.Second
	return cmd
}

// run runs the program and returns its exit status and what it wrote.
func (e e2e) run(t *testing.T, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	cmd := e.command(t, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	// The status says how it ended.
	_ = cmd.Run()
	return cmd.ProcessState.ExitCode(), out.String(), errOut.String()
}

// syncBuffer is a buffer that a process writes while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// processesWith are the pids of the processes whose command line has text in it.
func processesWith(text string) []string {
	var pids []string
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err == nil && bytes.Contains(cmdline, []byte(text)) {
			pids = append(pids, e.Name())
		}
	}
	return pids
}

func TestEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("openvpn"); err != nil {
		t.Skip("openvpn is not installed")
	}
	if os.Getenv("TUBE_NS") != "1" {
		t.Skip("not inside the namespace of scripts/vpn-ns.sh")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tube")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	t.Run("a ping", func(t *testing.T) {
		e, server := setup(t, bin, vpntest.ServerOptions{})
		status, _, stderr := e.run(t, "--profile", "test", "ping", "-c", "2", "-W", "2", server.TunnelAddr().String())
		if status != 0 || !strings.Contains(stderr, "tube: connected to test") {
			t.Errorf("status %d, standard error:\n%s\nthe server's log:\n%s", status, stderr, server.Log())
		}
	})
	t.Run("quiet", func(t *testing.T) {
		e, _ := setup(t, bin, vpntest.ServerOptions{})
		if status, _, stderr := e.run(t, "--quiet", "--profile", "test", "true"); status != 0 || stderr != "" {
			t.Errorf("status %d, standard error %q", status, stderr)
		}
	})
	t.Run("the exit status", func(t *testing.T) {
		e, _ := setup(t, bin, vpntest.ServerOptions{})
		if status, _, stderr := e.run(t, "--profile", "test", "sh", "-c", "exit 7"); status != 7 {
			t.Errorf("status %d, standard error:\n%s", status, stderr)
		}
	})
	t.Run("the names go to the VPN", func(t *testing.T) {
		if _, err := exec.LookPath("getent"); err != nil {
			t.Skip("getent is not installed")
		}
		server0 := netip.MustParseAddr("10.8.0.1")
		e, server := setup(t, bin, vpntest.ServerOptions{DNS: []netip.Addr{server0}})
		vpntest.StartDNS(t, netip.AddrPortFrom(server.TunnelAddr(), 53), map[string]netip.Addr{"tube-test.example": netip.MustParseAddr("192.0.2.7")})
		status, stdout, stderr := e.run(t, "--profile", "test", "getent", "hosts", "tube-test.example")
		if status != 0 || !strings.Contains(stdout, "192.0.2.7") {
			t.Errorf("status %d, standard output %q, standard error:\n%s", status, stdout, stderr)
		}
	})
	t.Run("a wrong password", func(t *testing.T) {
		e, _ := setup(t, bin, vpntest.ServerOptions{Username: "alice", Password: "right"})
		for _, tt := range []struct {
			password, stderr string
			status           int
		}{
			{"wrong", "tube: The VPN server refused the user name or the password. (AuthFailed)\n", 125},
			{"right", "", 0},
		} {
			auth := filepath.Join(t.TempDir(), "auth")
			if err := os.WriteFile(auth, []byte("alice\n"+tt.password+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			status, _, stderr := e.run(t, "--quiet", "--profile", "test", "--auth-file", auth, "true")
			if status != tt.status || stderr != tt.stderr {
				t.Errorf("the password %s: status %d, standard error %q", tt.password, status, stderr)
			}
		}
	})
	t.Run("the remembered password", func(t *testing.T) {
		e, _ := setup(t, bin, vpntest.ServerOptions{Username: "alice", Password: "right"})
		ctx := context.Background()
		store := profiles.NewStore(e.dir("TUBE_CONFIG_DIR"), clock.System())
		list, err := store.List(ctx)
		if err != nil || len(list) != 1 {
			t.Fatalf("the profiles are %+v, %v", list, err)
		}
		id := list[0].ID
		remember := func(password string) {
			t.Helper()
			if err := store.SetUsername(ctx, id, "alice"); err != nil {
				t.Fatal(err)
			}
			if err := store.SetPassword(ctx, id, password); err != nil {
				t.Fatal(err)
			}
		}
		// Nothing is asked: there is no terminal here to ask on, and no file.
		remember("right")
		if status, _, stderr := e.run(t, "--quiet", "--profile", "test", "true"); status != 0 || stderr != "" {
			t.Errorf("with the remembered password: status %d, standard error %q", status, stderr)
		}
		// A remembered password that is wrong is a refused login; the file of --auth-file wins
		// over it, and does not change it.
		remember("stale")
		status, _, stderr := e.run(t, "--quiet", "--profile", "test", "true")
		if status != 125 || stderr != "tube: The VPN server refused the user name or the password. (AuthFailed)\n" {
			t.Errorf("with a stale remembered password: status %d, standard error %q", status, stderr)
		}
		auth := filepath.Join(t.TempDir(), "auth")
		if err := os.WriteFile(auth, []byte("alice\nright\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if status, _, stderr := e.run(t, "--quiet", "--profile", "test", "--auth-file", auth, "true"); status != 0 || stderr != "" {
			t.Errorf("with --auth-file: status %d, standard error %q", status, stderr)
		}
		if got, err := store.Password(ctx, id); err != nil || got != "stale" {
			t.Errorf("--auth-file changed the remembered password: %v", err)
		}
	})
	t.Run("a firefox gets a profile of its own", func(t *testing.T) {
		e, _ := setup(t, bin, vpntest.ServerOptions{})
		// A program of the test with the name of Firefox, which prints its arguments: the real
		// one is never started.
		fake := filepath.Join(t.TempDir(), "firefox")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		own := filepath.Join(e.dir("TUBE_STATE_DIR"), "firefox-profile")
		status, stdout, stderr := e.run(t, "--profile", "test", fake, "https://example.com")
		if want := "--no-remote\n--profile\n" + own + "\nhttps://example.com\n"; status != 0 || stdout != want {
			t.Errorf("status %d, the arguments were %q, want %q; standard error:\n%s", status, stdout, want, stderr)
		}
		if !strings.Contains(stderr, "tube: firefox runs on a profile of its own, which Tube keeps") {
			t.Errorf("no line about the profile:\n%s", stderr)
		}
		if info, err := os.Stat(own); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("the folder of the profile: %v, %v", info, err)
		}
		// A profile that the user chose stays the user's.
		status, stdout, _ = e.run(t, "--quiet", "--profile", "test", fake, "-P", "mine")
		if status != 0 || stdout != "-P\nmine\n" {
			t.Errorf("status %d, the arguments were %q", status, stdout)
		}
	})
	t.Run("the application is not found", func(t *testing.T) {
		e, server := setup(t, bin, vpntest.ServerOptions{})
		status, _, stderr := e.run(t, "--profile", "test", "tube-no-such-program")
		if status != 127 || !strings.Contains(stderr, `"tube-no-such-program" was not found`) {
			t.Errorf("status %d, standard error %q", status, stderr)
		}
		if strings.Contains(server.Log(), "Initial packet") {
			t.Error("Tube connected before it knew that the application is not there")
		}
	})
	t.Run("SIGTERM", func(t *testing.T) {
		e, _ := setup(t, bin, vpntest.ServerOptions{})
		cmd := e.command(t, "--profile", "test", "sleep", "31.7")
		stderr, err := cmd.StderrPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		lines := bufio.NewScanner(stderr)
		for lines.Scan() && !strings.HasPrefix(lines.Text(), "tube: connected to test") {
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			for lines.Scan() {
			}
			// The status says how it ended.
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Tube did not end within 10 seconds of SIGTERM")
		}
		if status := cmd.ProcessState.ExitCode(); status != 143 {
			t.Errorf("the status is %d", status)
		}
		if left := processesWith("sleep\x0031.7\x00"); len(left) > 0 {
			t.Errorf("the application is left: %v", left)
		}
	})
	t.Run("the server restarts", func(t *testing.T) {
		e, server := setup(t, bin, vpntest.ServerOptions{})
		cmd := e.command(t, "--profile", "test", "ping", "-i", "1", "-c", "25", "-W", "1", server.TunnelAddr().String())
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stderr syncBuffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		// The replies, with their numbers, as ping prints them.
		replies := make(chan int, 64)
		go func() {
			defer close(replies)
			lines := bufio.NewScanner(stdout)
			for lines.Scan() {
				var seq int
				if i := strings.Index(lines.Text(), "icmp_seq="); i >= 0 {
					if _, err := fmt.Sscanf(lines.Text()[i:], "icmp_seq=%d", &seq); err == nil {
						replies <- seq
					}
				}
			}
		}()
		last := 0
		for range 3 {
			last = <-replies
		}
		server.Stop()
		time.Sleep(3 * time.Second)
		server.Start()
		after := 0
		for seq := range replies {
			if seq > last+1 {
				after++
			}
		}
		// The status says how it ended; ping counts lost replies as a failure.
		_ = cmd.Wait()
		if after == 0 {
			t.Errorf("no reply came after the server restarted; standard error:\n%s\nthe server's log:\n%s", stderr.String(), server.Log())
		}
		for _, want := range []string{"the VPN connection ended", "connected again"} {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("standard error has no %q:\n%s", want, stderr.String())
			}
		}
	})
	t.Run("nothing is left", func(t *testing.T) {
		if left := processesWith(dir); len(left) > 0 {
			t.Errorf("processes of the program are left: %v", left)
		}
	})
}
