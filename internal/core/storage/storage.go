// SPDX-License-Identifier: GPL-2.0-only

// Package storage is where Tube keeps its files: it resolves the configuration and state
// folders, makes folders that only the user can open, writes files atomically, reads them with a
// size limit, and makes the temporary folder of a run (docs/SPEC.md §5).
package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/rm4n0s/errors"
)

// The Tags of this package's errors.
const (
	TagDirNotAbsolute     = "StorageDirNotAbsolute"
	TagHomeUnknown        = "StorageHomeUnknown"
	TagEnsureCancelled    = "StorageEnsureCancelled"
	TagDirCreateFailed    = "StorageDirCreateFailed"
	TagDirStatFailed      = "StorageDirStatFailed"
	TagNotADir            = "StorageNotADir"
	TagDirNarrowFailed    = "StorageDirNarrowFailed"
	TagWriteCancelled     = "StorageWriteCancelled"
	TagWriteCreateFailed  = "StorageWriteCreateFailed"
	TagWriteFailed        = "StorageWriteFailed"
	TagWriteSyncFailed    = "StorageWriteSyncFailed"
	TagWriteCloseFailed   = "StorageWriteCloseFailed"
	TagWriteRenameFailed  = "StorageWriteRenameFailed"
	TagWriteDirSyncFailed = "StorageWriteDirSyncFailed"
	TagReadCancelled      = "StorageReadCancelled"
	TagFileNotExist       = "StorageFileNotExist"
	TagFileOpenFailed     = "StorageFileOpenFailed"
	TagFileReadFailed     = "StorageFileReadFailed"
	TagFileTooLarge       = "FileTooLarge"
	TagTempDirCancelled   = "StorageTempDirCancelled"
	TagTempDirFailed      = "StorageTempDirFailed"
)

// ErrNotExist matches, with errors.Is, the error ReadFile returns for a file that does not exist.
var ErrNotExist = &errors.Error{Tag: TagFileNotExist}

// The names of the environment variables that replace the folders (docs/SPEC.md §5).
const (
	EnvConfigDir = "TUBE_CONFIG_DIR"
	EnvStateDir  = "TUBE_STATE_DIR"
)

// The modes of everything Tube makes (docs/SPEC.md §5): only the user can open it.
const (
	// DirMode is the mode of a folder.
	DirMode os.FileMode = 0o700
	// FileMode is the mode of a file.
	FileMode os.FileMode = 0o600
)

// Dirs are the folders Tube uses.
type Dirs struct {
	// Config holds the profiles.
	Config string
	// State holds the log.
	State string
	// TempBase is where the temporary folder of a run is made: the user's runtime folder, or
	// empty for the system's temporary folder.
	TempBase string
}

// Env looks up an environment variable, as os.LookupEnv does.
type Env func(name string) (string, bool)

// Resolve decides the folders by docs/SPEC.md §5: a TUBE_ variable wins over the XDG variable,
// which wins over the place under the home folder. A TUBE_ variable must be an absolute path; an
// XDG variable that is empty or not an absolute path is ignored, as the XDG specification says.
// It creates nothing.
func Resolve(env Env) (Dirs, error) {
	lookup := func(name string) string {
		value, _ := env(name)
		return value
	}
	absolute := func(name string) string {
		if dir := lookup(name); filepath.IsAbs(dir) {
			return dir
		}
		return ""
	}
	config, state := lookup(EnvConfigDir), lookup(EnvStateDir)
	for _, dir := range []string{config, state} {
		if dir != "" && !filepath.IsAbs(dir) {
			return Dirs{}, errors.New(TagDirNotAbsolute, "a folder given by a TUBE_ variable must be an absolute path", "dir", dir)
		}
	}
	home := absolute("HOME")
	if config == "" {
		switch dir := absolute("XDG_CONFIG_HOME"); {
		case dir != "":
			config = filepath.Join(dir, "tube")
		case home != "":
			config = filepath.Join(home, ".config", "tube")
		}
	}
	if state == "" {
		switch dir := absolute("XDG_STATE_HOME"); {
		case dir != "":
			state = filepath.Join(dir, "tube")
		case home != "":
			state = filepath.Join(home, ".local", "state", "tube")
		}
	}
	if config == "" || state == "" {
		return Dirs{}, errors.New(TagHomeUnknown, "HOME is not set, so Tube does not know where to keep its files; set "+EnvConfigDir+" and "+EnvStateDir)
	}
	return Dirs{Config: config, State: state, TempBase: absolute("XDG_RUNTIME_DIR")}, nil
}

// Ensure makes the configuration and the state folder (EnsureDir).
func (d Dirs) Ensure(ctx context.Context) error {
	for _, dir := range []string{d.Config, d.State} {
		if err := EnsureDir(ctx, dir); err != nil {
			return err
		}
	}
	return nil
}

