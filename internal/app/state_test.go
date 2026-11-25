// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/storage"
)

// addFolder is the folder of the last add, as the snapshot gives it.
func addFolder(t *testing.T, a *App) string {
	t.Helper()
	snap, err := a.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snap.AddFolder
}

func TestTheFolderOfTheLastAddIsRemembered(t *testing.T) {
	a, dirs := open(t)
	ctx := context.Background()
	if got := addFolder(t, a); got != "" {
		t.Fatalf("before an add the folder is %q", got)
	}
	first, second := t.TempDir(), t.TempDir()
	pki := profilestest.NewPKI(t)
	if _, err := a.AddProfile(ctx, profilestest.WriteProfile(t, first, "one.ovpn", pki, profilestest.Options{}), "", "", false); err != nil {
		t.Fatal(err)
	}
	if got := addFolder(t, a); got != first {
		t.Errorf("after an add the folder is %q, want %q", got, first)
	}
	// An add that fails changes nothing.
	if _, err := a.AddProfile(ctx, filepath.Join(second, "missing.ovpn"), "", "", false); err == nil {
		t.Fatal("a missing file was added")
	}
	if got := addFolder(t, a); got != first {
		t.Errorf("after a failed add the folder is %q, want %q", got, first)
	}
	// The file has the modes of every file of Tube, and no more than the folder.
	path := filepath.Join(dirs.State, stateFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != storage.FileMode {
		t.Errorf("the state file has the mode %v", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"version":1,"add_folder":"` + first + `"}` + "\n"; string(data) != want {
		t.Errorf("the state file is %q, want %q", data, want)
	}
	// A path under "~" is remembered as the folder that it is.
	home := os.Getenv("HOME")
	profilestest.WriteProfile(t, home, "two.ovpn", pki, profilestest.Options{})
	if _, err := a.AddProfile(ctx, "~/two.ovpn", "", "", false); err != nil {
		t.Fatal(err)
	}
	if got := addFolder(t, a); got != home {
		t.Errorf("after an add from ~ the folder is %q, want %q", got, home)
	}
	// The next run of Tube remembers it.
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(ctx, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := addFolder(t, again); got != home {
		t.Errorf("after a new start the folder is %q, want %q", got, home)
	}
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAStateFileThatIsNotTrusted(t *testing.T) {
	tests := []struct {
		name, content, want string
	}{
		{"a folder", `{"version":1,"add_folder":"/home/ada/Downloads"}`, "/home/ada/Downloads"},
		{"no file", "", ""},
		{"not JSON", `{"version":1,"add_folder":`, ""},
		{"a version that Tube does not know", `{"version":2,"add_folder":"/home/ada"}`, ""},
		{"a folder that is not absolute", `{"version":1,"add_folder":"Downloads"}`, ""},
		{"a folder that is not clean", `{"version":1,"add_folder":"/home/ada/../root"}`, ""},
		{"a folder with an escape sequence", `{"version":1,"add_folder":"/home/\u001b[31mred"}`, ""},
		{"a folder with a new line", `{"version":1,"add_folder":"/home/a\nb"}`, ""},
		{"a folder that is too long", `{"version":1,"add_folder":"/` + strings.Repeat("x", maxFolderLen) + `"}`, ""},
		{"a file that is too large", `{"version":1,"add_folder":"/home/ada","more":"` + strings.Repeat("x", maxStateBytes) + `"}`, ""},
		{"no folder", `{"version":1}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, dirs := open(t)
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			if tt.content != "" {
				if err := os.WriteFile(filepath.Join(dirs.State, stateFile), []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			a, err := Open(context.Background(), Options{})
			if err != nil {
				t.Fatalf("Tube did not start: %v", err)
			}
			defer func() {
				if err := a.Close(); err != nil {
					t.Error(err)
				}
			}()
			if got := addFolder(t, a); got != tt.want {
				t.Errorf("the folder is %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAStateFileThatCannotBeWritten(t *testing.T) {
	a, dirs := open(t)
	// A folder where the file belongs: the file cannot be put there, by any user.
	if err := os.Mkdir(filepath.Join(dirs.State, stateFile), 0o700); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := profilestest.WriteProfile(t, dir, "one.ovpn", profilestest.NewPKI(t), profilestest.Options{})
	if _, err := a.AddProfile(context.Background(), path, "", "", false); err != nil {
		t.Fatalf("the add failed with the state: %v", err)
	}
	// This run remembers it all the same, and the log says what happened.
	if got := addFolder(t, a); got != dir {
		t.Errorf("the folder is %q, want %q", got, dir)
	}
	if log := logText(t, dirs); !strings.Contains(log, "the folder of the added profile is not remembered") {
		t.Errorf("the log does not say that the state was not written:\n%s", log)
	}
}
