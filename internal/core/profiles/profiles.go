// SPDX-License-Identifier: GPL-2.0-only

// Package profiles is the store of OpenVPN client profiles (docs/SPEC.md §6): each profile is a
// copy of the .ovpn file and of the files it names, in a folder of its own under the
// configuration folder, with a meta.json that says its name and what the user set for it. A
// profile is only parsed, by the parser of gopenvpn-conn: nothing that it names is ever run.
package profiles

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/storage"
)

// The limits of what the user sets for a profile.
const (
	// MaxNameLen is the longest name of a profile, in characters.
	MaxNameLen = 64
	// MaxDNS is the largest number of DNS servers of a profile.
	MaxDNS = 4
	// maxMetaBytes is the largest meta.json that is read.
	maxMetaBytes = 64 << 10
	// metaVersion is the version of meta.json that this Tube writes and reads.
	metaVersion = 1
	// staleAdd is how old a folder without meta.json must be before it is taken for an add
	// that failed and is removed: a younger one may be an add of another Tube in progress.
	staleAdd = time.Minute
)

// The names of the files and folders of a profile's folder.
const (
	metaFile  = "meta.json"
	filesDir  = "files"
	storeDir  = "profiles"
	idPattern = "16 lowercase hexadecimal characters"
)

// The Tags of the store's errors.
const (
	TagProfileNameEmpty       = "ProfileNameEmpty"
	TagProfileNameTooLong     = "ProfileNameTooLong"
	TagProfileNameTaken       = "ProfileNameTaken"
	TagProfileIDInvalid       = "ProfileIDInvalid"
	TagProfileNotFound        = "ProfileNotFound"
	TagProfileStatFailed      = "ProfileStatFailed"
	TagProfileListFailed      = "ProfileListFailed"
	TagProfileMetaInvalid     = "ProfileMetaInvalid"
	TagProfileMetaNewer       = "ProfileMetaNewer"
	TagProfileMetaEncodeFail  = "ProfileMetaEncodeFailed"
	TagProfileTooManyDNS      = "ProfileTooManyDNS"
	TagProfileDeleteFailed    = "ProfileDeleteFailed"
	TagProfileDNSInvalid      = "ProfileDNSInvalid"
	TagProfileDNSTooMany      = "ProfileDNSTooMany"
	TagProfileRandomFailed    = "ProfileRandomFailed"
	TagProfileAddCancelled    = "ProfileAddCancelled"
	TagProfilePathEmpty       = "ProfilePathEmpty"
	TagProfileCopyReadFailed  = "ProfileCopyReadFailed"
	TagProfileCopyFolderFails = "ProfileCopyFolderOpenFailed"
)

// ErrNotFound matches, with errors.Is, the error of a profile that is not in the store.
var ErrNotFound = &errors.Error{Tag: TagProfileNotFound}

// Remote is one server of a profile, as it is shown.
type Remote struct {
	// Network is "udp" or "tcp", with "4" or "6" when the profile says so.
	Network string
	// Address is "host:port", safe to draw.
	Address string
}

// Profile is one stored profile.
type Profile struct {
	// ID is 16 hexadecimal characters: the name of the profile's folder.
	ID string
	// Name is what the user sees; safe to draw.
	Name string
	// File is the name of the profile file inside FilesDir.
	File string
	// FilesDir is <config>/profiles/<id>/files, absolute when the configuration folder is.
	FilesDir string
	// Created is when the profile was added.
	Created time.Time
	// LastUsed is when the profile was last connected; the zero time when never.
	LastUsed time.Time
	// Username is the remembered user name, or "".
	Username string
	// DNS are the DNS servers that the user set, or none.
	DNS []netip.Addr
	// Remotes are the servers of the profile, in its order.
	Remotes []Remote
	// NeedsPassword says that the profile asks for a user name and a password.
	NeedsPassword bool
	// HasPassword says that a password is remembered for the profile (Store.Password). The
	// password itself is never in a Profile.
	HasPassword bool
	// Ignored are the options that the parser accepted and that do nothing; safe to draw.
	Ignored []string
	// Files are the files next to the profile file, relative to FilesDir, sorted.
	Files []string
	// Broken says why the profile cannot be used, or is "". A broken profile has only ID,
	// Name and FilesDir set.
	Broken string
}

// meta is the content of meta.json (docs/SPEC.md §6.1).
type meta struct {
	Version  int        `json:"version"`
	Name     string     `json:"name"`
	File     string     `json:"file"`
	Created  time.Time  `json:"created"`
	LastUsed *time.Time `json:"last_used,omitempty"`
	Username string     `json:"username"`
	DNS      []string   `json:"dns,omitempty"`
}

