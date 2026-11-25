// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
)

// The promises of docs/SPEC.md §14 that a session keeps: each test shows a refusal.

// syncBuffer is a buffer that goroutines may write while the test reads it.
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

// waitFile waits at most d for the file to exist with something in it, and returns what.
func waitFile(t *testing.T, path string, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return string(data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not come within %v", filepath.Base(path), d)
			return ""
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNoNetworkBeforeThePump(t *testing.T) {
	nstest.Require(t)
	ping := lookPath(t, "ping")
	sb, err := sandbox.Start(context.Background(), sandbox.Options{
		Path: ping, Argv: []string{"ping", "-c", "1", "-W", "1", "10.9.0.1"}, TempDir: t.TempDir(), KillDelay: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sb.Close() }) // only the application's status matters here
	if _, err := sb.Configure(context.Background(), network, nil); err != nil {
		t.Fatal(err)
	}
	// The packets reach the device, and nothing moves them on.
	peer := sandboxtest.NewPeer(t, sb.Device())
	if err := sb.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := sb.Wait()
	if err != nil || st.Status() == 0 {
		t.Errorf("ping without a pump ended with %+v, %v", st, err)
	}
	if len(peer.Packets()) == 0 {
		t.Error("the ping did not reach the device")
	}
}

func TestNoNetworkAfterTheTunnelEnded(t *testing.T) {
	lookPath(t, "ping")
	tun := pingTunnel(network)
	path, argv := probe(t, "10.9.0.1", "10.9.0.1")
	opts, r := options(t, tun, path, argv)
	steps := pingSteps(t, &opts)
	r = start(t, opts, r)
	steps.ready(t, 1)
	tun.Fail(io.EOF)
	steps.goOn(t, 2)
	r.wait(t)
	if out := strings.TrimSpace(read(t, r.stdout)); out != "1=ok 2=failed" {
		t.Errorf("the probe printed %q:\n%s", out, read(t, r.stderr))
	}
}

// steps are the files through which a test and the probe ping-steps talk.
type steps struct{ readyFile, goFile string }

// pingSteps makes the probe of the options ping-steps, with files of the test.
func pingSteps(t *testing.T, opts *Options) steps {
	t.Helper()
	dir := t.TempDir()
	st := steps{filepath.Join(dir, "ready"), filepath.Join(dir, "go")}
	opts.ExtraEnv = append(opts.ExtraEnv, sandboxtest.ProbeEnv("ping-steps"), sandboxtest.ReadyFileVar+"="+st.readyFile, sandboxtest.GoFileVar+"="+st.goFile)
	return st
}

// ready waits until the probe has done the ping number i.
func (st steps) ready(t *testing.T, i int) {
	t.Helper()
	waitFile(t, st.readyFile+"."+strconv.Itoa(i), 20*time.Second)
}

// goOn lets the probe do the ping number i.
func (st steps) goOn(t *testing.T, i int) {
	t.Helper()
	if err := os.WriteFile(st.goFile+"."+strconv.Itoa(i), []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// report runs the probe report in a session and reads what it printed.
func report(t *testing.T) sandboxtest.Report {
	t.Helper()
	path, argv := probe(t)
	opts, r := options(t, pingTunnel(network), path, argv)
	opts.ExtraEnv = []string{sandboxtest.ProbeEnv("report")}
	r = start(t, opts, r)
	if status := r.wait(t); status != 0 {
		t.Fatalf("the probe ended with %d:\n%s", status, read(t, r.stderr))
	}
	var rep sandboxtest.Report
	if err := json.Unmarshal([]byte(read(t, r.stdout)), &rep); err != nil {
		t.Fatalf("the report cannot be read: %v", err)
	}
	return rep
}

func TestNothingButTheTunnelAndTheLoopback(t *testing.T) {
	rep := report(t)
	var names []string
	for _, l := range rep.Links {
		names = append(names, l.Name)
	}
	if strings.Join(names, " ") != "lo tube0" {
		t.Errorf("the links inside are %v", names)
	}
}

func TestTheApplicationCannotChangeTheNetwork(t *testing.T) {
	if rep := report(t); rep.AddRouteError == "" {
		t.Error("the application added a default route")
	}
}

func TestNoNetworkAfterTubeIsKilled(t *testing.T) {
	nstest.Require(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ready, out := filepath.Join(dir, "ready"), filepath.Join(dir, "links")
	holder := exec.Command(exe)
	holder.Env = append(os.Environ(), holderVar+"=1", holderDirVar+"="+t.TempDir(),
		sandboxtest.ReadyFileVar+"="+ready, sandboxtest.OutFileVar+"="+out)
	var stderr syncBuffer
	holder.Stderr = &stderr
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Killed below; this is for a test that failed before.
		_ = holder.Process.Kill()
		_ = holder.Wait()
	})
	pid, err := strconv.Atoi(strings.TrimPrefix(waitFile(t, ready, 10*time.Second), "ready "))
	if err != nil {
		t.Fatalf("the ready file: %v\n%s", err, stderr.String())
	}
	keeper := parentOf(t, pid)
	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// The error of a killed process says it was killed.
	_ = holder.Wait()
	if links := strings.Fields(waitFile(t, out, 3*time.Second)); strings.Join(links, " ") != "lo" {
		t.Errorf("after Tube was killed, the links inside are %v", links)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !gone(pid) || !gone(keeper) {
		if time.Now().After(deadline) {
			t.Fatalf("the application (%v) or the keeper (%v) is left", !gone(pid), !gone(keeper))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// parentOf is the pid of the parent of the process.
func parentOf(t *testing.T, pid int) int {
	t.Helper()
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		t.Fatal(err)
	}
	// The name, in parentheses, may hold spaces: the fields after it are what counts.
	fields := strings.Fields(string(data[bytes.LastIndexByte(data, ')')+1:]))
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatal(err)
	}
	return ppid
}

// gone says whether the process has ended.
func gone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func TestASecretStaysOutOfTheLog(t *testing.T) {
	var log, lines syncBuffer
	opts, r := options(t, pingTunnel(network), lookPath(t, "sh"), []string{"sh", "-c", "exit 0"})
	opts.ProfileName = "work\x1b]0;marker\x07\x1b[2J"
	opts.Logger = slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	opts.Lines = &lines
	start(t, opts, r).wait(t)
	for name, text := range map[string]string{"log": log.String(), "lines": lines.String()} {
		if text == "" {
			t.Errorf("the %s is empty", name)
		}
		for _, c := range []byte(text) {
			if c < 0x20 && c != '\n' {
				t.Errorf("the %s has the byte %#x:\n%q", name, c, text)
				break
			}
		}
	}
}
