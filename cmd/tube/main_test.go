// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/nstest"
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
		{"no arguments", nil, true, 125, "", "tube: " + noTUI + "\n"},
		{"no --profile", []string{"true"}, true, 125, "", "tube: " + noTUI + "\n"},
		{"a profile that is not there", []string{"--profile", "nothing", "true"}, true, 125, "",
			"tube: There is no profile called \"nothing\". (AppProfileNotFound)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.namespace {
				nstest.Require(t)
			}
			storagetest.Isolate(t)
			var out, errOut bytes.Buffer
			if got := run(context.Background(), tt.args, os.LookupEnv, &out, &errOut); got != tt.wantStatus {
				t.Errorf("status = %d, want %d", got, tt.wantStatus)
			}
			if out.String() != tt.wantOut {
				t.Errorf("stdout = %q, want %q", out.String(), tt.wantOut)
			}
			if errOut.String() != tt.wantErr {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.wantErr)
			}
		})
	}
}

func TestRunWithClosedStreams(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"--help"}} {
		if got := run(context.Background(), args, os.LookupEnv, failingWriter{}, failingWriter{}); got != 125 {
			t.Errorf("%v: status = %d, want 125", args, got)
		}
	}
}
