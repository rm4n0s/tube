// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/rm4n0s/tube/internal/app"
)

// failingWriter is a stream that cannot be written to.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantOut    string
		wantErr    string
	}{
		{"version", []string{"--version"}, statusOK, "tube " + app.Version() + "\n", ""},
		{"version with one dash", []string{"-version"}, statusOK, "tube " + app.Version() + "\n", ""},
		{"no arguments", nil, statusFailed, "", "tube: not implemented yet\n"},
		{"an application", []string{"ping", "-c", "1", "example.com"}, statusFailed, "", "tube: not implemented yet\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run(tt.args, &out, &errOut); got != tt.wantStatus {
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
	tests := []struct {
		name string
		args []string
	}{
		{"version", []string{"--version"}},
		{"no arguments", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(tt.args, failingWriter{}, failingWriter{}); got != statusFailed {
				t.Errorf("status = %d, want %d", got, statusFailed)
			}
		})
	}
}
