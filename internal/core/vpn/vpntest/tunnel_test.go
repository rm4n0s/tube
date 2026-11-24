// SPDX-License-Identifier: GPL-2.0-only

package vpntest

import (
	"bytes"
	"io"
	"net"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	openvpn "github.com/rm4n0s/gopenvpn-conn"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// network is the network of the tunnels of these tests.
var network = vpn.Network{Addrs: []netip.Prefix{netip.MustParsePrefix("10.9.0.2/24")}, MTU: 1400}

// readResult is what a Read returned.
type readResult struct {
	data []byte
	err  error
}

// readAsync reads once in a goroutine.
func readAsync(tun *Tunnel) <-chan readResult {
	ch := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 2000)
		n, err := tun.Read(buf)
		ch <- readResult{buf[:n], err}
	}()
	return ch
}

// waitRead waits for a result for at most two seconds.
func waitRead(t *testing.T, ch <-chan readResult) readResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return")
		return readResult{}
	}
}

func TestDeliverAndRead(t *testing.T) {
	tun := NewTunnel(network)
	tun.Deliver([]byte("one"))
	tun.Deliver([]byte("two"))
	for _, want := range []string{"one", "two"} {
		if r := waitRead(t, readAsync(tun)); r.err != nil || string(r.data) != want {
			t.Errorf("Read = %q, %v, want %q", r.data, r.err, want)
		}
	}
}

func TestReadWaitsForADelivery(t *testing.T) {
	tun := NewTunnel(network)
	ch := readAsync(tun)
	select {
	case r := <-ch:
		t.Fatalf("Read returned %q, %v before anything was delivered", r.data, r.err)
	case <-time.After(50 * time.Millisecond):
	}
	tun.Deliver([]byte("late"))
	if r := waitRead(t, ch); r.err != nil || string(r.data) != "late" {
		t.Errorf("Read = %q, %v", r.data, r.err)
	}
}

func TestDeliverCopies(t *testing.T) {
	tun := NewTunnel(network)
	packet := []byte("abc")
	tun.Deliver(packet)
	packet[0] = 'x'
	if r := waitRead(t, readAsync(tun)); string(r.data) != "abc" {
		t.Errorf("Read = %q: the delivered packet was not copied", r.data)
	}
}

func TestWritten(t *testing.T) {
	tun := NewTunnel(network)
	packet := []byte("hello")
	if n, err := tun.Write(packet); err != nil || n != 5 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	packet[0] = 'j'
	select {
	case got := <-tun.Written():
		if string(got) != "hello" {
			t.Errorf("Written gave %q: the written packet was not copied", got)
		}
	case <-time.After(time.Second):
		t.Fatal("nothing was written")
	}
}

func TestWritingMoreThanTheBufferDoesNotBlock(t *testing.T) {
	tun := NewTunnel(network)
	done := make(chan struct{})
	go func() {
		for range writtenBuffer + 10 {
			if _, err := tun.Write([]byte("x")); err != nil {
				t.Error(err)
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked when nobody read Written")
	}
}

func TestFail(t *testing.T) {
	tun := NewTunnel(network)
	ch := readAsync(tun)
	tun.Fail(io.EOF)
	if r := waitRead(t, ch); r.err != io.EOF {
		t.Errorf("Read error = %v, want io.EOF", r.err)
	}
	if _, err := tun.Write([]byte("x")); err != io.EOF {
		t.Errorf("Write error = %v, want io.EOF", err)
	}
}

func TestClose(t *testing.T) {
	tun := NewTunnel(network)
	ch := readAsync(tun)
	if tun.Closed() {
		t.Fatal("Closed before Close")
	}
	if err := tun.Close(); err != nil {
		t.Fatal(err)
	}
	if r := waitRead(t, ch); !errors.Is(r.err, net.ErrClosed) {
		t.Errorf("Read error = %v, want net.ErrClosed", r.err)
	}
	if _, err := tun.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
		t.Errorf("Write error = %v, want net.ErrClosed", err)
	}
	if !tun.Closed() {
		t.Error("Closed is false after Close")
	}
	if err := tun.Close(); err != nil {
		t.Errorf("a second Close: %v", err)
	}
}

func TestReadDeadline(t *testing.T) {
	t.Run("a deadline in the past returns at once", func(t *testing.T) {
		tun := NewTunnel(network)
		if err := tun.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		if r := waitRead(t, readAsync(tun)); !errors.Is(r.err, os.ErrDeadlineExceeded) {
			t.Errorf("Read error = %v", r.err)
		}
	})
	t.Run("a deadline that passes while Read waits", func(t *testing.T) {
		tun := NewTunnel(network)
		if err := tun.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if r := waitRead(t, readAsync(tun)); !errors.Is(r.err, os.ErrDeadlineExceeded) {
			t.Errorf("Read error = %v", r.err)
		}
	})
	t.Run("a deadline set while Read waits", func(t *testing.T) {
		tun := NewTunnel(network)
		ch := readAsync(tun)
		time.Sleep(20 * time.Millisecond)
		if err := tun.SetReadDeadline(time.Now()); err != nil {
			t.Fatal(err)
		}
		if r := waitRead(t, ch); !errors.Is(r.err, os.ErrDeadlineExceeded) {
			t.Errorf("Read error = %v", r.err)
		}
	})
	t.Run("the zero time removes it", func(t *testing.T) {
		tun := NewTunnel(network)
		if err := tun.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := tun.SetReadDeadline(time.Time{}); err != nil {
			t.Fatal(err)
		}
		tun.Deliver([]byte("x"))
		if r := waitRead(t, readAsync(tun)); r.err != nil {
			t.Errorf("Read error = %v", r.err)
		}
	})
}

func TestAPacketOverTheMTU(t *testing.T) {
	tun := NewTunnel(network)
	_, err := tun.Write(make([]byte, network.MTU+1))
	if !errors.Is(err, &errors.Error{Tag: openvpn.TagPacketTooLarge}) {
		t.Errorf("Write error = %v, want %s", err, openvpn.TagPacketTooLarge)
	}
	if vpn.KindOf(err) != vpn.KindPacketTooLarge {
		t.Error("KindOf does not see it as a packet too large")
	}
	if _, err := tun.Write(make([]byte, network.MTU)); err != nil {
		t.Errorf("a packet of the MTU: %v", err)
	}
}

func TestOnWrite(t *testing.T) {
	tun := NewTunnel(network)
	tun.OnWrite(func(packet []byte) [][]byte {
		return [][]byte{append([]byte("re:"), packet...)}
	})
	if _, err := tun.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if r := waitRead(t, readAsync(tun)); string(r.data) != "re:ping" {
		t.Errorf("Read = %q", r.data)
	}
}

func TestConcurrentUse(t *testing.T) {
	tun := NewTunnel(network)
	var wg sync.WaitGroup
	const count = 200
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range count {
			tun.Deliver([]byte("p"))
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 10)
		for range count {
			if _, err := tun.Read(buf); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	wg.Wait()
	if !bytes.Equal(tun.Network().Addrs[0].Addr().AsSlice(), []byte{10, 9, 0, 2}) {
		t.Error("Network changed")
	}
}
