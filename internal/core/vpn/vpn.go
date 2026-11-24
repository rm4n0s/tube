// SPDX-License-Identifier: GPL-2.0-only

// Package vpn connects a stored profile with gopenvpn-conn and gives the rest of Tube a Tunnel:
// a stream of IP packets, and what the server pushed, checked (docs/SPEC.md §7). It is, with
// package profiles, the only part of Tube that imports gopenvpn-conn.
package vpn

import (
	"context"
	"time"

	"github.com/rm4n0s/errors"
	openvpn "github.com/rm4n0s/gopenvpn-conn"
	"github.com/rm4n0s/tube/internal/core/profiles"
)

// The Tags of Dial's errors.
const (
	TagProfileBroken     = "VPNProfileBroken"
	TagCredentialsNeeded = "VPNCredentialsNeeded"
)

// Credentials are the user name and the password of a profile that asks for them. The password
// is never logged and never put in an error.
type Credentials struct{ Username, Password string }

// Tunnel is a connected VPN: a stream of IP packets.
type Tunnel interface {
	// Read reads one IP packet from the VPN.
	Read(packet []byte) (int, error)
	// Write sends one IP packet to the VPN.
	Write(packet []byte) (int, error)
	// SetReadDeadline interrupts a Read, as net.Conn's does.
	SetReadDeadline(t time.Time) error
	// Network is what the server pushed, checked.
	Network() Network
	// Close ends the connection; the library tells the server.
	Close() error
}

// Dial connects the profile and returns when the tunnel is ready. The errors of gopenvpn-conn
// are returned untouched; KindOf says what kind they are.
func Dial(ctx context.Context, p profiles.Profile, creds *Credentials) (Tunnel, error) {
	if p.Broken != "" {
		return nil, errors.New(TagProfileBroken, "the profile cannot be used", "id", p.ID, "reason", p.Broken)
	}
	profile, _, err := profiles.Load(p.FilesDir, p.File)
	if err != nil {
		return nil, err
	}
	if profile.NeedsCredentials {
		if creds == nil {
			return nil, errors.New(TagCredentialsNeeded, "the profile needs a user name and a password", "id", p.ID)
		}
		given := openvpn.Credentials{Username: creds.Username, Password: creds.Password}
		// The profile was parsed for this call only: nothing else shares its Config.
		profile.Config.Credentials = func(context.Context) (openvpn.Credentials, error) { return given, nil }
	}
	conn, err := profile.Dial(ctx)
	if err != nil {
		return nil, err
	}
	network, err := NetworkFromState(conn.ConnectionState())
	if err != nil {
		// The network is what matters; the server learns that the client left either way.
		_ = conn.Close()
		return nil, err
	}
	return &tunnel{conn: conn, network: network}, nil
}

// tunnel is the Tunnel of a real connection.
type tunnel struct {
	conn    *openvpn.Conn
	network Network
}

func (t *tunnel) Read(packet []byte) (int, error)   { return t.conn.Read(packet) }
func (t *tunnel) Write(packet []byte) (int, error)  { return t.conn.Write(packet) }
func (t *tunnel) SetReadDeadline(d time.Time) error { return t.conn.SetReadDeadline(d) }
func (t *tunnel) Network() Network                  { return t.network }
func (t *tunnel) Close() error                      { return t.conn.Close() }
