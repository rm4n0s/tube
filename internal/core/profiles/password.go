// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"context"
	"os"
	"path/filepath"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/storage"
)

// The remembered password of a profile (docs/SPEC.md §6.6): a file of the profile's folder, next
// to meta.json, with the mode of every file of Tube.
const (
	passwordFile = "password"
	// MaxPasswordLen is the longest password that is remembered, in bytes.
	MaxPasswordLen = 4096
)

// The Tags of the remembered password.
const (
	TagProfilePasswordTooLong      = "ProfilePasswordTooLong"
	TagProfilePasswordForgetFailed = "ProfilePasswordForgetFailed"
)

// SetPassword remembers the password of the profile, in a file that only the user can read; an
// empty one forgets it. The password is never in an error.
func (s *Store) SetPassword(ctx context.Context, id, password string) error {
	dir, err := s.profileDir(id)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, passwordFile)
	if password == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errors.NewErr(TagProfilePasswordForgetFailed, err).SetMetadata("id", id)
		}
		return nil
	}
	if len(password) > MaxPasswordLen {
		return errors.New(TagProfilePasswordTooLong, "the password is too long to be remembered", "id", id, "max", MaxPasswordLen)
	}
	return storage.WriteFileAtomic(ctx, path, []byte(password))
}

// Password returns the remembered password of the profile, or "" when none is remembered.
func (s *Store) Password(ctx context.Context, id string) (string, error) {
	dir, err := s.profileDir(id)
	if err != nil {
		return "", err
	}
	data, err := storage.ReadFile(ctx, filepath.Join(dir, passwordFile), MaxPasswordLen)
	if err != nil {
		if appErr, ok := errors.FromError(err); ok && appErr.Tag == storage.TagFileNotExist {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// hasPassword says whether the folder of a profile has a remembered password.
func hasPassword(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, passwordFile))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
