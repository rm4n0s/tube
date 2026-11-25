// SPDX-License-Identifier: GPL-2.0-only

// Package apptest has a fake of app.Application with fixed content, for the tests of the TUI.
package apptest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rm4n0s/tube/internal/app"
)

// SampleTime is the time of the sample: 2026-11-25 12:00 UTC.
var SampleTime = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

// Fake is an app.Application with fixed content, for the tests of the TUI. A method with no
// entry in Fail succeeds and changes nothing: a test sets Snap itself and calls Emit. It is safe
// for several goroutines.
type Fake struct {
	// Snap is what Snapshot returns.
	Snap app.Snapshot
	// Fail maps a method's name to the error it returns.
	Fail map[string]error
	// Block maps a method's name to a channel: the method waits for it to be closed, or for its
	// context to end.
	Block map[string]chan struct{}

	mu      sync.Mutex
	calls   []string
	subs    map[int]func()
	nextSub int
}

var _ app.Application = (*Fake)(nil)

// Sample has five profiles, the target "firefox", and the time SampleTime.
func Sample() *Fake {
	long := "東京-" + strings.Repeat("long-name-", 6) + "x"
	return &Fake{Snap: app.Snapshot{
		Target: []string{"firefox"},
		Now:    SampleTime,
		Profiles: []app.Profile{
			{ID: "0123456789abcdef", Name: "0123456789abcdef", Broken: "its profile cannot be read: a mistake on line 3"},
			{ID: "id-home", Name: "Home", Remotes: []string{"home.example.net:1194 udp"}, LastUsed: SampleTime.Add(-time.Hour)},
			{
				ID: "id-work", Name: "Work", Remotes: []string{"vpn1.example.com:1194 udp", "vpn2.example.com:443 tcp"},
				NeedsPassword: true, Username: "ada", DNS: []string{"10.0.0.53"}, LastUsed: SampleTime.Add(-72 * time.Hour),
				Ignored: []string{"persist-key", "persist-tun"}, Files: []string{"ca.crt", "client.crt", "client.key"},
			},
			{ID: "id-work-backup", Name: "work-backup", Remotes: []string{"backup.example.com:1194 udp"}},
			{ID: "id-long", Name: long, Remotes: []string{"far.example.org:1194 udp"}, LastUsed: SampleTime.Add(-40 * 24 * time.Hour)},
		},
	}}
}

// Empty has no profile and no target.
func Empty() *Fake {
	return &Fake{Snap: app.Snapshot{Now: SampleTime}}
}

// Calls are the calls so far, in order, such as "RenameProfile(id-work, Office)". A password
// is written as "***". Snapshot and Subscribe are not recorded.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Emit calls the subscribers.
func (f *Fake) Emit() {
	f.mu.Lock()
	subs := make([]func(), 0, len(f.subs))
	for _, s := range f.subs {
		subs = append(subs, s)
	}
	f.mu.Unlock()
	for _, s := range subs {
		s()
	}
}

// call records a call, waits when Block says so, and returns the error of Fail.
func (f *Fake) call(ctx context.Context, name string, args ...any) error {
	text := make([]string, len(args))
	for i, a := range args {
		text[i] = fmt.Sprint(a)
	}
	f.mu.Lock()
	f.calls = append(f.calls, name+"("+strings.Join(text, ", ")+")")
	block := f.Block[name]
	err := f.Fail[name]
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// Snapshot returns Snap.
func (f *Fake) Snapshot(ctx context.Context) (app.Snapshot, error) {
	f.mu.Lock()
	snap, err := f.Snap, f.Fail["Snapshot"]
	f.mu.Unlock()
	return snap, err
}

// Subscribe registers f for Emit.
func (f *Fake) Subscribe(fn func()) (cancel func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subs == nil {
		f.subs = map[int]func(){}
	}
	f.nextSub++
	id := f.nextSub
	f.subs[id] = fn
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.subs, id)
	}
}

// AddProfile records the call; the id of the new profile is "id-new".
func (f *Fake) AddProfile(ctx context.Context, path, name string, withoutScripts bool) (string, error) {
	if err := f.call(ctx, "AddProfile", path, name, withoutScripts); err != nil {
		return "", err
	}
	return "id-new", nil
}

// RenameProfile records the call.
func (f *Fake) RenameProfile(ctx context.Context, id, name string) error {
	return f.call(ctx, "RenameProfile", id, name)
}

// SetUsername records the call.
func (f *Fake) SetUsername(ctx context.Context, id, username string) error {
	return f.call(ctx, "SetUsername", id, username)
}

// SetDNS records the call.
func (f *Fake) SetDNS(ctx context.Context, id, text string) error {
	return f.call(ctx, "SetDNS", id, text)
}

// DeleteProfile records the call.
func (f *Fake) DeleteProfile(ctx context.Context, id string) error {
	return f.call(ctx, "DeleteProfile", id)
}

// BeginEdit records the call; the path is "/edit/<id>.ovpn".
func (f *Fake) BeginEdit(ctx context.Context, id string) (string, error) {
	if err := f.call(ctx, "BeginEdit", id); err != nil {
		return "", err
	}
	return "/edit/" + id + ".ovpn", nil
}

// EndEdit records the call; it says that the profile changed.
func (f *Fake) EndEdit(ctx context.Context, id, path string) (bool, error) {
	if err := f.call(ctx, "EndEdit", id, path); err != nil {
		return false, err
	}
	return true, nil
}

// Connect records the call, with the password as "***".
func (f *Fake) Connect(ctx context.Context, id string, creds *app.Credentials, remember bool) error {
	if creds == nil {
		return f.call(ctx, "Connect", id, "nil", remember)
	}
	return f.call(ctx, "Connect", id, creds.Username, "***", remember)
}
