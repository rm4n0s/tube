// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// The errors of this file are made in one goroutine of the session and returned or logged in
// another. Their routes go on from the function that handed the work over, as if it had done
// the work itself (errors.Async), with nothing of the goroutine in them (CLAUDE.md "Error
// handling").

// loggedError is the error of the first record of a JSON log with the message, read back as
// the log has it (CLAUDE.md "Testing errors").
func loggedError(t *testing.T, log, msg string) errors.ErrorJson {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(log))
	for sc.Scan() {
		// The err of another record may be a plain text: only the one asked for is read.
		var record struct {
			Msg string          `json:"msg"`
			Err json.RawMessage `json:"err"`
		}
		if err := json.Unmarshal(sc.Bytes(), &record); err != nil {
			t.Fatalf("a line of the log is not JSON: %v\n%s", err, sc.Text())
		}
		if record.Msg != msg {
			continue
		}
		var logged errors.ErrorJson
		if err := json.Unmarshal(record.Err, &logged); err != nil {
			t.Fatalf("the err of the record %q is not an error of Tube: %v\n%s", msg, err, sc.Text())
		}
		return logged
	}
	t.Fatalf("the log has no record %q:\n%s", msg, log)
	return errors.ErrorJson{}
}

func TestTheErrorOfARedialStartsAtTheController(t *testing.T) {
	tests := []struct {
		name string
		// redial fails as vpn.Dial does: it makes the error where the dial fails, in the
		// goroutine of the try.
		redial    func(context.Context) (vpn.Tunnel, error)
		wantMsg   string
		wantRoute string
	}{
		{
			name: "a server that does not answer",
			redial: func(context.Context) (vpn.Tunnel, error) {
				return nil, errors.New("PingTimeout", "the server did not answer")
			},
			wantMsg: "connecting again failed",
			wantRoute: "Session.control->controller.redial->controller.redial.func1->redialer.redial->" +
				"TestTheErrorOfARedialStartsAtTheController.func1.PingTimeout",
		},
		{
			name: "a login that is refused",
			redial: func(context.Context) (vpn.Tunnel, error) {
				return nil, errors.New("AuthFailed", "the server refused the login")
			},
			wantMsg: "connecting again was refused",
			wantRoute: "Session.control->controller.redial->controller.redial.func1->redialer.redial->" +
				"TestTheErrorOfARedialStartsAtTheController.func2.AuthFailed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lines, log syncBuffer
			tun1 := pingTunnel(network)
			clk, r := fakeClock(), &redialer{answers: []func(context.Context) (vpn.Tunnel, error){tt.redial}}
			opts, rn := reconnecting(t, tun1, lookPath(t, "sleep"), []string{"sleep", "30"}, r, clk, &lines)
			opts.Logger = slog.New(slog.NewJSONHandler(&log, nil))
			rn = start(t, opts, rn)
			tun1.Fail(io.EOF)
			eventually(t, "the session waits", func() bool { return clk.Pending() == 1 })
			clk.Advance(time.Second)
			eventually(t, "the record of the try", func() bool { return strings.Contains(log.String(), tt.wantMsg) })
			rn.s.Signal(syscall.SIGTERM)
			rn.wait(t)
			logged := loggedError(t, log.String(), tt.wantMsg)
			if !logged.HasRoute(tt.wantRoute) {
				t.Errorf("unexpected failure path: %s (want %s)", logged.Route(), tt.wantRoute)
			}
		})
	}
}

// children are the pids of the children of a process ("self" is the test process).
func children(t *testing.T, of string) []int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("/proc", of, "task", "*", "children"))
	if err != nil {
		t.Fatal(err)
	}
	var pids []int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue // a thread that has ended
		}
		for _, field := range strings.Fields(string(data)) {
			pid, err := strconv.Atoi(field)
			if err != nil {
				t.Fatal(err)
			}
			pids = append(pids, pid)
		}
	}
	return pids
}

// keeperAndApp are the pids of the keeper of the session of a test, which is the child of the
// test process that was started as the keeper, and of its application.
func keeperAndApp(t *testing.T) (keeper, app int) {
	t.Helper()
	for _, pid := range children(t, "self") {
		cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
		if err != nil || !strings.HasPrefix(string(cmdline), sandbox.KeeperArg0+"\x00") {
			continue
		}
		apps := children(t, strconv.Itoa(pid))
		if len(apps) != 1 {
			t.Fatalf("the keeper %d has the children %v, want its application alone", pid, apps)
		}
		return pid, apps[0]
	}
	t.Fatal("the test process has no keeper")
	return 0, 0
}

func TestTheErrorOfALostKeeperStartsAtStart(t *testing.T) {
	var log syncBuffer
	// The application ends by itself after a second, should the test fail to end it.
	opts, rn := options(t, pingTunnel(network), lookPath(t, "sleep"), []string{"sleep", "1"})
	opts.Logger = slog.New(slog.NewJSONHandler(&log, nil))
	rn = start(t, opts, rn)
	keeper, app := keeperAndApp(t)
	// The application lost its keeper, which would have ended it: the test does, so that
	// nothing of it is left.
	t.Cleanup(func() {
		_ = syscall.Kill(app, syscall.SIGKILL) // it may have ended by itself
		eventually(t, "the application has ended", func() bool { return gone(app) })
	})
	if err := syscall.Kill(keeper, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	status, err := rn.s.Wait()
	if status != sandbox.StatusTubeFailed {
		t.Errorf("the status is %d, want %d", status, sandbox.StatusTubeFailed)
	}
	// The wait for the application ran in a goroutine that Start started, from the helper start.
	want := "TestTheErrorOfALostKeeperStartsAtStart->start->Start->Session.waitApp->Sandbox.Wait.SandboxKeeperLost"
	errtest.Route(t, err, want)
	appErr, _ := errors.FromError(err)
	if route := appErr.Route(); strings.Contains(route, "Start.func") {
		t.Errorf("the route has the goroutine of the wait in it: %s", route)
	}
	// The log has it with the same route.
	if logged := loggedError(t, log.String(), "the keeper was lost"); !logged.HasRoute(want) {
		t.Errorf("unexpected failure path in the log: %s (want %s)", logged.Route(), want)
	}
}
