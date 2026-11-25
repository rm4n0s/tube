// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"net/netip"
	"time"

	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/sanitize"
)

// maxTargetPiece is the length of a piece of the target in the snapshot.
const maxTargetPiece = 255

// Profile is a profile as the TUI shows it. Every string is safe to draw.
type Profile struct {
	ID   string
	Name string
	// Remotes are the servers, as "vpn.example.com:1194 udp".
	Remotes       []string
	NeedsPassword bool
	Username      string
	// DNS are the profile's own DNS servers.
	DNS []string
	// LastUsed is when it was last connected; the zero time: never.
	LastUsed time.Time
	Ignored  []string
	Files    []string
	// Broken says why it cannot be used, or is "".
	Broken string
}

// Snapshot is what the TUI draws from (docs/SPEC.md §12.1).
type Snapshot struct {
	// Target is the application and its arguments, or none.
	Target []string
	// Profiles are sorted by name.
	Profiles []Profile
	// Now is the clock's time, for "3 days ago".
	Now time.Time
}

// Snapshot returns what the TUI draws from.
func (a *App) Snapshot(ctx context.Context) (Snapshot, error) {
	list, err := a.store.List(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Now: a.opts.Clock.Now()}
	for _, s := range a.opts.Argv {
		snap.Target = append(snap.Target, sanitize.Display(s, maxTargetPiece))
	}
	for _, p := range list {
		snap.Profiles = append(snap.Profiles, profileOf(p))
	}
	return snap, nil
}

// profileOf is a stored profile as the TUI shows it. The store made its strings safe.
func profileOf(p profiles.Profile) Profile {
	out := Profile{
		ID: p.ID, Name: p.Name, NeedsPassword: p.NeedsPassword, Username: p.Username, LastUsed: p.LastUsed,
		Ignored: p.Ignored, Files: p.Files, Broken: p.Broken,
	}
	for _, r := range p.Remotes {
		out.Remotes = append(out.Remotes, r.Address+" "+r.Network)
	}
	out.DNS = addrStrings(p.DNS)
	return out
}

// addrStrings are the addresses as text.
func addrStrings(addrs []netip.Addr) []string {
	var out []string
	for _, a := range addrs {
		out = append(out, a.String())
	}
	return out
}
