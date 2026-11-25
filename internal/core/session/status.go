// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"net/netip"
	"slices"
	"time"

	"github.com/rm4n0s/tube/internal/core/pump"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// State is what the session's connection is doing.
type State int

// The states of a session's connection.
const (
	// StateConnected: a tunnel is in use.
	StateConnected State = iota
	// StateReconnecting: the tunnel ended; Tube is connecting again.
	StateReconnecting
	// StateDisconnected: the user disconnected, a switch failed, or the tunnel ended with
	// nothing to connect again with: no tries.
	StateDisconnected
	// StateRefused: the server refused the login when connecting again: no tries.
	StateRefused
)

// String is the state's name, for logs and tests.
func (s State) String() string {
	switch s {
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateDisconnected:
		return "disconnected"
	case StateRefused:
		return "refused"
	}
	return "unknown"
}

// Status is a picture of the session at one moment.
type Status struct {
	State State
	// Since is when this state began, on the session's clock.
	Since time.Time
	// ProfileName is the profile in use, or the last one; safe to draw.
	ProfileName string
	// Network is the network of the tunnel in use; the zero value when there is none.
	Network vpn.Network
	// DNS are the servers that resolv.conf of the namespace has now.
	DNS []netip.Addr
	// Traffic is everything since the session started, over all its tunnels.
	Traffic pump.Counters
	// AppRunning says whether the application runs.
	AppRunning bool
	// Exit is the exit status, once the application has ended.
	Exit int
}

// snapshot is what the controller publishes after each change, and the pump that runs, whose
// counters Status adds.
type snapshot struct {
	status Status
	pump   *pump.Pump
}

// Status returns a picture of the session. It may be called from any goroutine.
func (s *Session) Status() Status {
	s.snapMu.Lock()
	snap := s.snap
	s.snapMu.Unlock()
	st := snap.status
	st.DNS = slices.Clone(st.DNS)
	st.Network.Addrs = slices.Clone(st.Network.Addrs)
	st.Network.DNSServers = slices.Clone(st.Network.DNSServers)
	st.Network.SearchDomains = slices.Clone(st.Network.SearchDomains)
	if snap.pump != nil {
		st.Traffic = addCounters(st.Traffic, snap.pump.Counters())
	}
	return st
}

// Done is closed when the application has ended.
func (s *Session) Done() <-chan struct{} { return s.appEnded }

// publish writes the picture of the session after a change; only the controller (and Start,
// before it) calls it. With changed, the state changed, and OnChange is called, without the
// lock.
func (s *Session) publish(change func(*Status), changed bool) {
	s.snapMu.Lock()
	change(&s.snap.status)
	s.snap.status.Traffic = s.traffic
	s.snap.pump = s.pump
	s.snapMu.Unlock()
	if changed && s.opts.OnChange != nil {
		s.opts.OnChange()
	}
}

// setState changes the state, and calls OnChange.
func (s *Session) setState(state State) {
	now := s.opts.Clock.Now()
	s.publish(func(st *Status) {
		st.State, st.Since = state, now
		if s.tunnel == nil {
			st.Network = vpn.Network{}
		}
	}, true)
}
