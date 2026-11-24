// SPDX-License-Identifier: GPL-2.0-only

package archtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

// formatUses formats uses one per line, as the errortags command prints them.
func formatUses(uses []TagUse) string {
	var b strings.Builder
	for _, use := range uses {
		fmt.Fprintf(&b, "%s %s %s\n", use.Tag, use.File, use.Func)
	}
	return b.String()
}

func TestModuleTagsAreTested(t *testing.T) {
	report, err := ScanTags(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range report.Problems {
		t.Errorf("%s:%d: %s", p.File, p.Line, p.Message)
	}
	for _, use := range report.Untested(false) {
		t.Errorf("%s:%d: no test asserts the Tag %s of %s", use.File, use.Line, use.Tag, use.Func)
	}
	for _, use := range report.Uses {
		if reason, ok := report.Excepted(use.Tag); ok && report.Asserted(use.Tag) {
			t.Errorf("%s lists the Tag %s (%s), but a test asserts it: remove it from the list", ExceptionsFile, use.Tag, reason)
		}
	}
}

func TestScanTags(t *testing.T) {
	report, err := ScanTags(fixture("tags"))
	if err != nil {
		t.Fatal(err)
	}

	wantUses := `Aliased internal/b/b.go Aliased
Alpha internal/a/a.go Store.Save
Compared internal/a/a.go Compared
Excepted internal/a/a.go Excepted
ExceptedButTested internal/a/a.go ExceptedButTested
InClosure internal/a/a.go Closure
Keyed internal/a/a.go Keyed
Literal internal/a/a.go Literal
Literal internal/a/a.go Twice
OpenFailed internal/b/open_linux.go Open
OpenFailed internal/b/open_windows.go Open
Private internal/a/a.go Store.Load
SeekFailed internal/b/open_linux.go Seek
SeekFailed internal/b/open_windows.go Tell
Sentinel internal/a/a.go Sentinel
ToolFailed cmd/tool/main.go main
Untested internal/a/a.go Untested
`
	if got := formatUses(report.Uses); got != wantUses {
		t.Errorf("uses:\n%s\nwant:\n%s", got, wantUses)
	}
	for _, use := range report.Uses {
		if use.Line <= 0 {
			t.Errorf("%s in %s has no line", use.Tag, use.File)
		}
	}

	wantUntested := "ToolFailed cmd/tool/main.go main\nUntested internal/a/a.go Untested\n"
	if got := formatUses(report.Untested(false)); got != wantUntested {
		t.Errorf("untested:\n%s\nwant:\n%s", got, wantUntested)
	}
	wantAll := "Excepted internal/a/a.go Excepted\n" + wantUntested
	if got := formatUses(report.Untested(true)); got != wantAll {
		t.Errorf("untested, with the excepted ones:\n%s\nwant:\n%s", got, wantAll)
	}

	if reason, ok := report.Excepted("Excepted"); !ok || reason != "it needs a full disk, which a test cannot make" {
		t.Errorf("Excepted(Excepted) = %q, %v", reason, ok)
	}
	if _, ok := report.Excepted("Untested"); ok {
		t.Error("Untested is not in the exceptions file")
	}
	if !report.Asserted("ExceptedButTested") || report.Asserted("Excepted") {
		t.Error("Asserted is wrong for the excepted Tags")
	}

	var problems []string
	for _, p := range report.Problems {
		if p.Line <= 0 {
			t.Errorf("the problem %q has no line", p.Message)
		}
		message := p.Message
		// The duplicate's message names the line of the first use; the line is not what is tested.
		if i := strings.Index(message, ".go:"); i >= 0 {
			j := strings.Index(message[i:], ";")
			message = message[:i+len(".go")] + message[i+j:]
		}
		problems = append(problems, p.File+": "+message)
	}
	wantProblems := []string{
		"internal/a/a.go: errors.New outside a function: its route would be init; a sentinel is &errors.Error{Tag: ...}",
		"internal/a/a.go: the Tag of errors.New is not a string literal or a Tag constant",
		"internal/a/a.go: the Tag of errors.New is not a string literal or a Tag constant",
		"internal/a/a.go: the Tag Literal is already created at internal/a/a.go; every failure case has its own Tag",
		"internal/b/open_windows.go: the Tag SeekFailed is already created at internal/b/open_linux.go; every failure case has its own Tag",
	}
	if strings.Join(problems, "\n") != strings.Join(wantProblems, "\n") {
		t.Errorf("problems:\n%s\n\nwant:\n%s", strings.Join(problems, "\n"), strings.Join(wantProblems, "\n"))
	}
}

func TestPlatformOf(t *testing.T) {
	tests := []struct{ file, want string }{
		{"internal/core/pty/pty_linux.go", "_linux"},
		{"internal/core/pty/pty_windows.go", "_windows"},
		{"internal/core/pty/pty_darwin.go", "_darwin"},
		{"internal/core/pty/pty_unix.go", "_unix"},
		{"internal/core/pty/pty_other.go", "_other"},
		{"internal/core/pty/pty.go", ""},
		{"internal/core/pty/linux.go", ""},
		{"internal/core/windows_helpers/x.go", ""},
	}
	for _, tt := range tests {
		if got := platformOf(tt.file); got != tt.want {
			t.Errorf("platformOf(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

func TestExceptionsErrors(t *testing.T) {
	tests := []struct {
		name      string
		root      func(t *testing.T) string
		wantRoute string
		wantMeta  map[string]any
	}{
		{
			name:      "an exception without a reason",
			root:      func(*testing.T) string { return fixture("badexception") },
			wantRoute: "ScanTags->readExceptions.ErrortagsExceptionMalformed",
			wantMeta:  map[string]any{"tag": "NoReason", "line": 2, "file": ExceptionsFile},
		},
		{
			name: "an exceptions file that cannot be read",
			root: func(t *testing.T) string {
				root := t.TempDir()
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/x\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				// A folder where the file should be: reading it fails, and not with "does not exist".
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(ExceptionsFile)), 0o700); err != nil {
					t.Fatal(err)
				}
				return root
			},
			wantRoute: "ScanTags->readExceptions.ErrortagsExceptionsUnreadable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ScanTags(tt.root(t))
			appErr, ok := errors.FromError(err)
			if !ok {
				t.Fatalf("expected *errors.Error, got %T: %v", err, err)
			}
			if !appErr.HasRoute(tt.wantRoute) {
				t.Errorf("unexpected failure path: %s", appErr.Route())
			}
			for key, want := range tt.wantMeta {
				if appErr.Metadata[key] != want {
					t.Errorf("metadata[%q] = %v, want %v", key, appErr.Metadata[key], want)
				}
			}
		})
	}
}

func TestMissingExceptionsFileMeansNone(t *testing.T) {
	report, err := ScanTags(fixture("unparsable"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Uses) != 0 || len(report.Problems) != 0 {
		t.Errorf("a module without Go files has uses or problems: %+v", report)
	}
}
