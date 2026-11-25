// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"context"
	"fmt"
	"time"

	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/pump"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// redialDelays are the waits before the tries to connect again; the last one repeats
// (docs/SPEC.md §10, D14).
var redialDelays = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second}

// delay is the wait before the try number n, from 0.
func delay(n int) time.Duration {
	if n >= len(redialDelays) {
		return redialDelays[len(redialDelays)-1]
	}
	return redialDelays[n]
}

// dialed is the result of a redial.
type dialed struct {
	tunnel vpn.Tunnel
	err    error
}

// controller is the state of the goroutine that owns the tunnel and the pump: a wait before the
// next try, or a redial in flight.
type controller struct {
	s *Session
	// ctx ends with the controller: it cancels a redial in flight.
	ctx context.Context
	// tries counts the tries since the tunnel was lost.
	tries int
	// waited is closed when the wait is over; timer is the wait's timer.
	waited <-chan struct{}
	timer  clock.Timer
	// result gets the result of the redial in flight; cancel cancels it.
	result chan dialed
	cancel context.CancelFunc
	// refused is set when the server refused the login: no more tries.
	refused bool
}

// control is the controller: one goroutine that owns the tunnel and the pump until the
// application has ended. It connects again when the tunnel fails (docs/SPEC.md §10). Every
// event is one case of its select.
func (s *Session) control() {
	defer close(s.ctrlDone)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &controller{s: s, ctx: ctx}
	for {
		var pumpDone <-chan struct{}
		if s.pump != nil {
			pumpDone = s.pump.Done()
		}
		select {
		case <-pumpDone:
			c.lost()
		case <-c.waited:
			c.redial()
		case r := <-c.result:
			c.dialed(r)
		case <-s.appEnded:
			c.end()
			return
		}
	}
}

// stopPump stops the pump in use, adds what it moved to the session's traffic, and closes the
// tunnel in use.
func (s *Session) stopPump() {
	if s.pump != nil {
		s.traffic = addCounters(s.traffic, s.pump.Stop())
		s.pump = nil
	}
	s.closeTunnel()
}

// addCounters adds the counters of two pumps.
func addCounters(a, b pump.Counters) pump.Counters {
	return pump.Counters{
		ToTunnel:   pump.Traffic{Packets: a.ToTunnel.Packets + b.ToTunnel.Packets, Bytes: a.ToTunnel.Bytes + b.ToTunnel.Bytes},
		FromTunnel: pump.Traffic{Packets: a.FromTunnel.Packets + b.FromTunnel.Packets, Bytes: a.FromTunnel.Bytes + b.FromTunnel.Bytes},
		Dropped:    a.Dropped + b.Dropped,
	}
}

// lost handles the end of the pump: the tunnel failed. The application has no network from
// now on, until a redial works.
func (c *controller) lost() {
	s := c.s
	err := s.pump.Err()
	s.stopPump()
	s.log.Warn("the VPN connection ended", "err", err)
	if s.opts.Redial == nil {
		s.line(fmt.Sprintf("the VPN connection ended (%s); the application has no network", reason(err)))
		return
	}
	s.line(fmt.Sprintf("the VPN connection ended (%s); the application has no network; connecting again…", reason(err)))
	c.tries = 0
	c.wait()
}

// wait starts the wait before the next try.
func (c *controller) wait() {
	waited := make(chan struct{})
	c.waited = waited
	c.timer = c.s.opts.Clock.AfterFunc(delay(c.tries), func() { close(waited) })
}

// redial starts a try in its own goroutine; its result comes to the loop.
func (c *controller) redial() {
	c.waited, c.timer = nil, nil
	ctx, cancel := context.WithCancel(c.ctx)
	result := make(chan dialed, 1)
	c.result, c.cancel = result, cancel
	redial := c.s.opts.Redial
	go func() {
		tun, err := redial(ctx)
		result <- dialed{tun, err}
	}()
}

// dialed handles the result of a try.
func (c *controller) dialed(r dialed) {
	s := c.s
	c.result = nil
	c.cancel()
	c.tries++
	if r.err != nil {
		if vpn.KindOf(r.err) == vpn.KindAuthFailed {
			c.refused = true
			s.line("the VPN server refused the user name or the password; the application stays without network")
			s.log.Error("connecting again was refused", "err", r.err)
			return
		}
		s.log.Warn("connecting again failed", "err", r.err, "try", c.tries)
		c.wait()
		return
	}
	n := r.tunnel.Network()
	if err := s.configure(c.ctx, n); err != nil {
		s.closeTunnelOf(r.tunnel)
		s.log.Error("the namespace cannot be configured for the new tunnel", "err", err)
		c.wait()
		return
	}
	s.tunnel = r.tunnel
	s.pump = pump.Start(s.sb.Device(), r.tunnel, n.MTU)
	c.tries = 0
	s.line("connected again")
	s.log.Info("connected again", "addrs", n.Addrs, "dns", s.dnsOf(n), "mtu", n.MTU)
}

// end ends the controller with the application: a wait is cut short, a redial in flight is
// cancelled and a tunnel that it still returns is closed, and the pump and the tunnel in use are
// closed.
func (c *controller) end() {
	if c.timer != nil {
		c.timer.Stop()
	}
	if c.result != nil {
		c.cancel()
		if r := <-c.result; r.tunnel != nil {
			c.s.closeTunnelOf(r.tunnel)
		}
	}
	c.s.stopPump()
}
