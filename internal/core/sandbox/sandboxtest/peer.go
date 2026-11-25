// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

// Package sandboxtest helps the tests of the namespace: a peer that plays the VPN on the TUN
// device, and the probes, the roles in which the test binary is the application.
package sandboxtest

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/packettest"
)

// Peer plays the VPN for a test: it reads the packets that leave the namespace through the
// device and answers some of them.
type Peer struct {
	device *os.File

	mu      sync.Mutex
	packets [][]byte
	pings   bool
	// writeErr is the first failure to answer, reported when the test ends.
	writeErr error
}

// NewPeer starts reading the device. It stops when the test ends, before the device is closed
// by a cleanup that was registered earlier.
func NewPeer(t testing.TB, device *os.File) *Peer {
	t.Helper()
	p := &Peer{device: device}
	done := make(chan struct{})
	go p.read(done)
	t.Cleanup(func() {
		// A deadline in the past ends the read; the error that read gets then is expected. When
		// the test has closed the device already, that ended the read, and the deadline fails.
		_ = device.SetReadDeadline(time.Now().Add(-time.Second))
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("sandboxtest: the peer does not stop")
			return
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.writeErr != nil {
			t.Errorf("sandboxtest: the peer could not answer: %v", p.writeErr)
		}
	})
	return p
}

// read reads packets until a read fails.
func (p *Peer) read(done chan<- struct{}) {
	defer close(done)
	buf := make([]byte, 65536)
	for {
		n, err := p.device.Read(buf)
		if err != nil {
			return
		}
		packet := append([]byte(nil), buf[:n]...)
		p.mu.Lock()
		p.packets = append(p.packets, packet)
		answer := p.answer(packet)
		p.mu.Unlock()
		if answer == nil {
			continue
		}
		if _, err := p.device.Write(answer); err != nil {
			p.mu.Lock()
			if p.writeErr == nil {
				p.writeErr = err
			}
			p.mu.Unlock()
		}
	}
}

// answer is the peer's answer to a packet, or nil. It is called with mu held.
func (p *Peer) answer(packet []byte) []byte {
	switch {
	case p.pings && packettest.IsEchoRequest4(packet):
		return packettest.EchoReply4(packet)
	case p.pings && packettest.IsEchoRequest6(packet):
		return packettest.EchoReply6(packet)
	}
	return nil
}

// AnswerPings makes the peer answer echo requests of IPv4 and IPv6, or stop answering them.
// It is off when the peer is made.
func (p *Peer) AnswerPings(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pings = on
}

// Packets is a copy of every packet that the peer has seen so far.
func (p *Peer) Packets() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.packets))
	for i, packet := range p.packets {
		out[i] = append([]byte(nil), packet...)
	}
	return out
}

// WaitFor waits at most d for a packet that matches, one seen already included, and returns
// it. It fails the test when none comes.
func (p *Peer) WaitFor(t testing.TB, d time.Duration, match func(packet []byte) bool) []byte {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		for _, packet := range p.Packets() {
			if match(packet) {
				return packet
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("sandboxtest: no such packet came within %v (the peer saw %d packets)", d, len(p.Packets()))
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}
