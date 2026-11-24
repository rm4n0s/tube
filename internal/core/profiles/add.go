// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/storage"
)

// AddOptions say how a profile is added.
type AddOptions struct {
	// WithoutScripts adds a profile that names scripts with those lines disabled.
	WithoutScripts bool
}

// TagProfileRunsScripts is the error of a profile that names scripts, added without
// AddOptions.WithoutScripts. Its metadata "options" names them.
const TagProfileRunsScripts = "ProfileRunsScripts"

// Add adds the profile at path under the name: it parses it, copies it and the files that it
// names into a new folder of the store, parses the copy again, and writes meta.json last
// (docs/SPEC.md §6.2). The original is only read. When Add fails, nothing is left in the store.
func (s *Store) Add(ctx context.Context, path, name string, opts AddOptions) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, errors.NewErr(TagProfileAddCancelled, err)
	}
	name, err := CleanName(name)
	if err != nil {
		return Profile{}, err
	}
	if err := s.nameFree(ctx, name, ""); err != nil {
		return Profile{}, err
	}
	if path == "" {
		return Profile{}, errors.New(TagProfilePathEmpty, "the path of the profile is empty")
	}
	dir, file := filepath.Dir(path), filepath.Base(path)
	text, names, disabled, err := withoutScripts(dir, file)
	if err != nil {
		return Profile{}, err
	}
	if len(disabled) > 0 && !opts.WithoutScripts {
		return Profile{}, errors.New(TagProfileRunsScripts, "the profile runs scripts", "options", strings.Join(disabled, ", "))
	}
	if s.afterLoad != nil {
		s.afterLoad()
	}
	id, err := newID()
	if err != nil {
		return Profile{}, err
	}
	folder := filepath.Join(s.dir, id)
	p, err := s.store(ctx, id, dir, file, text, names, name)
	if err != nil {
		// The error that matters is the first; a folder that is left is removed by a later
		// List, as a failed add.
		_ = os.RemoveAll(folder)
		return Profile{}, err
	}
	return p, nil
}

// store writes the text of the profile file, copies the files it names from dir into the new
// folder of id, checks the copy, and writes meta.json.
func (s *Store) store(ctx context.Context, id, dir, file string, text []byte, names []string, name string) (Profile, error) {
	files := filepath.Join(s.dir, id, filesDir)
	if err := storage.EnsureDir(ctx, files); err != nil {
		return Profile{}, err
	}
	contents, failed, err := readFiles(dir, names)
	if err != nil {
		if failed == "" {
			return Profile{}, errors.NewErr(TagProfileCopyFolderFails, err).SetMetadata("dir", dir)
		}
		return Profile{}, errors.NewErr(TagProfileCopyReadFailed, err).SetMetadata("file", failed)
	}
	if err := storage.WriteFileAtomic(ctx, filepath.Join(files, file), text); err != nil {
		return Profile{}, err
	}
	for i, n := range names {
		dest := filepath.Join(files, filepath.FromSlash(n))
		if err := storage.EnsureDir(ctx, filepath.Dir(dest)); err != nil {
			return Profile{}, err
		}
		if err := storage.WriteFileAtomic(ctx, dest, contents[i]); err != nil {
			return Profile{}, err
		}
	}
	if _, _, err := Load(files, file); err != nil {
		return Profile{}, err
	}
	m := meta{Version: metaVersion, Name: name, File: file, Created: s.clock.Now()}
	data, err := encodeMeta(m)
	if err != nil {
		return Profile{}, err
	}
	if err := storage.WriteFileAtomic(ctx, filepath.Join(s.dir, id, metaFile), data); err != nil {
		return Profile{}, err
	}
	return s.build(id, m), nil
}

// readFiles reads the files, named relative to dir, through an os.Root of dir, each up to one
// byte more than MaxFileBytes (the copy is checked again after it is made). It returns the
// operating system's error as it is, and the name of the file that failed ("" when the
// folder itself could not be opened).
func readFiles(dir string, names []string) ([][]byte, string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, "", err
	}
	// A root that was only read loses nothing when its close fails.
	defer func() { _ = root.Close() }()
	out := make([][]byte, 0, len(names))
	for _, name := range names {
		f, err := root.Open(name)
		if err != nil {
			return nil, name, err
		}
		data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
		// A file that was only read loses nothing when its close fails.
		_ = f.Close()
		if err != nil {
			return nil, name, err
		}
		out = append(out, data)
	}
	return out, "", nil
}
