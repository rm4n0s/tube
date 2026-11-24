// SPDX-License-Identifier: GPL-2.0-only

package archtest

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

// moduleRoot returns the root of Tube's module, two folders above this package.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// fixture returns the path of a fixture tree.
func fixture(name string) string {
	return filepath.Join("testdata", name)
}

func TestModuleKeepsTheBoundaries(t *testing.T) {
	violations, err := CheckBoundaries(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Error(v.String())
	}
}

func TestCheckBoundariesReportsEachRule(t *testing.T) {
	violations, err := CheckBoundaries(fixture("boundaries"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range violations {
		if v.Line <= 0 {
			t.Errorf("%s has no line", v.String())
		}
		got = append(got, "rule "+strconv.Itoa(v.Rule)+" "+v.File+": "+v.Message)
	}
	want := []string{
		// cmd/tube may import the UI and the application, not gopenvpn-conn, and may not start programs
		"rule 5 cmd/tube/main.go: os/exec may be imported only under internal/core/sandbox and internal/tui, and by tests",
		"rule 3 cmd/tube/main.go: only internal/core/vpn and internal/core/profiles may import github.com/rm4n0s/gopenvpn-conn/ovpn",
		// rule 9
		"rule 9 go.mod: the dependency github.com/evil/dependency is not one CLAUDE.md allows",
		"rule 9 go.mod: the dependency github.com/evil/single is not one CLAUDE.md allows",
		// rule 1
		"rule 1 internal/app/bad.go: internal/app imports charm.land/lipgloss/v2",
		"rule 1 internal/app/bad.go: internal/app imports github.com/rm4n0s/tube/internal/tui",
		"rule 1 internal/core/bad/imports.go: internal/core imports charm.land/bubbletea/v2",
		"rule 1 internal/core/bad/imports.go: internal/core imports github.com/rm4n0s/tube/internal/app",
		"rule 1 internal/core/bad/imports.go: internal/core imports github.com/rm4n0s/tube/internal/tui/widgets",
		// rule 8
		`rule 8 internal/core/cgo/cgo.go: import "C": Tube is cgo-free`,
		// rule 4: the import in the test file counts, its fmt.Errorf does not
		"rule 4 internal/core/errs/errs.go: the standard library errors package is imported; use github.com/rm4n0s/errors",
		"rule 4 internal/core/errs/errs.go: fmt.Errorf is used; use github.com/rm4n0s/errors",
		"rule 4 internal/core/errs/errs_test.go: the standard library errors package is imported; use github.com/rm4n0s/errors",
		// rule 11: not in the test file
		"rule 11 internal/core/insecure/insecure.go: InsecureSkipVerify outside tests",
		"rule 11 internal/core/insecure/insecure.go: InsecureSkipVerify outside tests",
		// rule 10: a line with two invisible characters is reported once
		"rule 10 internal/core/invisible/invisible.go: an invisible character (U+202E); write it as an escape",
		"rule 10 internal/core/invisible/invisible.go: an invisible character (U+200B); write it as an escape",
		"rule 10 internal/core/invisible/invisible.go: an invisible character (U+1B); write it as an escape",
		// rule 6: no SPDX line, and one that is not the first line
		"rule 6 internal/core/nospdx/late.go: the first line is not // SPDX-License-Identifier: GPL-2.0-only",
		"rule 6 internal/core/nospdx/nospdx.go: the first line is not // SPDX-License-Identifier: GPL-2.0-only",
		// rules 3 and 5: the test file may start programs, but it may not import gopenvpn-conn
		"rule 5 internal/core/session/bad.go: os/exec may be imported only under internal/core/sandbox and internal/tui, and by tests",
		"rule 3 internal/core/session/bad.go: only internal/core/vpn and internal/core/profiles may import github.com/rm4n0s/gopenvpn-conn",
		"rule 3 internal/core/session/session_test.go: only internal/core/vpn and internal/core/profiles may import github.com/rm4n0s/gopenvpn-conn/ovpn",
		// rule 3: nothing imports vpnnet
		"rule 3 internal/core/vpn/vpn.go: vpnnet of gopenvpn-conn is not used: github.com/rm4n0s/gopenvpn-conn/vpnnet",
		// rule 2
		"rule 2 internal/other/bad.go: Bubble Tea and Lip Gloss may be imported only under internal/tui and cmd/tube, not charm.land/lipgloss/v2 here",
		"rule 2 internal/other/bad.go: only cmd/tube may import github.com/rm4n0s/tube/internal/tui",
		"rule 2 internal/tui/tui.go: internal/tui imports github.com/rm4n0s/tube/internal/core/profiles; of the core it may import only clock and sanitize",
	}
	sortedGot := append([]string(nil), got...)
	sort.Strings(sortedGot)
	sort.Strings(want)
	if strings.Join(sortedGot, "\n") != strings.Join(want, "\n") {
		t.Errorf("violations:\n%s\n\nwant:\n%s", strings.Join(sortedGot, "\n"), strings.Join(want, "\n"))
	}
	// The result is sorted by file, then line.
	for i := 1; i < len(violations); i++ {
		a, b := violations[i-1], violations[i]
		if a.File > b.File || (a.File == b.File && a.Line > b.Line) {
			t.Errorf("not sorted: %s before %s", a.String(), b.String())
		}
	}
}

func TestViolationString(t *testing.T) {
	v := Violation{Rule: 4, File: "internal/x/x.go", Line: 12, Message: "fmt.Errorf is used"}
	if got, want := v.String(), "internal/x/x.go:12: rule 4: fmt.Errorf is used"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// unparsableTree copies the unparsable fixture into a temporary folder, giving its broken file
// the .go name that the repository's own tools must not see.
func unparsableTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []struct{ from, to string }{
		{"go.mod", "go.mod"},
		{filepath.Join("internal", "x", "x.go.txt"), filepath.Join("internal", "x", "x.go")},
	} {
		data, err := os.ReadFile(filepath.Join(fixture("unparsable"), f.from))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f.to)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f.to), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// unreadableTree makes a module with a source folder that cannot be listed.
func unreadableTree(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can list any folder")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/unreadable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(root, "internal", "locked")
	if err := os.MkdirAll(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o700); err != nil {
			t.Error(err)
		}
	})
	return root
}

