// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"time"

	"github.com/rm4n0s/tube/internal/core/pump"
)

// Traffic is what passed in one direction.
type Traffic struct {
	Bytes, Packets uint64
	// Rate is in bytes a second, over the last second.
	Rate float64
}

// Event is a line that Tube had to say while the application runs.
type Event struct {
	Time time.Time
	// Text is without the "tube: " in front; safe to draw.
	Text string
}

// RunState is what the run screen draws from (docs/SPEC.md §12.4, §13.8).
type RunState struct {
	// Started says that Start was called and worked.
	Started bool
	// AppName is the application's name (the base name of argv[0]), safe to draw.
	AppName    string
	AppRunning bool
	// Exit is the exit status, once the application has ended.
	Exit int
	// State is "connected", "reconnecting", "disconnected" or "refused".
	State string
	// Since is when the state began.
	Since time.Time
	// ProfileID and ProfileName are the profile in use, or the last one.
	ProfileID   string
	ProfileName string
	// Addrs are the tunnel's addresses, "10.8.0.2/24".
	Addrs []string
	// DNS are the DNS servers in use.
	DNS []string
	MTU int
	// Down is from the VPN to the application, Up the other way.
	Down, Up Traffic
	Dropped  uint64
	// Events are the last 100, oldest first.
	Events []Event
	Now    time.Time
}

// sampler keeps the traffic of the last two samples, a second apart, for the rates.
type sampler struct {
	prev, cur pump.Counters
	n         int
}

// add takes a sample.
func (s *sampler) add(c pump.Counters) {
	s.prev, s.cur = s.cur, c
	s.n++
}

// rates are the rates down and up over the last second; 0 before there are two samples.
func (s *sampler) rates() (down, up float64) {
	if s.n < 2 {
		return 0, 0
	}
	return float64(s.cur.FromTunnel.Bytes - s.prev.FromTunnel.Bytes), float64(s.cur.ToTunnel.Bytes - s.prev.ToTunnel.Bytes)
}
