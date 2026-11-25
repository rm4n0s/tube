// SPDX-License-Identifier: GPL-2.0-only

package pump

import (
	"bytes"
	"io"
	"net/netip"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/vpn"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// network is the network of the tunnels of these tests.
var network = vpn.Network{Addrs: []netip.Prefix{netip.MustParsePrefix("10.9.0.2/24")}, MTU: 1400}

// fakeDevice is a device in memory: Deliver is a packet that the application sends, Written
// are the packets that the pump wrote to it. Its Write can be made to fail.
type fakeDevice struct {
	*vpntest.Tunnel

	mu        sync.Mutex
	writeErrs []error
}

// newDevice makes a fake device that takes packets of any size.
func newDevice() *fakeDevice {
	return &fakeDevice{Tunnel: vpntest.NewTunnel(vpn.Network{MTU: maxPacketSize})}
}

// failWrites makes the next Writes fail with these errors, one each.
func (d *fakeDevice) failWrites(errs ...error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writeErrs = append(d.writeErrs, errs...)
}

func (d *fakeDevice) Write(packet []byte) (int, error) {
	d.mu.Lock()
	if len(d.writeErrs) > 0 {
		err := d.writeErrs[0]
		d.writeErrs = d.writeErrs[1:]
		d.mu.Unlock()
		return 0, err
	}
	d.mu.Unlock()
	return d.Tunnel.Write(packet)
}

// next is the next packet written, or a failure of the test after a second.
func next(t *testing.T, written <-chan []byte) []byte {
	t.Helper()
	select {
	case p := <-written:
		return p
	case <-time.After(time.Second):
		t.Fatal("no packet came")
		return nil
	}
}

// waitDone waits a second for the pump to end by itself.
func waitDone(t *testing.T, p *Pump) {
	t.Helper()
	select {
	case <-p.Done():
	case <-time.After(time.Second):
		t.Fatal("the pump did not end")
	}
}

// start starts a pump between a new fake device and a new tunnel; it is stopped when the test
// ends.
func start(t *testing.T) (*Pump, *fakeDevice, *vpntest.Tunnel) {
	t.Helper()
	dev, tun := newDevice(), vpntest.NewTunnel(network)
	p := Start(dev, tun, network.MTU)
	t.Cleanup(func() { p.Stop() })
	return p, dev, tun
}

func TestPacketsGoBothWays(t *testing.T) {
	p, dev, tun := start(t)
	out := [][]byte{[]byte("one"), []byte("two!"), []byte("three")}
	in := [][]byte{[]byte("uno"), []byte("dos"), []byte("tres")}
	for _, packet := range out {
		dev.Deliver(packet)
	}
	for _, packet := range in {
		tun.Deliver(packet)
	}
	for _, want := range out {
		if got := next(t, tun.Written()); !bytes.Equal(got, want) {
			t.Errorf("the tunnel got %q, want %q", got, want)
		}
	}
	for _, want := range in {
		if got := next(t, dev.Written()); !bytes.Equal(got, want) {
			t.Errorf("the device got %q, want %q", got, want)
		}
	}
	want := Counters{ToTunnel: Traffic{3, 12}, FromTunnel: Traffic{3, 10}}
	if got := p.Counters(); got != want {
		t.Errorf("Counters = %+v, want %+v", got, want)
	}
	if got := p.Stop(); got != want {
		t.Errorf("Stop = %+v, want %+v", got, want)
	}
}

func TestTheTunnelRefusesALargePacket(t *testing.T) {
	p, dev, tun := start(t)
	dev.Deliver(make([]byte, network.MTU+1))
	dev.Deliver([]byte("small"))
	if got := next(t, tun.Written()); string(got) != "small" {
		t.Errorf("the tunnel got %q", got)
	}
	if c := p.Counters(); c.Dropped != 1 || c.ToTunnel.Packets != 1 {
		t.Errorf("Counters = %+v", c)
	}
}

func TestTheDeviceRefusesAPacket(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.EINVAL, syscall.EIO, syscall.ENOBUFS, syscall.EAGAIN} {
		t.Run(errno.Error(), func(t *testing.T) {
			p, dev, tun := start(t)
			dev.failWrites(errno)
			tun.Deliver([]byte("refused"))
			tun.Deliver([]byte("taken"))
			if got := next(t, dev.Written()); string(got) != "taken" {
				t.Errorf("the device got %q", got)
			}
			if c := p.Counters(); c.Dropped != 1 || c.FromTunnel.Packets != 1 || p.Err() != nil {
				t.Errorf("Counters = %+v, Err = %v", c, p.Err())
			}
		})
	}
}

