// SPDX-License-Identifier: GPL-2.0-only

// Package logging writes Tube's log: slog JSON lines in a file that is replaced when it grows
// past a limit (docs/SPEC.md §15). While a UI runs, the log goes nowhere else. The lines are JSON
// so that a logged error can be read back as an errors.ErrorJson and replayed in a test.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/rm4n0s/errors"
)

// The Tags of this package's errors.
const (
	TagOpenCancelled   = "LogOpenCancelled"
	TagOpenFailed      = "LogOpenFailed"
	TagWriteAfterClose = "LogWriteAfterClose"
	TagReopenFailed    = "LogReopenFailed"
	TagWriteFailed     = "LogWriteFailed"
	TagRotateFailed    = "LogRotateFailed"
	TagCloseFailed     = "LogCloseFailed"
)

// MaxBytes is the size a log file may reach; the write that would pass it starts a new file, and
// the full one becomes the file with ".1" after its name, replacing the one before.
const MaxBytes = 1 << 20

// Open opens the log file at path for appending, creating it with mode 0600, and returns a
// logger that writes the records at or above level to it, and the file's closer.
func Open(ctx context.Context, path string, level slog.Level) (*slog.Logger, io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, errors.NewErr(TagOpenCancelled, err).SetMetadata("path", path)
	}
	f, size, err := openAppend(path)
	if err != nil {
		return nil, nil, errors.NewErr(TagOpenFailed, err).SetMetadata("path", path)
	}
	w := &rotatingFile{path: path, max: MaxBytes, file: f, size: size}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})), w, nil
}

// openAppend opens a file for appending and tells its size. It returns the operating system's
// errors as they are.
func openAppend(path string) (*os.File, int64, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, 0, err
	}
	// A log that existed with wider rights is narrowed: it may hold the names of profiles.
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close() // nothing was written; the chmod's error is the one that matters
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close() // nothing was written; the stat's error is the one that matters
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// rotatingFile is a log file that starts over when it is full.
type rotatingFile struct {
	path string
	max  int64

	mu   sync.Mutex
	file *os.File // nil after Close, and after a rotation that could not open the new file
	size int64
	// closed says that Close was called.
	closed bool
}

// Write appends p, first starting a new file when p would take the file past its limit. When
// the full file cannot be put aside, p is appended to it all the same, and the error says so:
// a log that grows is better than a log that stops.
func (w *rotatingFile) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New(TagWriteAfterClose, "the log file is closed", "path", w.path)
	}
	var renameErr error
	if w.file == nil || (w.size > 0 && w.size+int64(len(p)) > w.max) {
		var reopenErr error
		renameErr, reopenErr = w.rotate()
		if reopenErr != nil {
			return 0, errors.NewErr(TagReopenFailed, reopenErr).SetMetadata("path", w.path)
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return n, errors.NewErr(TagWriteFailed, err).SetMetadata("path", w.path)
	}
	if renameErr != nil {
		return n, errors.NewErr(TagRotateFailed, renameErr).SetMetadata("path", w.path)
	}
	return n, nil
}

// rotate closes the full file, renames it to its ".1" name and opens a new one. When the rename
// fails, the file that is opened is the full one again. It returns the operating system's errors
// as they are: the rename's, and that of opening the file, after which there is no open file.
func (w *rotatingFile) rotate() (renameErr, reopenErr error) {
	if w.file != nil {
		// The records are written already; a failed close of a file that is put aside loses nothing more.
		_ = w.file.Close()
		w.file = nil
		renameErr = os.Rename(w.path, w.path+".1")
	}
	f, size, err := openAppend(w.path)
	if err != nil {
		return renameErr, err
	}
	w.file, w.size = f, size
	return renameErr, nil
}

// Close closes the log file. A second call does nothing.
func (w *rotatingFile) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	f := w.file
	w.file = nil
	if err := f.Close(); err != nil {
		return errors.NewErr(TagCloseFailed, err).SetMetadata("path", w.path)
	}
	return nil
}