// unreadableFileTree makes a module with a Go file that cannot be read.
func unreadableFileTree(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/unreadable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "x", "x.go"), []byte("package x\n"), 0); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSourceErrors(t *testing.T) {
	tests := []struct {
		name      string
		root      func(t *testing.T) string
		wantRoute string
	}{
		{"no go.mod", func(t *testing.T) string { return t.TempDir() }, "readGoMod.ArchtestGoModUnreadable"},
		{"go.mod without a module line", func(*testing.T) string { return fixture("nomodule") }, "readGoMod.ArchtestGoModWithoutModule"},
		{"a file that does not parse", unparsableTree, "loadSources.func1.ArchtestSourceParseFailed"},
		{"a file that cannot be read", unreadableFileTree, "loadSources.func1.ArchtestSourceReadFailed"},
		{"a folder that cannot be listed", unreadableTree, "loadSources.func1.ArchtestSourceWalkFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root(t)
			checks := map[string]func() error{
				"CheckBoundaries": func() error { _, err := CheckBoundaries(root); return err },
				"ScanTags":        func() error { _, err := ScanTags(root); return err },
			}
			for name, check := range checks {
				err := check()
				appErr, ok := errors.FromError(err)
				if !ok {
					t.Fatalf("%s: expected *errors.Error, got %T: %v", name, err, err)
				}
				// The walk's own frames (fs.WalkDir) are in the route between loadSources and its callback.
				if !appErr.HasRoute(name+"->") || !appErr.HasRoute(tt.wantRoute) {
					t.Errorf("%s: unexpected failure path: %s", name, appErr.Route())
				}
			}
		})
	}
}