// EnsureDir makes the folder, and the folders above it that do not exist, with mode 0700
// whatever the umask is, and narrows a folder that exists with wider rights to 0700
// (docs/SPEC.md §5).
func EnsureDir(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return errors.NewErr(TagEnsureCancelled, err).SetMetadata("dir", dir)
	}
	// The folders that do not exist, from the innermost up.
	var missing []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err == nil || filepath.Dir(d) == d {
			break
		}
		missing = append(missing, d)
	}
	// Each folder is made and set to 0700 at once, from the outermost down: the mode of Mkdir
	// is cut by the umask, and a parent cut to 000 could not hold its child. A folder that
	// existed keeps the mode it had, so it is narrowed.
	paths := []string{dir}
	if len(missing) > 0 {
		paths = paths[:0]
		for _, p := range slices.Backward(missing) {
			paths = append(paths, p)
		}
	}
	for _, p := range paths {
		if len(missing) > 0 {
			if err := os.Mkdir(p, DirMode); err != nil && !os.IsExist(err) {
				return errors.NewErr(TagDirCreateFailed, err).SetMetadata("dir", dir)
			}
		}
		info, err := os.Stat(p)
		if err != nil {
			return errors.NewErr(TagDirStatFailed, err).SetMetadata("dir", dir)
		}
		if !info.IsDir() {
			return errors.New(TagNotADir, "the path is not a folder", "dir", p)
		}
		if info.Mode().Perm() != DirMode {
			if err := os.Chmod(p, DirMode); err != nil {
				return errors.NewErr(TagDirNarrowFailed, err).SetMetadata("dir", dir)
			}
		}
	}
	return nil
}

// NewTempDir makes the temporary folder of one run, with mode 0700, under the user's runtime
// folder or the system's temporary folder (docs/SPEC.md §5). The caller removes it.
func (d Dirs) NewTempDir(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.NewErr(TagTempDirCancelled, err)
	}
	dir, err := os.MkdirTemp(d.TempBase, "tube-")
	if err != nil {
		return "", errors.NewErr(TagTempDirFailed, err).SetMetadata("base", d.TempBase)
	}
	return dir, nil
}

// atomicFS is the part of the file system WriteFileAtomic uses, so that tests can make each step
// fail.
type atomicFS interface {
	// CreateTemp creates a new file in dir with mode 0600, as os.CreateTemp does.
	CreateTemp(dir, pattern string) (tempFile, error)
	// Rename moves a file over another, as os.Rename does.
	Rename(from, to string) error
	// Remove deletes a file.
	Remove(name string) error
	// SyncDir makes a folder's entries durable.
	SyncDir(dir string) error
}

// tempFile is the part of *os.File WriteFileAtomic uses.
type tempFile interface {
	io.Writer
	Sync() error
	Close() error
	Name() string
}

// osFS is the real file system.
type osFS struct{}

func (osFS) CreateTemp(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
func (osFS) Rename(from, to string) error                     { return os.Rename(from, to) }
func (osFS) Remove(name string) error                         { return os.Remove(name) }
func (osFS) SyncDir(dir string) error                         { return syncDir(dir) }

// syncDir makes the entries of a folder durable.
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close() // the sync's error is the one that matters
		return err
	}
	return f.Close()
}

// WriteFileAtomic replaces the file at path with data so that a reader, or a crash, sees the old
// content or the new, never a part: it writes a temporary file with mode 0600 in the same
// folder, makes it durable, and renames it over path (docs/SPEC.md §5). When it fails, the old
// file is untouched and the temporary file is removed.
func WriteFileAtomic(ctx context.Context, path string, data []byte) error {
	return writeFileAtomic(ctx, osFS{}, path, data)
}

// writeFileAtomic is WriteFileAtomic over a given file system.
func writeFileAtomic(ctx context.Context, fsys atomicFS, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return errors.NewErr(TagWriteCancelled, err).SetMetadata("path", path)
	}
	dir := filepath.Dir(path)
	tmp, err := fsys.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.NewErr(TagWriteCreateFailed, err).SetMetadata("path", path)
	}
	// From here on, a failure removes the temporary file. The removal is best effort: its own
	// failure would hide the error that matters, and leaves only a hidden temporary file.
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = fsys.Remove(tmp.Name())
		return errors.NewErr(TagWriteFailed, err).SetMetadata("path", path)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = fsys.Remove(tmp.Name())
		return errors.NewErr(TagWriteSyncFailed, err).SetMetadata("path", path)
	}
	if err := tmp.Close(); err != nil {
		_ = fsys.Remove(tmp.Name())
		return errors.NewErr(TagWriteCloseFailed, err).SetMetadata("path", path)
	}
	if err := fsys.Rename(tmp.Name(), path); err != nil {
		_ = fsys.Remove(tmp.Name())
		return errors.NewErr(TagWriteRenameFailed, err).SetMetadata("path", path)
	}
	if err := fsys.SyncDir(dir); err != nil {
		return errors.NewErr(TagWriteDirSyncFailed, err).SetMetadata("path", path)
	}
	return nil
}

// ReadFile reads the file at path when it is at most maxBytes long. A file that does not exist
// is reported with an error that errors.Is matches with ErrNotExist; a longer file with the Tag
// FileTooLarge.
func ReadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr(TagReadCancelled, err).SetMetadata("path", path)
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, errors.NewErr(TagFileNotExist, err).SetMetadata("path", path)
	}
	if err != nil {
		return nil, errors.NewErr(TagFileOpenFailed, err).SetMetadata("path", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		_ = f.Close() // the read's error is the one that matters
		return nil, errors.NewErr(TagFileReadFailed, err).SetMetadata("path", path)
	}
	_ = f.Close() // a file that was only read loses nothing when its close fails
	if int64(len(data)) > maxBytes {
		return nil, errors.New(TagFileTooLarge, "the file is larger than the limit", "path", path, "maxBytes", maxBytes)
	}
	return data, nil
}
