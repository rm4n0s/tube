// SPDX-License-Identifier: GPL-2.0-only

package storagetest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// inside says whether path is root or below it.
func inside(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestIsolate(t *testing.T) {
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("the test needs to know the real home to prove it is left alone: %v", err)
	}
	before := filepath.Join(realHome, "before")
	t.Setenv("TUBE_CONFIG_DIR", before)
	var dirs struct{ config, state, tempBase string }
	t.Run("isolated", func(t *testing.T) {
		d := Isolate(t)
		dirs.config, dirs.state, dirs.tempBase = d.Config, d.State, d.TempBase

		for _, dir := range []string{d.Config, d.State, d.TempBase} {
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				t.Errorf("%s was not created: %v", dir, err)
			}
			if inside(dir, realHome) {
				t.Errorf("%s is inside the real home %s", dir, realHome)
			}
		}
		// No variable that names a folder of the user still points into the real home.
		for _, name := range []string{
			"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
			"XDG_RUNTIME_DIR", "TUBE_CONFIG_DIR", "TUBE_STATE_DIR",
		} {
			value, ok := os.LookupEnv(name)
			if !ok || value == "" {
				t.Errorf("%s is not set", name)
				continue
			}
			if inside(value, realHome) {
				t.Errorf("%s=%s points into the real home", name, value)
			}
		}
		if home, err := os.UserHomeDir(); err != nil || inside(home, realHome) {
			t.Errorf("os.UserHomeDir() = %q, %v: still the real home", home, err)
		}
	})
	// After the isolated test: its folders are gone and the environment is back.
	for _, dir := range []string{dirs.config, dirs.state, dirs.tempBase} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s is still there after the test: %v", dir, err)
		}
	}
	if got := os.Getenv("TUBE_CONFIG_DIR"); got != before {
		t.Errorf("TUBE_CONFIG_DIR was not restored: %q", got)
	}
}
