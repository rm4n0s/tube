// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package vpntest

import (
	"net"
	"net/netip"
	"testing"

	"github.com/rm4n0s/tube/internal/core/dnstest"
)

// StartDNS answers DNS queries over UDP on the address (in the namespace of scripts/vpn-ns.sh,
// where the test may bind port 53): a name of names gets its address, another NXDOMAIN. It
// stops when the test ends.
func StartDNS(t testing.TB, addr netip.AddrPort, names map[string]netip.Addr) {
	t.Helper()
	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addr))
	if err != nil {
		t.Fatalf("vpntest: a DNS server on %v: %v", addr, err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1500)
		for {
			n, from, err := conn.ReadFromUDPAddrPort(buf)
			if err != nil {
				return // closed by the cleanup
			}
			name, _, _, ok := dnstest.Question(buf[:n])
			if !ok {
				continue
			}
			found, known := names[name]
			// A client that is gone does not need the answer.
			_, _ = conn.WriteToUDPAddrPort(dnstest.Answer(buf[:n], found, known), from)
		}
	}()
	t.Cleanup(func() {
		closeOrLog(t, conn)
		<-done
	})
}