// Store is the store of profiles in a configuration folder. Its methods may be called from
// several goroutines and by several Tubes at once: each reads the folder when it needs it, and
// the last write of a meta.json wins (docs/SPEC.md §5).
type Store struct {
	dir   string
	clock clock.Clock
	// afterLoad, when set, is called by Add between parsing the original and copying it. Tests
	// use it to change the original under Add's feet.
	afterLoad func()
}

// NewStore returns the store of the configuration folder configDir.
func NewStore(configDir string, clk clock.Clock) *Store {
	return &Store{dir: filepath.Join(configDir, storeDir), clock: clk}
}

// CleanName returns the name as it is stored: trimmed, and made safe with
// sanitize.Display(name, MaxNameLen).
func CleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New(TagProfileNameEmpty, "a profile needs a name")
	}
	if utf8.RuneCountInString(name) > MaxNameLen {
		return "", errors.New(TagProfileNameTooLong, "the name of a profile is too long", "max", MaxNameLen)
	}
	return sanitize.Display(name, MaxNameLen), nil
}

// nameFree returns an error when a profile other than exceptID has the name, compared without
// case.
func (s *Store) nameFree(ctx context.Context, name, exceptID string) error {
	list, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.ID != exceptID && strings.EqualFold(p.Name, name) {
			return errors.New(TagProfileNameTaken, "another profile has this name", "name", name)
		}
	}
	return nil
}

// validID says whether id is one that the store makes.
func validID(id string) bool {
	if len(id) != 16 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// newID makes the id of a new profile.
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", errors.NewErr(TagProfileRandomFailed, err)
	}
	return hex.EncodeToString(b), nil
}

// profileDir returns the folder of the profile id, after checking that the id is one the store
// makes (so that it can never be a path) and that the profile is there.
func (s *Store) profileDir(id string) (string, error) {
	if !validID(id) {
		return "", errors.New(TagProfileIDInvalid, "the id of a profile is "+idPattern, "id", sanitize.Display(id, 64))
	}
	dir := filepath.Join(s.dir, id)
	if _, err := os.Lstat(filepath.Join(dir, metaFile)); err != nil {
		if os.IsNotExist(err) {
			return "", errors.New(TagProfileNotFound, "there is no profile with this id", "id", id)
		}
		return "", errors.NewErr(TagProfileStatFailed, err).SetMetadata("id", id)
	}
	return dir, nil
}

// readMeta reads and checks the meta.json of the profile's folder dir.
func readMeta(ctx context.Context, dir string) (meta, error) {
	data, err := storage.ReadFile(ctx, filepath.Join(dir, metaFile), maxMetaBytes)
	if err != nil {
		return meta{}, err
	}
	var m meta
	if err := json.Unmarshal(data, &m); err != nil {
		return meta{}, errors.NewErr(TagProfileMetaInvalid, err).SetMetadata("dir", dir)
	}
	if m.Version != metaVersion {
		return meta{}, errors.New(TagProfileMetaNewer, "the profile was made by a newer Tube", "dir", dir, "version", m.Version)
	}
	return m, nil
}

// encodeMeta is the content of a meta.json.
func encodeMeta(m meta) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, errors.NewErr(TagProfileMetaEncodeFail, err)
	}
	return append(data, '\n'), nil
}

// build makes the Profile of the folder of id, whose meta.json was read, by parsing its
// profile file. A profile that does not parse is broken.
func (s *Store) build(id string, m meta) Profile {
	dir := filepath.Join(s.dir, id, filesDir)
	p := Profile{ID: id, Name: sanitize.Display(m.Name, MaxNameLen), FilesDir: dir}
	parsed, files, err := Load(dir, m.File)
	if err != nil {
		p.Broken = "its profile cannot be read: " + sanitize.Display(err.Error(), 200)
		return p
	}
	p.File = m.File
	p.Created = m.Created
	if m.LastUsed != nil {
		p.LastUsed = *m.LastUsed
	}
	p.Username = sanitize.Display(m.Username, 128)
	for _, text := range m.DNS {
		if addr, err := netip.ParseAddr(text); err == nil {
			p.DNS = append(p.DNS, addr)
		}
	}
	for _, r := range parsed.Remotes {
		p.Remotes = append(p.Remotes, Remote{Network: sanitize.Display(r.Network, 8), Address: sanitize.Display(r.Address, 255)})
	}
	p.NeedsPassword = parsed.NeedsCredentials
	p.HasPassword = hasPassword(filepath.Join(s.dir, id))
	for _, name := range parsed.Ignored {
		p.Ignored = append(p.Ignored, sanitize.Display(name, 64))
	}
	p.Files = files
	return p
}

