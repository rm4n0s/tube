// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/storage"
)

// TagEditUnknown is the error of an EndEdit with a path that BeginEdit did not give.
const TagEditUnknown = "AppEditUnknown"

// profileExt is the extension of a profile file.
const profileExt = ".ovpn"

// notify calls the subscribers, without the lock.
func (a *App) notify() {
	a.subsMu.Lock()
	subs := make([]func(), 0, len(a.subs))
	for _, f := range a.subs {
		subs = append(subs, f)
	}
	a.subsMu.Unlock()
	for _, f := range subs {
		f()
	}
}

// Subscribe calls f, from any goroutine, after every change of the profiles. The returned
// function ends the subscription.
func (a *App) Subscribe(f func()) (cancel func()) {
	a.subsMu.Lock()
	defer a.subsMu.Unlock()
	a.nextSub++
	id := a.nextSub
	a.subs[id] = f
	return func() {
		a.subsMu.Lock()
		defer a.subsMu.Unlock()
		delete(a.subs, id)
	}
}

// expandHome is the path with a leading "~/" made the home folder.
func (a *App) expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, ok := a.opts.Env("HOME"); ok && home != "" {
			return filepath.Join(home, rest)
		}
	}
	return path
}

// profileNameOf is the name that a profile file gives its profile: the file's name without
// ".ovpn".
func profileNameOf(path string) string {
	name := filepath.Base(path)
	if strings.HasSuffix(strings.ToLower(name), profileExt) {
		name = name[:len(name)-len(profileExt)]
	}
	return name
}

// AddProfile adds the profile file at path under the name; an empty name is the file's name
// without ".ovpn". A path that starts with "~/" is in the home folder. dns are the DNS servers
// of the profile, as text (IP addresses separated by spaces or commas); empty means the ones
// that the VPN pushes. They are checked before anything is added, so a mistake in them adds
// nothing. It returns the new profile's id.
func (a *App) AddProfile(ctx context.Context, path, name, dns string, withoutScripts bool) (string, error) {
	servers, err := profiles.ParseDNS(dns)
	if err != nil {
		return "", err
	}
	path = a.expandHome(path)
	if name == "" {
		name = profileNameOf(path)
	}
	p, err := a.store.Add(ctx, path, name, profiles.AddOptions{WithoutScripts: withoutScripts})
	if err != nil {
		return "", err
	}
	if len(servers) > 0 {
		if err := a.store.SetDNS(ctx, p.ID, servers); err != nil {
			// A profile without the DNS servers that were asked for is not what was asked
			// for: it is taken back.
			if derr := a.store.Delete(ctx, p.ID); derr != nil {
				a.log.Warn("a profile that was added without its DNS servers cannot be removed", "err", derr, "profile", p.ID)
			}
			return "", err
		}
	}
	a.log.Info("profile added", "profile", p.ID, "name", p.Name)
	a.rememberAddFolder(ctx, path)
	a.notify()
	return p.ID, nil
}

// RenameProfile renames the profile.
func (a *App) RenameProfile(ctx context.Context, id, name string) error {
	return a.changed(a.store.Rename(ctx, id, name))
}

// ForgetPassword forgets the remembered password of the profile: the next connect asks for it.
func (a *App) ForgetPassword(ctx context.Context, id string) error {
	return a.changed(a.store.SetPassword(ctx, id, ""))
}

// SetUsername sets the remembered user name of the profile; "" forgets it.
func (a *App) SetUsername(ctx context.Context, id, username string) error {
	return a.changed(a.store.SetUsername(ctx, id, username))
}

// SetDNS sets the profile's DNS servers from text ("1.1.1.1, 9.9.9.9"); empty text removes them.
func (a *App) SetDNS(ctx context.Context, id, text string) error {
	dns, err := profiles.ParseDNS(text)
	if err != nil {
		return err
	}
	return a.changed(a.store.SetDNS(ctx, id, dns))
}

// DeleteProfile deletes the profile.
func (a *App) DeleteProfile(ctx context.Context, id string) error {
	return a.changed(a.store.Delete(ctx, id))
}

// changed tells the subscribers when the change worked, and returns its error.
func (a *App) changed(err error) error {
	if err == nil {
		a.notify()
	}
	return err
}

// BeginEdit copies the profile's text into a file that only the user can read and returns its
// path (docs/SPEC.md §13.5).
func (a *App) BeginEdit(ctx context.Context, id string) (string, error) {
	text, err := a.store.Text(ctx, id)
	if err != nil {
		return "", err
	}
	dir, err := a.editDir(ctx)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+profileExt)
	if err := storage.WriteFileAtomic(ctx, path, text); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.edits[id] = path
	a.mu.Unlock()
	return path, nil
}

// editDir is the App's temporary folder for edits, made on first use; Close removes it.
func (a *App) editDir(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tempDir == "" {
		dir, err := a.dirs.NewTempDir(ctx)
		if err != nil {
			return "", err
		}
		a.tempDir = dir
	}
	return a.tempDir, nil
}

// EndEdit reads the file back, replaces the profile's text when it changed, and removes the
// file. changed says whether the profile was replaced. A text that the parser refuses leaves the
// profile as it was.
func (a *App) EndEdit(ctx context.Context, id, path string) (changed bool, err error) {
	a.mu.Lock()
	given, ok := a.edits[id]
	if ok && given == path {
		delete(a.edits, id)
	}
	a.mu.Unlock()
	if !ok || given != path {
		return false, errors.New(TagEditUnknown, "this file was not given for an edit of the profile", "profile", id)
	}
	defer func() {
		if err := os.Remove(path); err != nil {
			a.log.Warn("the file of an edit cannot be removed", "err", err)
		}
	}()
	text, err := storage.ReadFile(ctx, path, profiles.MaxFileBytes)
	if err != nil {
		return false, err
	}
	old, err := a.store.Text(ctx, id)
	if err != nil {
		return false, err
	}
	if bytes.Equal(old, text) {
		return false, nil
	}
	if err := a.store.ReplaceText(ctx, id, text); err != nil {
		return false, err
	}
	a.log.Info("profile edited", "profile", id)
	a.notify()
	return true, nil
}
