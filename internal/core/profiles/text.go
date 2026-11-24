// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"context"
	"path/filepath"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/storage"
)

// The Tags of the errors of a profile's text.
const (
	TagProfileBroken       = "ProfileBroken"
	TagProfileTextTooLarge = "ProfileTextTooLarge"
)

// usable returns the profile id when it can be used.
func (s *Store) usable(ctx context.Context, id string) (Profile, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	if p.Broken != "" {
		return Profile{}, errors.New(TagProfileBroken, "the profile cannot be used", "id", id, "reason", p.Broken)
	}
	return p, nil
}

// Text returns the text of the profile file.
func (s *Store) Text(ctx context.Context, id string) ([]byte, error) {
	p, err := s.usable(ctx, id)
	if err != nil {
		return nil, err
	}
	return storage.ReadFile(ctx, filepath.Join(p.FilesDir, p.File), MaxFileBytes)
}

// ReplaceText replaces the text of the profile file, when the new text parses with the files
// that are stored next to it. When it does not, the stored profile stays as it was.
func (s *Store) ReplaceText(ctx context.Context, id string, text []byte) error {
	if len(text) > MaxFileBytes {
		return errors.New(TagProfileTextTooLarge, "the text of the profile is too large", "max", MaxFileBytes)
	}
	p, err := s.usable(ctx, id)
	if err != nil {
		return err
	}
	if _, _, err := loadWith(p.FilesDir, p.File, text); err != nil {
		return err
	}
	return storage.WriteFileAtomic(ctx, filepath.Join(p.FilesDir, p.File), text)
}
