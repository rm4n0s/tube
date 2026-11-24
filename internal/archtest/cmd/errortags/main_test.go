// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

// failingWriter is a stream that cannot be written to.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// tagsFixture is the fixture module of internal/archtest with tested, untested, excepted and
// malformed Tags.
var tagsFixture = filepath.Join("..", "..", "testdata", "tags")

// cleanFixture is a fixture module without any Go file.
var cleanFixture = filepath.Join("..", "..", "testdata", "unparsable")

func TestRun(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		stdout       io.Writer
		stderr       io.Writer
		wantRoute    string
		wantOut      []string
		wantNotOut   []string
		wantProblems int
	}{
		{
			name:         "every Tag",
			args:         []string{tagsFixture},
			wantRoute:    "run.ErrortagsFound", // the fixture has malformed Tags
			wantOut:      []string{"Alpha\tinternal/a/a.go:", "\tStore.Save\n", "Untested\t", "Excepted\t", "ToolFailed\tcmd/tool/main.go:"},
			wantProblems: 5,
		},
		{
			name:         "untested",
			args:         []string{"--untested", tagsFixture},
			wantRoute:    "run.ErrortagsFound",
			wantOut:      []string{"Untested\t", "ToolFailed\t"},
			wantNotOut:   []string{"Alpha\t", "Excepted\t"},
			wantProblems: 5,
		},
		{
			name:         "untested, with the excepted ones",
			args:         []string{"--untested", "--all", tagsFixture},
			wantRoute:    "run.ErrortagsFound",
			wantOut:      []string{"Untested\t", "ToolFailed\t", "Excepted\t"},
			wantNotOut:   []string{"Alpha\t", "ExceptedButTested\t"},
			wantProblems: 5,
		},
		{name: "a clean module", args: []string{"--untested", cleanFixture}},
		{name: "an unknown flag", args: []string{"--nope", tagsFixture}, wantRoute: "run.ErrortagsUsage"},
		{name: "no root", args: []string{"--untested"}, wantRoute: "run.ErrortagsRootMissing"},
		{name: "two roots", args: []string{tagsFixture, cleanFixture}, wantRoute: "run.ErrortagsRootMissing"},
		{name: "a root that is not a module", args: []string{t.TempDir()}, wantRoute: "run->ScanTags->readGoMod.ArchtestGoModUnreadable"},
		{name: "a closed output", args: []string{tagsFixture}, stdout: failingWriter{}, wantRoute: "run.ErrortagsListWriteFailed", wantProblems: 5},
		{name: "a closed error output", args: []string{tagsFixture}, stderr: failingWriter{}, wantRoute: "run.ErrortagsProblemWriteFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			stdout, stderr := tt.stdout, tt.stderr
			if stdout == nil {
				stdout = &out
			}
			if stderr == nil {
				stderr = &errOut
			}
			err := run(tt.args, stdout, stderr)
			if tt.wantRoute == "" {
				if err != nil {
					t.Fatalf("run(%v) failed: %v", tt.args, err)
				}
			} else {
				appErr, ok := errors.FromError(err)
				if !ok {
					t.Fatalf("expected *errors.Error, got %T: %v", err, err)
				}
				if !appErr.HasRoute(tt.wantRoute) {
					t.Errorf("unexpected failure path: %s", appErr.Route())
				}
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout lacks %q:\n%s", want, out.String())
				}
			}
			for _, not := range tt.wantNotOut {
				if strings.Contains(out.String(), not) {
					t.Errorf("stdout has %q:\n%s", not, out.String())
				}
			}
			if tt.stderr == nil {
				if got := strings.Count(errOut.String(), "\n"); got != tt.wantProblems {
					t.Errorf("%d problems on stderr, want %d:\n%s", got, tt.wantProblems, errOut.String())
				}
			}
		})
	}
}
