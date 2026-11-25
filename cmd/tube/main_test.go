// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/pty"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
)

func TestMain(m *testing.M) {
	// run starts the keeper (sandbox.Check): it is the test binary.
	if sandbox.IsKeeper() {
		os.Exit(sandbox.KeeperMain())
	}
	nstest.Main(m, nil)
}

// failingWriter is a stream that cannot be written to.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// streams are a standard input that is a pipe and a standard output that is a file, for run.
func streams(t *testing.T) (stdin, stdout *os.File, read func() string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Only the test's own pipe.
		_ = r.Close()
		_ = w.Close()
	})
	out, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = out.Close() }) // read through its path
	return r, out, func() string {
		data, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		namespace  bool // the run gets as far as the check of the machine
		wantStatus int
		wantOut    string
		wantErr    string
	}{
		{"help", []string{"--help"}, false, 0, usage, ""},
		{"-h", []string{"-h"}, false, 0, usage, ""},
		{"version", []string{"--version"}, false, 0, "tube " + app.Version() + "\n", ""},
		{"version with one dash", []string{"-version"}, false, 0, "tube " + app.Version() + "\n", ""},
		{"an option that is not known", []string{"--nosuch", "ping"}, false, 2, "",
			"tube: flag provided but not defined: -nosuch (TubeUsage)\ntube: Try 'tube --help'.\n"},
		{"a program that is not there", []string{"tube-no-such-program"}, false, 127, "",
			"tube: The program \"tube-no-such-program\" was not found. (SandboxLookPathFailed)\n"},
		{"no arguments, no terminal", nil, false, 125, "", "tube: the TUI needs a terminal; use --profile <name> (TubeNotATerminal)\n"},
		{"no --profile, no terminal", []string{"true"}, false, 125, "", "tube: the TUI needs a terminal; use --profile <name> (TubeNotATerminal)\n"},
		{"a profile that is not there", []string{"--profile", "nothing", "true"}, true, 125, "",
			"tube: There is no profile called \"nothing\", and no .ovpn file at that path. (AppNoProfileOrFile)\n"},
		{"--dns without --profile", []string{"--dns", "9.9.9.9", "true"}, false, 2, "",
			"tube: --dns goes with --profile (TubeOptionWithoutProfile)\ntube: Try 'tube --help'.\n"},
		{"--auth-file with --password", []string{"-p", "work", "--auth-file", "f", "--password", "hunter2", "true"}, false, 2, "",
			"tube: --auth-file has the user name and the password: leave --username and --password out (TubeAuthFileWithCredentials)\ntube: Try 'tube --help'.\n"},
		{"a mistake in --dns adds nothing", []string{"--profile", "nothing", "--dns", "dns.example", "true"}, true, 125, "",
			"tube: \"dns.example\" is not an IP address. (ProfileDNSInvalid)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.namespace {
				nstest.Require(t)
			}
			storagetest.Isolate(t)
			var errOut bytes.Buffer
			stdin, stdout, read := streams(t)
			if got := run(context.Background(), tt.args, os.LookupEnv, stdin, stdout, &errOut); got != tt.wantStatus {
				t.Errorf("status = %d, want %d", got, tt.wantStatus)
			}
			if out := read(); out != tt.wantOut {
				t.Errorf("stdout = %q, want %q", out, tt.wantOut)
			}
			if errOut.String() != tt.wantErr {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.wantErr)
			}
		})
	}
}

func TestRunWithClosedStreams(t *testing.T) {
	stdin, stdout, _ := streams(t)
	readOnly, err := os.Open(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = readOnly.Close() }() // only the test's file
	for _, args := range [][]string{{"--version"}, {"--help"}} {
		if got := run(context.Background(), args, os.LookupEnv, stdin, readOnly, failingWriter{}); got != 125 {
			t.Errorf("%v: status = %d, want 125", args, got)
		}
	}
}

func TestCheckTerminal(t *testing.T) {
	stdin, stdout, _ := streams(t)
	errtest.Route(t, checkTerminal(stdin, stdout), "checkTerminal.TubeNotATerminal")
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Only the test's own terminal.
		_ = master.Close()
		_ = slave.Close()
	}()
	if err := checkTerminal(slave, slave); err != nil {
		t.Errorf("a terminal is not one: %v", err)
	}
	errtest.Route(t, checkTerminal(slave, stdout), "checkTerminal.TubeNotATerminal")
}