func TestTheTunnelFails(t *testing.T) {
	t.Run("the server left", func(t *testing.T) {
		p, _, tun := start(t)
		tun.Fail(io.EOF)
		waitDone(t, p)
		if !errors.Is(p.Err(), io.EOF) {
			t.Errorf("Err = %v", p.Err())
		}
	})
	t.Run("an error of the library", func(t *testing.T) {
		p, _, tun := start(t)
		timeout := &errors.Error{Tag: "PingTimeout"}
		tun.Fail(timeout)
		waitDone(t, p)
		if p.Err() != error(timeout) {
			t.Errorf("Err = %v, want the same value", p.Err())
		}
	})
	t.Run("a packet to send meets the failure", func(t *testing.T) {
		p, dev, tun := start(t)
		timeout := &errors.Error{Tag: "PingTimeout"}
		// The fake fails its reads and its writes alike: whichever side sees the failure
		// first, the pump ends with the tunnel's error, untouched.
		tun.Fail(timeout)
		dev.Deliver([]byte("x"))
		waitDone(t, p)
		if p.Err() != error(timeout) {
			t.Errorf("Err = %v", p.Err())
		}
	})
}

func TestTheDeviceFails(t *testing.T) {
	t.Run("a read", func(t *testing.T) {
		p, dev, _ := start(t)
		dev.Fail(syscall.EBADF)
		waitDone(t, p)
		errtest.Route(t, p.Err(), "Pump.toTunnel.PumpDeviceReadFailed")
	})
	t.Run("a write", func(t *testing.T) {
		p, dev, tun := start(t)
		dev.failWrites(syscall.EBADF)
		tun.Deliver([]byte("x"))
		waitDone(t, p)
		errtest.Route(t, p.Err(), "Pump.fromTunnel.PumpDeviceWriteFailed")
	})
}

func TestStop(t *testing.T) {
	before := runtime.NumGoroutine()
	dev, tun := newDevice(), vpntest.NewTunnel(network)
	p := Start(dev, tun, network.MTU)
	dev.Deliver([]byte("x"))
	next(t, tun.Written())
	stopped := make(chan Counters, 1)
	go func() { stopped <- p.Stop() }()
	var first Counters
	select {
	case first = <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return")
	}
	if p.Err() != nil {
		t.Errorf("Err after Stop = %v", p.Err())
	}
	select {
	case <-p.Done():
		t.Error("Done is closed after Stop")
	default:
	}
	if second := p.Stop(); second != first || first.ToTunnel.Packets != 1 {
		t.Errorf("Stop = %+v, then %+v", first, second)
	}
	if tun.Closed() {
		t.Error("Stop closed the tunnel")
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("%d goroutines before the pump, %d after it stopped", before, after)
	}
}

func TestASecondPumpOnTheSameDevice(t *testing.T) {
	dev := newDevice()
	first := Start(dev, vpntest.NewTunnel(network), network.MTU)
	first.Stop()
	tun := vpntest.NewTunnel(network)
	second := Start(dev, tun, network.MTU)
	defer second.Stop()
	dev.Deliver([]byte("again"))
	if got := next(t, tun.Written()); string(got) != "again" {
		t.Errorf("the second tunnel got %q", got)
	}
	tun.Deliver([]byte("back"))
	if got := next(t, dev.Written()); string(got) != "back" {
		t.Errorf("the device got %q", got)
	}
}
