// SPDX-License-Identifier: GPL-2.0-only

// Package vpntest has a vpn.Tunnel in memory for the tests of the parts that use one, and,
// with the build tag tube_server, a real OpenVPN server for the tests that need one.
package vpntest

import (
	"bytes"
	"net"
	"os"
	"sync"
	"time"

	"github.com/rm4n0s/errors"
	openvpn "github.com/rm4n0s/gopenvpn-conn"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// writtenBuffer is how many written packets Written keeps until they are read; more are dropped.
const writtenBuffer = 1024

// Tunnel is a vpn.Tunnel in memory. Its methods may be called from several goroutines.
type Tunnel struct {
	network vpn.Network
	written chan []byte

	mu       sync.Mutex
	queue    [][]byte
	err      error
	closed   bool
	deadline time.Time
	changed  chan struct{} // closed and replaced when something that a Read waits for changes
	onWrite  func(packet []byte) [][]byte
}

var _ vpn.Tunnel = (*Tunnel)(nil)

// NewTunnel returns a tunnel whose Network is network.
func NewTunnel(network vpn.Network) *Tunnel {
	return &Tunnel{network: network, written: make(chan []byte, writtenBuffer), changed: make(chan struct{})}
}

// notify wakes the Reads that wait. The caller holds mu.
func (t *Tunnel) notify() {
	close(t.changed)
	t.changed = make(chan struct{})
}

// Deliver makes the next Read return a copy of the packet.
func (t *Tunnel) Deliver(packet []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queue = append(t.queue, bytes.Clone(packet))
	t.notify()
}

// Written gives a copy of every packet given to Write, in order. When the test does not read
// them, more than 1024 are dropped.
func (t *Tunnel) Written() <-chan []byte { return t.written }

// Fail makes Read and Write return err from now on.
func (t *Tunnel) Fail(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
	t.notify()
}

// Closed says whether Close was called.
func (t *Tunnel) Closed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// OnWrite sets a function that is called with each written packet and may return packets to
// deliver: a fake server. Set it before the tunnel is used.
func (t *Tunnel) OnWrite(f func(packet []byte) [][]byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onWrite = f
}

// Read returns the next delivered packet. It waits for one, for a failure, for Close (then
// net.ErrClosed) or for the read deadline (then os.ErrDeadlineExceeded).
func (t *Tunnel) Read(packet []byte) (int, error) {
	for {
		t.mu.Lock()
		switch {
		case t.closed:
			t.mu.Unlock()
			return 0, net.ErrClosed
		case t.err != nil:
			err := t.err
			t.mu.Unlock()
			return 0, err
		case len(t.queue) > 0:
			next := t.queue[0]
			t.queue = t.queue[1:]
			t.mu.Unlock()
			return copy(packet, next), nil
		case !t.deadline.IsZero() && !time.Now().Before(t.deadline):
			t.mu.Unlock()
			return 0, os.ErrDeadlineExceeded
		}
		changed, deadline := t.changed, t.deadline
		t.mu.Unlock()
		var timer *time.Timer
		var timeout <-chan time.Time
		if !deadline.IsZero() {
			timer = time.NewTimer(time.Until(deadline))
			timeout = timer.C
		}
		select {
		case <-changed:
		case <-timeout:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

// Write takes one packet. A packet larger than the MTU is refused as the real tunnel refuses
// it.
func (t *Tunnel) Write(packet []byte) (int, error) {
	t.mu.Lock()
	switch {
	case t.closed:
		t.mu.Unlock()
		return 0, net.ErrClosed
	case t.err != nil:
		err := t.err
		t.mu.Unlock()
		return 0, err
	case len(packet) > t.network.MTU:
		t.mu.Unlock()
		return 0, &errors.Error{Tag: openvpn.TagPacketTooLarge, Message: "the packet is larger than the MTU of the tunnel"}
	}
	onWrite := t.onWrite
	t.mu.Unlock()
	select {
	case t.written <- bytes.Clone(packet):
	default:
	}
	if onWrite != nil {
		for _, reply := range onWrite(bytes.Clone(packet)) {
			t.Deliver(reply)
		}
	}
	return len(packet), nil
}

// SetReadDeadline sets the time after which a Read returns os.ErrDeadlineExceeded; the zero
// time removes it.
func (t *Tunnel) SetReadDeadline(d time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deadline = d
	t.notify()
	return nil
}

// Network returns the network that the tunnel was made with.
func (t *Tunnel) Network() vpn.Network { return t.network }

// Close ends the tunnel: Read and Write return net.ErrClosed. A second Close does nothing.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		t.notify()
	}
	return nil
}
