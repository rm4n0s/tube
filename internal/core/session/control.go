// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"context"
	"net/netip"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// TagEnded is the error of a command that came after the application had ended.
const TagEnded = "SessionEnded"

// Connection is a tunnel and what the session needs to know with it.
type Connection struct {
	// Tunnel is connected; the session closes it.
	Tunnel vpn.Tunnel
	// ProfileName is the profile's name, for the lines and the status.
	ProfileName string
	// DNS are the profile's own DNS servers, or none.
	DNS []netip.Addr
	// Redial connects the profile again after the tunnel failed; nil does not connect again.
	Redial func(ctx context.Context) (vpn.Tunnel, error)
}

// command is a command of the user to the controller: Disconnect, or Use. use is the work of a
// Use, which the controller runs, giving itself; it is nil for a Disconnect. The controller
// answers a Use on done, and closes disconnected when it has disconnected.
type command struct {
	use          func(running *controller) error
	done         chan error
	disconnected chan struct{}
}

// Disconnect takes the network away from the application: the pump stops, the tunnel is
// closed, and Tube does not connect again until Use is called. It returns when that is done.
func (s *Session) Disconnect() {
	disconnected := make(chan struct{})
	select {
	case s.cmds <- command{disconnected: disconnected}:
		<-disconnected
	case <-s.ctrlDone:
	}
}

// Use gives the application this tunnel in place of the one it has, or of none. When the
// namespace cannot be configured for it, the tunnel is closed, the application stays without
// network, and the error is returned. After the application has ended, the tunnel is closed and
// the error is SessionEnded.
func (s *Session) Use(ctx context.Context, c Connection) error {
	done := make(chan error, 1)
	// The controller does the work, in its goroutine. It is handed over through errors.Async,
	// here where it is asked for: the route of its error starts at the caller of Use, and not at
	// the controller's loop.
	var ctl *controller
	use := errors.Async(func() error { return ctl.use(ctx, c) })
	cmd := command{done: done, use: func(running *controller) error {
		ctl = running
		return use()
	}}
	select {
	case s.cmds <- cmd:
		return <-done
	case <-s.ctrlDone:
	}
	s.closeTunnelOf(c.Tunnel)
	return errors.New(TagEnded, "the application has ended")
}
