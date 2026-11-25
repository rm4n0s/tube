// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package session

import (
	"context"
	"fmt"
	"time"

	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/pump"
	"github.com/rm4n0s/tube/internal/core/sanitize"
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

// controller is the state of the goroutine that owns the connection: a wait before the next
// try, or a redial in flight.
type controller struct {
	s *Session
	// ctx ends with the controller: it cancels a redial in flight.
	ctx context.Context
	// state is the session's state, as the controller last published it.
	state State
	// tries counts the tries since the tunnel was lost.
	tries int
	// waited is closed when the wait is over; timer is the wait's timer.
	waited <-chan struct{}
	timer  clock.Timer
	// result gets the result of the redial in flight; cancel cancels it.
	result chan dialed
	cancel context.CancelFunc
}

// control is the controller: one goroutine that owns the tunnel, the pump and the session's
// state until the application has ended (docs/SPEC.md §10). Every event is one case of its
// select: the end of the pump, the end of a wait, the result of a redial, a command of the
// user, and the end of the application.
func (s *Session) control() {
	defer close(s.ctrlDone)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &controller{s: s, ctx: ctx, state: StateConnected}
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
		case cmd := <-s.cmds:
			if cmd.use != nil {
				cmd.done <- c.use(cmd.ctx, *cmd.use)
			} else {
				c.disconnect()
				close(cmd.disconnected)
			}
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

// setState publishes a new state.
func (c *controller) setState(state State) {
	c.state = state
	c.s.setState(state)
}

// lost handles the end of the pump: the tunnel failed. The application has no network from
// now on, until a redial works.
func (c *controller) lost() {
	s := c.s
	err := s.pump.Err()
	s.stopPump()
	s.log.Warn("the VPN connection ended", "err", err)
	if s.redial == nil {
		s.line(fmt.Sprintf("the VPN connection ended (%s); the application has no network", reason(err)))
		c.setState(StateDisconnected)
		return
	}
	s.line(fmt.Sprintf("the VPN connection ended (%s); the application has no network; connecting again…", reason(err)))
	c.tries = 0
	c.wait()
	c.setState(StateReconnecting)
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
	redial := c.s.redial
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
			s.line("the VPN server refused the user name or the password; the application stays without network")
			s.log.Error("connecting again was refused", "err", r.err)
			c.setState(StateRefused)
			return
		}
		s.log.Warn("connecting again failed", "err", r.err, "try", c.tries)
		c.wait()
		return
	}
	if err := c.connect(c.ctx, r.tunnel); err != nil {
		s.log.Error("the namespace cannot be configured for the new tunnel", "err", err)
		c.wait()
		return
	}
	c.tries = 0
	s.line("connected again")
	c.setState(StateConnected)
}

// connect configures the namespace for the tunnel and starts its pump. When the namespace
// cannot be configured, the tunnel is closed.
func (c *controller) connect(ctx context.Context, tun vpn.Tunnel) error {
	s := c.s
	n := tun.Network()
	if err := s.configure(ctx, n, s.dns); err != nil {
		s.closeTunnelOf(tun)
		return err
	}
	s.tunnel = tun
	s.pump = pump.Start(s.sb.Device(), tun, n.MTU)
	dns := s.dnsOf(n)
	s.publish(func(st *Status) { st.Network, st.DNS, st.ProfileName = n, dns, s.name }, false)
	s.log.Info("connected", "profile", s.name, "addrs", n.Addrs, "dns", dns, "mtu", n.MTU)
	return nil
}

// stopTries stops the tries to connect again: a wait is cut short, and a redial in flight is
// cancelled and a tunnel that it still returns is closed.
func (c *controller) stopTries() {
	if c.timer != nil {
		c.timer.Stop()
	}
	c.waited, c.timer = nil, nil
	if c.result != nil {
		c.cancel()
		if r := <-c.result; r.tunnel != nil {
			c.s.closeTunnelOf(r.tunnel)
		}
		c.result = nil
	}
}

// disconnect takes the network away from the application, until the user gives it another.
func (c *controller) disconnect() {
	if c.state == StateDisconnected {
		return
	}
	c.stopTries()
	c.s.stopPump()
	c.s.line("disconnected; the application has no network")
	c.s.log.Info("disconnected", "profile", c.s.name)
	c.setState(StateDisconnected)
}

// use gives the application the connection in place of the one it has, or of none. When the
// namespace cannot be configured for it, the new tunnel is closed and the session stays
// disconnected: the application has no network, and the user chooses again.
func (c *controller) use(ctx context.Context, conn Connection) error {
	s := c.s
	c.stopTries()
	s.stopPump()
	s.name = sanitize.Display(conn.ProfileName, maxNameInEnv)
	s.dns, s.redial = conn.DNS, conn.Redial
	if err := c.connect(ctx, conn.Tunnel); err != nil {
		s.log.Error("the namespace cannot be configured for the tunnel of the switch", "err", err, "profile", s.name)
		if c.state != StateDisconnected {
			s.line("disconnected; the application has no network")
		}
		c.setState(StateDisconnected)
		return err
	}
	s.line("now using " + s.name)
	c.setState(StateConnected)
	return nil
}

// end ends the controller with the application: the tries stop, the pump and the tunnel in use
// are closed, and the end of the application is published.
func (c *controller) end() {
	s := c.s
	c.stopTries()
	s.stopPump()
	st := s.appStatus.Status()
	if s.appErr != nil {
		st = statusLost
	}
	s.publish(func(status *Status) { status.AppRunning, status.Exit = false, st }, true)
}
