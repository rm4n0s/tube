// SPDX-License-Identifier: GPL-2.0-only

package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

// wantRoute fails the test unless err is an *errors.Error whose route has the part, and returns
// the error's metadata.
func wantRoute(t *testing.T, err error, part string) map[string]any {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(part) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), part)
	}
	return appErr.Metadata
}

// read returns a file's content.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// newFile opens a rotating file with a small limit.
func newFile(t *testing.T, path string, max int64) *rotatingFile {
	t.Helper()
	f, size, err := openAppend(path)
	if err != nil {
		t.Fatal(err)
	}
	w := &rotatingFile{path: path, max: max, file: f, size: size}
	t.Cleanup(func() {
		// A test may have broken the file on purpose; only a leak matters here.
		_ = w.Close()
	})
	return w
}

func TestOpenWritesRecordsAtTheLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tube.log")
	logger, closer, err := Open(context.Background(), path, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("hidden")
	logger.Info("server started", "port", 2222)
	logger.Error("listen failed", "err", errors.New("ListenFailed", "the port is taken", "address", ":2222"))
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Errorf("a second Close: %v", err)
	}

	got := read(t, path)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 2 {
		t.Fatalf("the log has %d lines, want 2 (the debug record is below the level):\n%s", len(lines), got)
	}
	var started struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
		Port  int    `json:"port"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &started); err != nil {
		t.Fatalf("the first line is not JSON: %v\n%s", err, lines[0])
	}
	if started.Level != "INFO" || started.Msg != "server started" || started.Port != 2222 {
		t.Errorf("the first record is %+v", started)
	}
	// A logged error can be read back and still knows its Tag, route and metadata.
	var failed struct {
		Level string           `json:"level"`
		Err   errors.ErrorJson `json:"err"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &failed); err != nil {
		t.Fatalf("the second line is not JSON: %v\n%s", err, lines[1])
	}
	if failed.Level != "ERROR" || failed.Err.Tag != "ListenFailed" || failed.Err.Metadata["address"] != ":2222" ||
		!failed.Err.HasRoute("TestOpenWritesRecordsAtTheLevel.ListenFailed") {
		t.Errorf("the logged error is %+v", failed)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}

	// Opening again appends.
	logger, closer, err = Open(context.Background(), path, slog.LevelDebug)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("second run")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	got = read(t, path)
	if !strings.Contains(got, "server started") || !strings.Contains(got, "second run") {
		t.Errorf("the second run did not append:\n%s", got)
	}
}

func TestOpenNarrowsAnExistingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tube.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, closer, err := Open(context.Background(), path, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	if got := read(t, path); got != "old\n" {
		t.Errorf("the old content is %q: opening must append, not replace", got)
	}
}

func TestOpenErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "tube.log")
	_, _, err := Open(ctx, path, slog.LevelInfo)
	wantRoute(t, err, "Open.LogOpenCancelled")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("the file was created")
	}

	missing := filepath.Join(t.TempDir(), "missing", "tube.log")
	_, _, err = Open(context.Background(), missing, slog.LevelInfo)
	if meta := wantRoute(t, err, "Open.LogOpenFailed"); meta["path"] != missing {
		t.Errorf("metadata = %v", meta)
	}
}

func TestRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tube.log")
	w := newFile(t, path, 10)
	write := func(s string) {
		t.Helper()
		if n, err := w.Write([]byte(s)); err != nil || n != len(s) {
			t.Fatalf("Write(%q) = %d, %v", s, n, err)
		}
	}

	write("12345")
	write("67890") // exactly the limit: no rotation
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatal("rotated at the limit, not past it")
	}
	write("a") // past the limit: the full file is put aside
	if got := read(t, path+".1"); got != "1234567890" {
		t.Errorf("the old file has %q", got)
	}
	if got := read(t, path); got != "a" {
		t.Errorf("the new file has %q", got)
	}
	write("bcdefghij") // the new file is now full
	write("k")         // the second rotation replaces the old file
	if got := read(t, path+".1"); got != "abcdefghij" {
		t.Errorf("after the second rotation the old file has %q", got)
	}
	if got := read(t, path); got != "k" {
		t.Errorf("after the second rotation the new file has %q", got)
	}

	// A record longer than the limit goes whole into a file of its own.
	write("this record is longer than the limit")
	if got := read(t, path); got != "this record is longer than the limit" {
		t.Errorf("the long record: %q", got)
	}
	if got := read(t, path+".1"); got != "k" {
		t.Errorf("the file before the long record: %q", got)
	}
}

func TestRotationCountsWhatTheFileAlreadyHad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tube.log")
	if err := os.WriteFile(path, []byte("12345678"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := newFile(t, path, 10)
	if _, err := w.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path+".1"); got != "12345678" {
		t.Errorf("the old file has %q", got)
	}
	if got := read(t, path); got != "abc" {
		t.Errorf("the new file has %q", got)
	}
}

func TestWriteErrors(t *testing.T) {
	t.Run("after Close", func(t *testing.T) {
		w := newFile(t, filepath.Join(t.TempDir(), "tube.log"), 10)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		_, err := w.Write([]byte("x"))
		wantRoute(t, err, "Write.LogWriteAfterClose")
	})
	t.Run("the file cannot be written", func(t *testing.T) {
		w := newFile(t, filepath.Join(t.TempDir(), "tube.log"), 10)
		if err := w.file.Close(); err != nil { // behind the writer's back
			t.Fatal(err)
		}
		_, err := w.Write([]byte("x"))
		wantRoute(t, err, "Write.LogWriteFailed")
	})
	t.Run("the full file cannot be put aside", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "tube.log")
		// A folder with something in it where the old file goes: the rename fails.
		if err := os.MkdirAll(filepath.Join(path+".1", "in-the-way"), 0o700); err != nil {
			t.Fatal(err)
		}
		w := newFile(t, path, 10)
		if _, err := w.Write([]byte("1234567890")); err != nil {
			t.Fatal(err)
		}
		n, err := w.Write([]byte("abc"))
		wantRoute(t, err, "Write.LogRotateFailed")
		if n != 3 {
			t.Errorf("n = %d, want 3: the record is written all the same", n)
		}
		if got := read(t, path); got != "1234567890abc" {
			t.Errorf("the file has %q, want the record appended to the full file", got)
		}
		// The writer keeps working.
		if _, err := w.Write([]byte("d")); err == nil {
			t.Error("the next write did not report that the rotation still fails")
		}
		if got := read(t, path); got != "1234567890abcd" {
			t.Errorf("the file has %q", got)
		}
	})
	t.Run("the new file cannot be opened", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "tube.log")
		w := newFile(t, path, 10)
		if _, err := w.Write([]byte("1234567890")); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		_, err := w.Write([]byte("abc"))
		wantRoute(t, err, "Write.LogReopenFailed")
		// When the folder is back, the next write opens the file again.
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("back")); err != nil {
			t.Fatalf("after the folder is back: %v", err)
		}
		if got := read(t, path); got != "back" {
			t.Errorf("the file has %q", got)
		}
	})
}

func TestCloseError(t *testing.T) {
	w := newFile(t, filepath.Join(t.TempDir(), "tube.log"), 10)
	if err := w.file.Close(); err != nil { // behind the writer's back
		t.Fatal(err)
	}
	wantRoute(t, w.Close(), "Close.LogCloseFailed")
	if err := w.Close(); err != nil {
		t.Errorf("a second Close: %v", err)
	}
}