// read makes the Profile of the folder of id; a meta.json that cannot be read makes it broken.
func (s *Store) read(ctx context.Context, id string) Profile {
	dir := filepath.Join(s.dir, id)
	m, err := readMeta(ctx, dir)
	if err != nil {
		reason := "its meta.json cannot be read"
		if appErr, ok := errors.FromError(err); ok && appErr.Tag == TagProfileMetaNewer {
			reason = "it was made by a newer Tube"
		}
		return Profile{ID: id, Name: id, FilesDir: filepath.Join(dir, filesDir), Broken: reason}
	}
	return s.build(id, m)
}

// List returns every profile of the store, sorted by name without case, then by id. A
// profile that cannot be used is in the list, broken. A folder without meta.json that is
// older than a minute is an add that failed, and is removed.
func (s *Store) List(ctx context.Context) ([]Profile, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.NewErr(TagProfileListFailed, err).SetMetadata("dir", s.dir)
	}
	var list []Profile
	for _, e := range entries {
		if !e.IsDir() || !validID(e.Name()) {
			continue // not a folder that the store made
		}
		dir := filepath.Join(s.dir, e.Name())
		if _, err := os.Lstat(filepath.Join(dir, metaFile)); os.IsNotExist(err) {
			if info, err := e.Info(); err == nil && s.clock.Now().Sub(info.ModTime()) > staleAdd {
				// A failed add; its own failure to go away only leaves the folder for next time.
				_ = os.RemoveAll(dir)
			}
			continue
		}
		list = append(list, s.read(ctx, e.Name()))
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := strings.ToLower(list[i].Name), strings.ToLower(list[j].Name)
		if a != b {
			return a < b
		}
		return list[i].ID < list[j].ID
	})
	return list, nil
}

// Get returns the profile id. A profile that cannot be used is returned, broken.
func (s *Store) Get(ctx context.Context, id string) (Profile, error) {
	if _, err := s.profileDir(id); err != nil {
		return Profile{}, err
	}
	return s.read(ctx, id), nil
}

// update reads the profile's meta.json, changes it, and writes it back.
func (s *Store) update(ctx context.Context, id string, change func(*meta)) error {
	dir, err := s.profileDir(id)
	if err != nil {
		return err
	}
	m, err := readMeta(ctx, dir)
	if err != nil {
		return err
	}
	change(&m)
	data, err := encodeMeta(m)
	if err != nil {
		return err
	}
	return storage.WriteFileAtomic(ctx, filepath.Join(dir, metaFile), data)
}

// Rename gives the profile another name.
func (s *Store) Rename(ctx context.Context, id, name string) error {
	name, err := CleanName(name)
	if err != nil {
		return err
	}
	if err := s.nameFree(ctx, name, id); err != nil {
		return err
	}
	return s.update(ctx, id, func(m *meta) { m.Name = name })
}

// SetUsername remembers the user name of the profile; an empty one forgets it.
func (s *Store) SetUsername(ctx context.Context, id, username string) error {
	username = sanitize.Display(strings.TrimSpace(username), 128)
	return s.update(ctx, id, func(m *meta) { m.Username = username })
}

// SetDNS sets the DNS servers of the profile; none means the ones that the VPN pushes.
func (s *Store) SetDNS(ctx context.Context, id string, dns []netip.Addr) error {
	if len(dns) > MaxDNS {
		return errors.New(TagProfileTooManyDNS, "a profile has 4 DNS servers at most", "max", MaxDNS)
	}
	var texts []string
	for _, addr := range dns {
		texts = append(texts, addr.String())
	}
	return s.update(ctx, id, func(m *meta) { m.DNS = texts })
}

// MarkUsed sets the time when the profile was last used to now.
func (s *Store) MarkUsed(ctx context.Context, id string) error {
	now := s.clock.Now()
	return s.update(ctx, id, func(m *meta) { m.LastUsed = &now })
}

// Delete removes the profile and its files.
func (s *Store) Delete(ctx context.Context, id string) error {
	dir, err := s.profileDir(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return errors.NewErr(TagProfileDeleteFailed, err).SetMetadata("id", id)
	}
	return nil
}

// ParseDNS reads DNS servers from text: IP addresses separated by spaces or commas.
func ParseDNS(text string) ([]netip.Addr, error) {
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	var out []netip.Addr
	for _, part := range parts {
		addr, err := netip.ParseAddr(part)
		if err != nil || addr.IsUnspecified() || addr.Zone() != "" {
			return nil, errors.New(TagProfileDNSInvalid, "a DNS server is an IP address", "value", sanitize.Display(part, 64))
		}
		out = append(out, addr)
	}
	if len(out) > MaxDNS {
		return nil, errors.New(TagProfileDNSTooMany, "a profile has 4 DNS servers at most", "max", MaxDNS)
	}
	return out, nil
}
