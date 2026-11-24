// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"io/fs"
	"os"
	"sort"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/gopenvpn-conn/ovpn"
)

// The limits of the files that a profile names (docs/SPEC.md §6.2).
const (
	// MaxFileBytes is the largest file that a profile may name.
	MaxFileBytes = 1 << 20
	// MaxFiles is the largest number of files that a profile may name.
	MaxFiles = 16
)

// The Tags of Load's errors.
const (
	TagProfileFolderOpenFailed = "ProfileFolderOpenFailed"
	TagProfileFileTooLarge     = "ProfileFileTooLarge"
	TagProfileTooManyFiles     = "ProfileTooManyFiles"
)

// maxFiles is MaxFiles; tests may lower it.
var maxFiles = MaxFiles

// Load parses the profile called file in the folder dir, and returns what the parser made
// and the names of the files that the profile names, relative to dir, sorted. The folder is
// opened as an os.Root, so no name leaves it, also not through a symbolic link.
func Load(dir, file string) (*ovpn.Profile, []string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, errors.NewErr(TagProfileFolderOpenFailed, err).SetMetadata("dir", dir)
	}
	// A root that was only read loses nothing when its close fails.
	defer func() { _ = root.Close() }()
	rec := &recordingFS{fsys: root.FS(), profile: file, opened: map[string]bool{}}
	profile, err := ovpn.ParseFS(rec, file)
	if err != nil {
		switch rec.breach {
		case breachTooLarge:
			return nil, nil, errors.New(TagProfileFileTooLarge, "a file that the profile names is too large",
				"file", rec.breachName, "max", MaxFileBytes)
		case breachTooMany:
			return nil, nil, errors.New(TagProfileTooManyFiles, "the profile names too many files", "max", maxFiles)
		}
		return nil, nil, err
	}
	names := make([]string, 0, len(rec.opened))
	for name := range rec.opened {
		names = append(names, name)
	}
	sort.Strings(names)
	return profile, names, nil
}

// breach is a limit of the files that a profile broke.
type breach int

const (
	breachNone breach = iota
	breachTooLarge
	breachTooMany
)

// recordingFS is a file system that remembers the files that the parser opens, besides the
// profile itself, and keeps the limits on them. A limit that is broken makes the open or the
// read fail, and is remembered, so that Load can say which limit it was.
type recordingFS struct {
	fsys    fs.FS
	profile string
	opened  map[string]bool

	breach     breach
	breachName string
}

// Open opens a file of the folder.
func (r *recordingFS) Open(name string) (fs.File, error) {
	if name == r.profile {
		return r.fsys.Open(name)
	}
	if !r.opened[name] && len(r.opened) >= maxFiles {
		r.breach, r.breachName = breachTooMany, name
		return nil, &fs.PathError{Op: "open", Path: name, Err: errLimit}
	}
	f, err := r.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	r.opened[name] = true
	return &limitedFile{File: f, owner: r, name: name}, nil
}

// limitedFile is a file whose reads fail once more than MaxFileBytes have been read.
type limitedFile struct {
	fs.File
	owner *recordingFS
	name  string
	read  int64
}

// Read reads the file, up to the limit.
func (f *limitedFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.read += int64(n)
	if f.read > MaxFileBytes {
		f.owner.breach, f.owner.breachName = breachTooLarge, f.name
		return 0, errLimit
	}
	return n, err
}

// errLimit is the cause that the parser gets when a limit is broken. Load replaces the
// parser's error with one of its own, so it is never seen outside.
var errLimit = &errors.Error{Tag: "ProfileLimitReached", Message: "a limit of the files of a profile was reached"}
