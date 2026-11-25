// SPDX-License-Identifier: GPL-2.0-only

// Package pump moves IP packets between the TUN device of the namespace and the VPN tunnel, in
// both directions, until the tunnel fails or the pump is stopped (docs/SPEC.md §9).
package pump

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// The Tags of the pump's own errors.
const (
	TagDeviceReadFailed  = "PumpDeviceReadFailed"
	TagDeviceWriteFailed = "PumpDeviceWriteFailed"
)

// The sizes of the buffers: a packet from the device is at most the tunnel's MTU (a larger one
// is refused by the tunnel); a packet from the tunnel is at most an IP packet.
const (
	deviceSlack   = 64
	maxPacketSize = 65535
)

// Device is the TUN device, or a fake of it: one IP packet for each Read and each Write.
type Device interface {
	Read(packet []byte) (int, error)
	Write(packet []byte) (int, error)
	SetReadDeadline(t time.Time) error
}

// Traffic is what passed one way.
type Traffic struct {
	Packets, Bytes uint64
}

// Counters say what a pump moved.
type Counters struct {
	// ToTunnel went from the application to the VPN; FromTunnel came back.
	ToTunnel, FromTunnel Traffic
	// Dropped are the packets that the device or the tunnel refused.
	Dropped uint64
}

// Pump moves packets between a device and a tunnel.
type Pump struct {
	device Device
	tunnel vpn.Tunnel

	// stopping is set by Stop: a read that ends then is the stop, not a failure.
	stopping atomic.Bool
	// endOnce lets the first end win: err is why, and done is closed when the pump ended by
	// itself.
	endOnce sync.Once
	err     error
	done    chan struct{}
	wg      sync.WaitGroup

	stopOnce sync.Once
	final    Counters

	toPackets, toBytes, fromPackets, fromBytes, dropped atomic.Uint64
}

// Start starts moving packets. mtu is the tunnel's.
func Start(device Device, tunnel vpn.Tunnel, mtu int) *Pump {
	p := &Pump{device: device, tunnel: tunnel, done: make(chan struct{})}
	p.wg.Add(2)
	go p.toTunnel(mtu + deviceSlack)
	go p.fromTunnel()
	return p
}

// Done is closed when the pump has ended by itself: the tunnel or the device failed.
func (p *Pump) Done() <-chan struct{} { return p.done }

// Err is why the pump ended by itself; nil before that, and nil when Stop ended it.
func (p *Pump) Err() error {
	select {
	case <-p.done:
		return p.err
	default:
		return nil
	}
}

// Counters returns what the pump moved so far.
func (p *Pump) Counters() Counters {
	return Counters{
		ToTunnel:   Traffic{Packets: p.toPackets.Load(), Bytes: p.toBytes.Load()},
		FromTunnel: Traffic{Packets: p.fromPackets.Load(), Bytes: p.fromBytes.Load()},
		Dropped:    p.dropped.Load(),
	}
}

// Stop ends the pump, waits for its goroutines, and returns the counters. It closes neither the
// device nor the tunnel, which can be used again. It can be called twice.
func (p *Pump) Stop() Counters {
	p.stopOnce.Do(func() {
		p.stopping.Store(true)
		p.interrupt()
		p.wg.Wait()
		// Both can be read again, by another pump. A deadline that cannot be set belongs to a
		// device or a tunnel that is closed, whose reads fail anyway.
		_ = p.device.SetReadDeadline(time.Time{})
		_ = p.tunnel.SetReadDeadline(time.Time{})
		p.final = p.Counters()
	})
	return p.final
}

// interrupt ends the reads of both goroutines. A deadline that cannot be set belongs to a device
// or a tunnel that is closed, whose reads fail anyway.
func (p *Pump) interrupt() {
	past := time.Now().Add(-time.Second)
	_ = p.device.SetReadDeadline(past)
	_ = p.tunnel.SetReadDeadline(past)
}

// end ends the pump because of err, unless the pump is stopping or has ended already: the first
// end wins, and the other goroutine is interrupted.
func (p *Pump) end(err error) {
	if p.stopping.Load() {
		return
	}
	p.endOnce.Do(func() {
		p.err = err
		close(p.done)
		p.interrupt()
	})
}

// toTunnel moves packets from the device to the tunnel.
func (p *Pump) toTunnel(size int) {
	defer p.wg.Done()
	buf := make([]byte, size)
	for {
		n, err := p.device.Read(buf)
		if err != nil {
			if !p.stopping.Load() {
				p.end(errors.NewErr(TagDeviceReadFailed, err))
			}
			return
		}
		if _, err := p.tunnel.Write(buf[:n]); err != nil {
			if vpn.KindOf(err) == vpn.KindPacketTooLarge {
				p.dropped.Add(1)
				continue
			}
			p.end(err)
			return
		}
		p.toPackets.Add(1)
		p.toBytes.Add(uint64(n))
	}
}

// fromTunnel moves packets from the tunnel to the device.
func (p *Pump) fromTunnel() {
	defer p.wg.Done()
	buf := make([]byte, maxPacketSize)
	for {
		n, err := p.tunnel.Read(buf)
		if err != nil {
			if !p.stopping.Load() {
				p.end(err)
			}
			return
		}
		if _, err := p.device.Write(buf[:n]); err != nil {
			if refused(err) {
				p.dropped.Add(1)
				continue
			}
			p.end(errors.NewErr(TagDeviceWriteFailed, err))
			return
		}
		p.fromPackets.Add(1)
		p.fromBytes.Add(uint64(n))
	}
}

// refused says whether the device refused one packet, which is dropped, rather than failed.
func refused(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EIO) || errors.Is(err, syscall.ENOBUFS) ||
		errors.Is(err, syscall.EAGAIN)
}
