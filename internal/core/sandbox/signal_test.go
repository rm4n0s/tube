// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/pty"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
)

// children are the pids of the children of the test process, of all its threads.
func children(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("/proc/self/task/*/children")
	if err != nil {
		t.Fatal(err)
	}
	var pids []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue // the thread has ended
		}
		pids = append(pids, strings.Fields(string(data))...)
	}
	return pids
}

// noChildrenLeft checks, when the test has ended and its sandboxes are closed, that no child of
// the test process is left. Call it first, so that its cleanup runs last.
func noChildrenLeft(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for {
			pids := children(t)
			if len(pids) == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("children of the test process are left: %v", pids)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

// waitGone waits at most d for the process to end, and says whether it did.
func waitGone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for !processGone(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

func TestSignalReachesTheApplication(t *testing.T) {
	noChildrenLeft(t)
	r := startSandbox(t, lookPath(t, "sh"), []string{"sh", "-c", `trap "exit 42" USR1; while :; do sleep 0.1; done`})
	r.configure(t, network4)
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // the trap is set
	if err := r.s.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	st, err := r.s.Wait()
	if err != nil || st != (ExitStatus{Code: 42}) {
		t.Errorf("Wait = %+v, %v", st, err)
	}
	if err := r.s.Signal(syscall.SIGUSR1); err != nil {
		t.Errorf("a signal after the end: %v", err)
	}
}

func TestSigtermEndsTheApplication(t *testing.T) {
	noChildrenLeft(t)
	r := startSandbox(t, lookPath(t, "sleep"), []string{"sleep", "30"})
	r.configure(t, network4)
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.s.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	st, err := r.s.Wait()
	if err != nil || st != (ExitStatus{Signal: syscall.SIGTERM}) || st.Status() != 143 {
		t.Errorf("Wait = %+v, %v", st, err)
	}
}

func TestTheSocketClosingEndsEverything(t *testing.T) {
	noChildrenLeft(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := startSandbox(t, exe, []string{"probe"}, sandboxtest.ProbeEnv("stubborn"), sandboxtest.PIDFileVar+"="+pidFile)
	r.configure(t, network4)
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil && len(data) > 0 {
			if pid, err = strconv.Atoi(string(data)); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatalf("the probe wrote no pid:\n%s", r.errors())
	}
	keeper := r.s.cmd.Process.Pid
	start := time.Now()
	if err := r.s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if !waitGone(pid, 2*time.Second) || !waitGone(keeper, time.Second) {
		t.Errorf("the application (%v) or the keeper (%v) is left", !processGone(pid), !processGone(keeper))
	}
	// The application ignored SIGTERM: it took SIGKILL, after the kill delay.
	if took := time.Since(start); took < 500*time.Millisecond {
		t.Errorf("the stubborn application ended after %v, before the kill delay", took)
	}
}

func TestTheKeeperSurvivesCtrlC(t *testing.T) {
	noChildrenLeft(t)
	r := startSandbox(t, lookPath(t, "sleep"), []string{"sleep", "30"})
	r.configure(t, network4)
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	keeper := r.s.cmd.Process.Pid
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGQUIT} {
		if err := syscall.Kill(keeper, sig); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(300 * time.Millisecond)
	if processGone(keeper) || processGone(r.s.appPID) {
		t.Fatalf("SIGINT ended the keeper (%v) or the application (%v)", processGone(keeper), processGone(r.s.appPID))
	}
	if err := r.s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestSigtermToTheKeeperIsPassedOn(t *testing.T) {
	noChildrenLeft(t)
	r := startSandbox(t, lookPath(t, "sleep"), []string{"sleep", "30"})
	r.configure(t, network4)
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(r.s.cmd.Process.Pid, syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	st, err := r.s.Wait()
	if err != nil || st != (ExitStatus{Signal: syscall.SIGHUP}) {
		t.Errorf("Wait = %+v, %v", st, err)
	}
}

func TestTheApplicationGetsDefaultSignals(t *testing.T) {
	noChildrenLeft(t)
	r := startProbe(t, "sigint")
	r.configure(t, network4)
	r.run(t)
	if out := strings.TrimSpace(r.output(t)); out != "false" {
		t.Errorf("SIGINT is ignored in the application: the probe printed %q", out)
	}
}

func TestTheApplicationOnItsOwnTerminal(t *testing.T) {
	noChildrenLeft(t)
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	closeLater(t, master)
	closeLater(t, slave)
	sh := lookPath(t, "sh")
	r := startSandboxWith(t, func(o *Options) {
		o.Terminal = true
		o.Stdin, o.Stdout, o.Stderr = slave, slave, slave
	}, sh, []string{"sh", "-c", "tty; echo hello; sleep 30"})
	r.configure(t, network4)
	if err := r.s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := master.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	buf := make([]byte, 256)
	for !strings.Contains(got.String(), "hello") {
		n, err := master.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			t.Fatalf("the terminal showed %q, then: %v", got.String(), err)
		}
	}
	if lines := strings.Split(got.String(), "\r\n"); len(lines) < 2 || !strings.HasPrefix(lines[0], "/dev/pts/") {
		t.Errorf("the application's terminal is not a pseudo-terminal: %q", got.String())
	}
	if _, err := master.Write([]byte{0x03}); err != nil { // what Ctrl-C types
		t.Fatal(err)
	}
	st, err := r.s.Wait()
	if err != nil || st != (ExitStatus{Signal: syscall.SIGINT}) || st.Status() != 130 {
		t.Errorf("Wait = %+v, %v", st, err)
	}
}

func TestLookPath(t *testing.T) {
	sh, err := LookPath("sh")
	if err != nil || !filepath.IsAbs(sh) {
		t.Errorf("LookPath(sh) = %q, %v", sh, err)
	}
	_, err = LookPath("tube-no-such-program")
	meta := errtest.Route(t, err, "LookPath.SandboxLookPathFailed")
	if meta["name"] != "tube-no-such-program" {
		t.Errorf("metadata = %v", meta)
	}
	_, err = LookPath("tube-\x1b[31mred")
	meta = errtest.Route(t, err, "LookPath.SandboxLookPathFailed")
	if name, _ := meta["name"].(string); strings.ContainsRune(name, 0x1b) {
		t.Errorf("the name in the error has an escape: %q", name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := LookPath("./testdata/app"); err != nil || got != filepath.Join(cwd, "testdata", "app") {
		t.Errorf("LookPath(./testdata/app) = %q, %v", got, err)
	}
}

func TestExitCodeOf(t *testing.T) {
	_, notFound := LookPath("tube-no-such-program")
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not in PATH", notFound, 127},
		{"not found when started", &errors.Error{Tag: TagAppNotFound}, 127},
		{"not executable", &errors.Error{Tag: TagAppNotExecutable}, 126},
		{"another error of the sandbox", &errors.Error{Tag: TagRunFailed}, 125},
		{"an error of another kind", os.ErrNotExist, 125},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCodeOf(tt.err); got != tt.want {
				t.Errorf("ExitCodeOf = %d, want %d", got, tt.want)
			}
		})
	}
}
