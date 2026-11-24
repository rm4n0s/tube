// SPDX-License-Identifier: GPL-2.0-only

// Package storagetest keeps tests away from the user's own files (docs/SPEC.md §16.1).
package storagetest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/tube/internal/core/storage"
)

// Isolate points Tube's folders, the home folder and the XDG folders at temporary folders that
// the test owns, and returns Tube's folders, created. The environment is restored and the
// folders are removed when the test ends. It cannot be used in a parallel test, because it
// changes the environment.
func Isolate(t testing.TB) storage.Dirs {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	runtime := filepath.Join(root, "run")
	for _, dir := range []string{home, runtime} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("storagetest: %v", err)
		}
	}
	for name, value := range map[string]string{
		"HOME":               home,
		"XDG_CONFIG_HOME":    filepath.Join(root, "xdg-config"),
		"XDG_STATE_HOME":     filepath.Join(root, "xdg-state"),
		"XDG_CACHE_HOME":     filepath.Join(root, "xdg-cache"),
		"XDG_DATA_HOME":      filepath.Join(root, "xdg-data"),
		"XDG_RUNTIME_DIR":    runtime,
		storage.EnvConfigDir: filepath.Join(root, "config"),
		storage.EnvStateDir:  filepath.Join(root, "state"),
	} {
		t.Setenv(name, value)
	}
	dirs, err := storage.Resolve(os.LookupEnv)
	if err != nil {
		t.Fatalf("storagetest: %v", err)
	}
	if err := dirs.Ensure(context.Background()); err != nil {
		t.Fatalf("storagetest: %v", err)
	}
	return dirs
}
