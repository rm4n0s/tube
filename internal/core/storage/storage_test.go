// SPDX-License-Identifier: GPL-2.0-only

package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/rm4n0s/errors"
)

// wantRoute fails the test unless err is an *errors.Error whose route has every given part, and
// returns the error's metadata.
func wantRoute(t *testing.T, err error, parts ...string) map[string]any {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	for _, part := range parts {
		if !appErr.HasRoute(part) {
			t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), part)
		}
	}
	return appErr.Metadata
}

// cancelled returns a context that is already cancelled.
func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// checkMode fails the test unless the file or folder has the mode, on platforms with mode bits.
func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode of %s = %o, want %o", path, got, want)
	}
}

// envOf makes an environment out of a map.
func envOf(vars map[string]string) Env {
	return func(name string) (string, bool) {
		value, ok := vars[name]
		return value, ok
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		want      Dirs
		wantRoute string
	}{
		{
			name: "defaults under the home",
			env:  map[string]string{"HOME": "/home/ada"},
			want: Dirs{Config: "/home/ada/.config/tube", State: "/home/ada/.local/state/tube"},
		},
		{
			name: "the XDG folders",
			env: map[string]string{"HOME": "/home/ada", "XDG_CONFIG_HOME": "/x/config", "XDG_STATE_HOME": "/x/state",
				"XDG_RUNTIME_DIR": "/run/user/1000"},
			want: Dirs{Config: "/x/config/tube", State: "/x/state/tube", TempBase: "/run/user/1000"},
		},
		{
			name: "relative and empty XDG folders are ignored",
			env: map[string]string{"HOME": "/home/ada", "XDG_CONFIG_HOME": "relative", "XDG_STATE_HOME": "",
				"XDG_RUNTIME_DIR": "run"},
			want: Dirs{Config: "/home/ada/.config/tube", State: "/home/ada/.local/state/tube"},
		},
		{
			name: "the TUBE_ variables win over XDG",
			env: map[string]string{"HOME": "/home/ada", "XDG_CONFIG_HOME": "/x/config", "XDG_STATE_HOME": "/x/state",
				"TUBE_CONFIG_DIR": "/c/config", "TUBE_STATE_DIR": "/c/state"},
			want: Dirs{Config: "/c/config", State: "/c/state"},
		},
		{
			name: "one TUBE_ variable, the other by default",
			env:  map[string]string{"HOME": "/home/ada", "TUBE_STATE_DIR": "/c/state"},
			want: Dirs{Config: "/home/ada/.config/tube", State: "/c/state"},
		},
		{
			name: "no home, but both folders given",
			env:  map[string]string{"TUBE_CONFIG_DIR": "/c/config", "TUBE_STATE_DIR": "/c/state"},
			want: Dirs{Config: "/c/config", State: "/c/state"},
		},
		{name: "no home", env: map[string]string{}, wantRoute: "Resolve.StorageHomeUnknown"},
		{name: "a relative home", env: map[string]string{"HOME": "home"}, wantRoute: "Resolve.StorageHomeUnknown"},
		{
			name:      "no home and only one folder given",
			env:       map[string]string{"TUBE_CONFIG_DIR": "/c/config"},
			wantRoute: "Resolve.StorageHomeUnknown",
		},
		{
			name:      "a relative TUBE_CONFIG_DIR",
			env:       map[string]string{"HOME": "/home/ada", "TUBE_CONFIG_DIR": "relative"},
			wantRoute: "Resolve.StorageDirNotAbsolute",
		},
		{
			name:      "a relative TUBE_STATE_DIR",
			env:       map[string]string{"HOME": "/home/ada", "TUBE_STATE_DIR": "relative/state"},
			wantRoute: "Resolve.StorageDirNotAbsolute",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(envOf(tt.env))
			if tt.wantRoute != "" {
				wantRoute(t, err, tt.wantRoute)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Resolve = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// notRoot skips a test that needs the mode bits to stop it.
func notRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root is not stopped by mode bits")
	}
}

func TestEnsure(t *testing.T) {
	t.Run("creates the folders with mode 0700", func(t *testing.T) {
		root := t.TempDir()
		dirs := Dirs{Config: filepath.Join(root, "a", "config"), State: filepath.Join(root, "b", "state")}
		if err := dirs.Ensure(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{dirs.Config, dirs.State, filepath.Join(root, "a")} {
			checkMode(t, dir, 0o700)
		}
		// A second call on folders that exist is fine.
		if err := dirs.Ensure(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("whatever the umask is", func(t *testing.T) {
		for _, mask := range []int{0, 0o022, 0o077, 0o777} {
			old := syscall.Umask(mask)
			dir := filepath.Join(t.TempDir(), "sub", "dir")
			err := EnsureDir(context.Background(), dir)
			syscall.Umask(old)
			if err != nil {
				t.Fatalf("umask %o: %v", mask, err)
			}
			checkMode(t, dir, 0o700)
		}
	})
	t.Run("narrows a folder that exists with wider rights", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "wide")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := EnsureDir(context.Background(), dir); err != nil {
			t.Fatal(err)
		}
		checkMode(t, dir, 0o700)
	})
	t.Run("a cancelled context", func(t *testing.T) {
		root := t.TempDir()
		dirs := Dirs{Config: filepath.Join(root, "config"), State: filepath.Join(root, "state")}
		err := dirs.Ensure(cancelled())
		wantRoute(t, err, "Ensure->EnsureDir.StorageEnsureCancelled")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("the cause is %v", errors.Unwrap(err))
		}
		if _, err := os.Stat(dirs.Config); !os.IsNotExist(err) {
			t.Error("the folder was created")
		}
	})
	t.Run("a file where a folder should be", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		dirs := Dirs{Config: filepath.Join(file, "config"), State: filepath.Join(root, "state")}
		meta := wantRoute(t, dirs.Ensure(context.Background()), "Ensure->EnsureDir.StorageDirCreateFailed")
		if meta["dir"] != dirs.Config {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("a folder of somebody else cannot be narrowed", func(t *testing.T) {
		notRoot(t)
		// The root folder is not this user's: its mode cannot be changed, and nothing changes.
		wantRoute(t, EnsureDir(context.Background(), "/"), "EnsureDir.StorageDirNarrowFailed")
	})
}

func TestNewTempDir(t *testing.T) {
	t.Run("under the base", func(t *testing.T) {
		base := t.TempDir()
		dir, err := Dirs{TempBase: base}.NewTempDir(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(dir) != base || !strings.HasPrefix(filepath.Base(dir), "tube-") {
			t.Errorf("the folder is %s, want tube-* under %s", dir, base)
		}
		checkMode(t, dir, 0o700)
		other, err := Dirs{TempBase: base}.NewTempDir(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if other == dir {
			t.Error("two runs got the same folder")
		}
	})
	t.Run("under the system's temporary folder", func(t *testing.T) {
		t.Setenv("TMPDIR", t.TempDir())
		dir, err := Dirs{}.NewTempDir(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(dir) != os.TempDir() {
			t.Errorf("the folder is %s, want one under %s", dir, os.TempDir())
		}
	})
	t.Run("a cancelled context", func(t *testing.T) {
		_, err := Dirs{TempBase: t.TempDir()}.NewTempDir(cancelled())
		wantRoute(t, err, "NewTempDir.StorageTempDirCancelled")
	})
	t.Run("a base that does not exist", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), "missing")
		meta := wantRoute(t, func() error { _, err := Dirs{TempBase: base}.NewTempDir(context.Background()); return err }(),
			"NewTempDir.StorageTempDirFailed")
		if meta["base"] != base {
			t.Errorf("metadata = %v", meta)
		}
	})
}

// failingFS is the real file system with steps that fail on demand.
type failingFS struct {
	osFS
	failWrite, failSync, failClose, failRename, failSyncDir bool
	// removed collects the names given to Remove.
	removed []string
}

// failingFile is a temporary file whose steps fail on demand.
type failingFile struct {
	tempFile
	fs *failingFS
}

var errInjected = &errors.Error{Tag: "Injected", Message: "injected failure"}

func (f *failingFS) CreateTemp(dir, pattern string) (tempFile, error) {
	file, err := f.osFS.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &failingFile{tempFile: file, fs: f}, nil
}

func (f *failingFS) Rename(from, to string) error {
	if f.failRename {
		return errInjected
	}
	return f.osFS.Rename(from, to)
}

func (f *failingFS) Remove(name string) error {
	f.removed = append(f.removed, name)
	return f.osFS.Remove(name)
}

func (f *failingFS) SyncDir(dir string) error {
	if f.failSyncDir {
		return errInjected
	}
	return f.osFS.SyncDir(dir)
}

func (f *failingFile) Write(p []byte) (int, error) {
	if f.fs.failWrite {
		return 0, errInjected
	}
	return f.tempFile.Write(p)
}

func (f *failingFile) Sync() error {
	if f.fs.failSync {
		return errInjected
	}
	return f.tempFile.Sync()
}

func (f *failingFile) Close() error {
	err := f.tempFile.Close()
	if f.fs.failClose {
		return errInjected
	}
	return err
}

// entries lists the names in a folder.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestWriteFileAtomic(t *testing.T) {
	ctx := context.Background()
	t.Run("writes a new file with mode 0600 and replaces it", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "people.json")
		for _, content := range []string{"first", "second, longer", ""} {
			if err := WriteFileAtomic(ctx, path, []byte(content)); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Errorf("content = %q, want %q", got, content)
			}
			checkMode(t, path, 0o600)
		}
		if names := entries(t, dir); len(names) != 1 || names[0] != "people.json" {
			t.Errorf("the folder has %v, want only people.json", names)
		}
	})
	t.Run("a cancelled context", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.json")
		err := WriteFileAtomic(cancelled(), path, []byte("x"))
		wantRoute(t, err, "WriteFileAtomic->writeFileAtomic.StorageWriteCancelled")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("the file was written")
		}
	})
	t.Run("a folder that does not exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "x.json")
		meta := wantRoute(t, WriteFileAtomic(ctx, path, []byte("x")), "WriteFileAtomic->writeFileAtomic.StorageWriteCreateFailed")
		if meta["path"] != path {
			t.Errorf("metadata = %v", meta)
		}
	})

	failures := []struct {
		name      string
		fs        *failingFS
		wantRoute string
		// renamed says that the failure comes after the new content is in place.
		renamed bool
	}{
		{"the write fails", &failingFS{failWrite: true}, "writeFileAtomic.StorageWriteFailed", false},
		{"the sync fails", &failingFS{failSync: true}, "writeFileAtomic.StorageWriteSyncFailed", false},
		{"the close fails", &failingFS{failClose: true}, "writeFileAtomic.StorageWriteCloseFailed", false},
		{"the rename fails", &failingFS{failRename: true}, "writeFileAtomic.StorageWriteRenameFailed", false},
		{"the folder's sync fails", &failingFS{failSyncDir: true}, "writeFileAtomic.StorageWriteDirSyncFailed", true},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")
			if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := writeFileAtomic(ctx, tt.fs, path, []byte("new"))
			wantRoute(t, err, tt.wantRoute)
			if !errors.Is(err, errInjected) {
				t.Errorf("the cause is %v", errors.Unwrap(err))
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			want := "old"
			if tt.renamed {
				want = "new"
			}
			if string(got) != want {
				t.Errorf("content after the failure = %q, want %q", got, want)
			}
			if names := entries(t, dir); len(names) != 1 {
				t.Errorf("the folder has %v: the temporary file was left", names)
			}
			if !tt.renamed && len(tt.fs.removed) != 1 {
				t.Errorf("Remove was called %d times, want 1", len(tt.fs.removed))
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	content := []byte("0123456789")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("a file within the limit", func(t *testing.T) {
		for _, limit := range []int64{10, 11, 1 << 20} {
			got, err := ReadFile(ctx, path, limit)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, content) {
				t.Errorf("limit %d: content = %q", limit, got)
			}
		}
	})
	t.Run("a file over the limit", func(t *testing.T) {
		_, err := ReadFile(ctx, path, 9)
		meta := wantRoute(t, err, "ReadFile.FileTooLarge")
		if meta["path"] != path || meta["maxBytes"] != int64(9) {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("a file that does not exist", func(t *testing.T) {
		_, err := ReadFile(ctx, filepath.Join(dir, "missing.json"), 100)
		wantRoute(t, err, "ReadFile.StorageFileNotExist")
		if !errors.Is(err, ErrNotExist) {
			t.Error("errors.Is(err, ErrNotExist) is false")
		}
		if errors.Is(err, ErrAlreadyRunning) {
			t.Error("the error matches another sentinel")
		}
	})
	t.Run("a cancelled context", func(t *testing.T) {
		_, err := ReadFile(cancelled(), path, 100)
		wantRoute(t, err, "ReadFile.StorageReadCancelled")
	})
}


func TestReadFileErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("a file that cannot be opened", func(t *testing.T) {
		notRoot(t)
		path := filepath.Join(t.TempDir(), "secret.json")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		_, err := ReadFile(ctx, path, 100)
		wantRoute(t, err, "ReadFile.StorageFileOpenFailed")
	})
	t.Run("a folder instead of a file", func(t *testing.T) {
		_, err := ReadFile(ctx, t.TempDir(), 100)
		wantRoute(t, err, "ReadFile.StorageFileReadFailed")
	})
}

func TestSyncDir(t *testing.T) {
	if err := syncDir(t.TempDir()); err != nil {
		t.Errorf("syncDir of a folder: %v", err)
	}
	if err := syncDir(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Errorf("syncDir of a missing folder: %v", err)
	}
}
