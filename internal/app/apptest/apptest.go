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
	// Run is what RunState returns; Out are the lines of the output, numbered from 0.
	Run app.RunState
	Out []string

	mu         sync.Mutex
	calls      []string
	typed      []byte
	cols, rows int
	subs       map[int]func()
	nextSub    int
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

// Running is the sample with a started application: firefox running through Work, connected
// for 12 minutes and 31 seconds at SampleTime, with traffic, four events and 40 lines of
// output.
func Running() *Fake {
	f := Sample()
	at := func(h, m, s int) time.Time { return time.Date(2026, 11, 25, h, m, s, 0, time.UTC) }
	f.Run = app.RunState{
		Started: true, AppName: "firefox", AppRunning: true, State: "connected", Since: at(11, 47, 29),
		ProfileID: "id-work", ProfileName: "Work", Addrs: []string{"10.8.0.2/24"}, DNS: []string{"10.0.0.53"}, MTU: 1500,
		Down:    app.Traffic{Bytes: 1_500_000_000, Packets: 1_100_000, Rate: 1_200_000},
		Up:      app.Traffic{Bytes: 80_000_000, Packets: 400_000, Rate: 80_000},
		Dropped: 3,
		Events: []app.Event{
			{Time: at(11, 40, 0), Text: "connected to Home; running firefox"},
			{Time: at(11, 44, 0), Text: "the VPN connection ended (the server did not answer); the application has no network; connecting again…"},
			{Time: at(11, 44, 2), Text: "connected again"},
			{Time: at(11, 47, 29), Text: "now using Work"},
		},
		Now: SampleTime,
	}
	for i := range 40 {
		f.Out = append(f.Out, fmt.Sprintf("firefox: line %d of the output", i+1))
	}
	return f
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
func (f *Fake) AddProfile(ctx context.Context, path, name, dns string, withoutScripts bool) (string, error) {
	if err := f.call(ctx, "AddProfile", path, name, dns, withoutScripts); err != nil {
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

// ForgetPassword records the call.
func (f *Fake) ForgetPassword(ctx context.Context, id string) error {
	return f.call(ctx, "ForgetPassword", id)
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
func (f *Fake) Connect(ctx context.Context, id string, creds *app.Credentials, remember app.Remember) error {
	if creds == nil {
		return f.call(ctx, "Connect", id, "nil", remember)
	}
	return f.call(ctx, "Connect", id, creds.Username, "***", remember)
}

// Start records the call; after it the application runs, unless Run says otherwise.
func (f *Fake) Start(ctx context.Context) error {
	if err := f.call(ctx, "Start"); err != nil {
		return err
	}
	// A start that worked makes the application run, unless the test gave a run state itself.
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Run.Started {
		f.Run.Started, f.Run.AppRunning = true, true
		if len(f.Snap.Target) > 0 {
			f.Run.AppName = f.Snap.Target[0]
		}
	}
	return nil
}

// RunState returns Run.
func (f *Fake) RunState() app.RunState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Run
}

// OutputLen is the numbers of the lines of Out.
func (f *Fake) OutputLen() (first, end int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return 0, len(f.Out)
}

// OutputRange returns the lines of Out from from to to, as far as there are some.
func (f *Fake) OutputRange(from, to int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	from, to = max(from, 0), min(to, len(f.Out))
	if from >= to {
		return nil
	}
	return slices.Clone(f.Out[from:to])
}

// Input keeps what was typed, or returns the error of Fail.
func (f *Fake) Input(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.Fail["Input"]; err != nil {
		return err
	}
	f.typed = append(f.typed, p...)
	return nil
}

// Typed is everything given to Input.
func (f *Fake) Typed() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.typed)
}

// ResizeOutput keeps the size.
func (f *Fake) ResizeOutput(cols, rows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cols, f.rows = cols, rows
}

// Size is the last size given to ResizeOutput.
func (f *Fake) Size() (cols, rows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cols, f.rows
}

// Disconnect records the call.
func (f *Fake) Disconnect() {
	// A Disconnect has no error to return.
	_ = f.call(context.Background(), "Disconnect")
}

// Switch records the call, with the password as "***".
func (f *Fake) Switch(ctx context.Context, id string, creds *app.Credentials, remember app.Remember) error {
	if creds == nil {
		return f.call(ctx, "Switch", id, "nil", remember)
	}
	return f.call(ctx, "Switch", id, creds.Username, "***", remember)
}

// Stop records the call.
func (f *Fake) Stop() {
	// A Stop has no error to return.
	_ = f.call(context.Background(), "Stop")
}
