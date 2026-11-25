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

// command is a command of the user to the controller: Disconnect, or Use with its connection.
// The controller answers a Use on done, and closes disconnected when it has disconnected.
type command struct {
	use          *Connection
	ctx          context.Context
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
	select {
	case s.cmds <- command{use: &c, ctx: ctx, done: done}:
		return <-done
	case <-s.ctrlDone:
	}
	s.closeTunnelOf(c.Tunnel)
	return errors.New(TagEnded, "the application has ended")
}
