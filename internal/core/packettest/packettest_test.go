// SPDX-License-Identifier: GPL-2.0-only

package packettest

import (
	"net/netip"
	"testing"
)

func TestChecksum(t *testing.T) {
	// The example of RFC 1071 §3: the sum of these words is 0xddf2, so the checksum is 0x220d.
	data := []byte{0x00, 0x01, 0xf2, 0x03, 0xf4, 0xf5, 0xf6, 0xf7}
	if got := Checksum(data); got != 0x220d {
		t.Errorf("Checksum = %#04x, want 0x220d", got)
	}
	// A header with its checksum in place sums to zero.
	packet := EchoRequest4(netip.MustParseAddr("10.8.0.2"), netip.MustParseAddr("10.8.0.1"), 7, 1)
	if got := Checksum(packet[:ipv4HeaderLen]); got != 0 {
		t.Errorf("the IPv4 header does not check: %#04x", got)
	}
	if got := Checksum(packet[ipv4HeaderLen:]); got != 0 {
		t.Errorf("the ICMP message does not check: %#04x", got)
	}
	if got := Checksum([]byte{0xff}); got != 0x00ff {
		t.Errorf("an odd length: %#04x", got)
	}
}

func TestEcho4(t *testing.T) {
	src, dst := netip.MustParseAddr("10.8.0.2"), netip.MustParseAddr("10.8.0.1")
	request := EchoRequest4(src, dst, 0x1234, 5)
	if Version(request) != 4 || Src(request) != src || Dst(request) != dst {
		t.Errorf("version %d, %v -> %v", Version(request), Src(request), Dst(request))
	}
	if !IsEchoRequest4(request) || IsEchoReply4(request, 0x1234) {
		t.Error("the request is not seen as a request")
	}
	reply := EchoReply4(request)
	if !IsEchoReply4(reply, 0x1234) || IsEchoReply4(reply, 0x4321) || IsEchoRequest4(reply) {
		t.Error("the reply is not seen as the reply")
	}
	if Src(reply) != dst || Dst(reply) != src {
		t.Errorf("the reply goes %v -> %v", Src(reply), Dst(reply))
	}
	if Checksum(reply[:ipv4HeaderLen]) != 0 || Checksum(reply[ipv4HeaderLen:]) != 0 {
		t.Error("the reply's checksums do not check")
	}
	if EchoReply4(reply) != nil {
		t.Error("a reply to a reply")
	}
}

func TestEcho6(t *testing.T) {
	src, dst := netip.MustParseAddr("fd00:9::2"), netip.MustParseAddr("fd00:9::1")
	request := EchoRequest6(src, dst, 0x1234, 5)
	if Version(request) != 6 || Src(request) != src || Dst(request) != dst {
		t.Errorf("version %d, %v -> %v", Version(request), Src(request), Dst(request))
	}
	if !IsEchoRequest6(request) || IsEchoRequest4(request) {
		t.Error("the request is not seen as an IPv6 request")
	}
	if checksum6(src, dst, request[ipv6HeaderLen:]) != 0 {
		t.Error("the request's checksum does not check")
	}
	reply := EchoReply6(request)
	if !IsEchoReply6(reply, 0x1234) || IsEchoRequest6(reply) || Src(reply) != dst || Dst(reply) != src {
		t.Error("the reply is not the reply")
	}
	if checksum6(dst, src, reply[ipv6HeaderLen:]) != 0 {
		t.Error("the reply's checksum does not check")
	}
}

func TestNotPackets(t *testing.T) {
	for _, p := range [][]byte{nil, {}, {0x45}, {0x60, 1, 2}, {0x10, 0, 0, 0}} {
		if Version(p) != 0 || Src(p).IsValid() || Dst(p).IsValid() || IsEchoRequest4(p) || IsEchoRequest6(p) {
			t.Errorf("%x is taken for a packet", p)
		}
	}
}

func TestUDP4(t *testing.T) {
	src := netip.MustParseAddrPort("10.9.0.2:40000")
	dst := netip.MustParseAddrPort("10.9.0.1:53")
	packet := UDP4(src, dst, []byte("query"))
	if Checksum(packet[:20]) != 0 {
		t.Error("the IPv4 header's checksum is wrong")
	}
	gotSrc, gotDst, payload, ok := ParseUDP4(packet)
	if !ok || gotSrc != src || gotDst != dst || string(payload) != "query" {
		t.Errorf("ParseUDP4 = %v, %v, %q, %v", gotSrc, gotDst, payload, ok)
	}
	cut := func(n int) []byte { return packet[:n] }
	wrongLength := append([]byte(nil), packet...)
	wrongLength[25] = 200 // the UDP length is more than there is
	for name, p := range map[string][]byte{
		"an echo request":       EchoRequest4(src.Addr(), dst.Addr(), 1, 1),
		"cut in the IP header":  cut(10),
		"cut in the UDP header": cut(24),
		"a UDP length too long": wrongLength,
		"nothing":               nil,
	} {
		if _, _, _, ok := ParseUDP4(p); ok {
			t.Errorf("%s is read as UDP", name)
		}
	}
}

func TestSYN4(t *testing.T) {
	src := netip.MustParseAddrPort("10.9.0.2:40000")
	dst := netip.MustParseAddrPort("10.9.0.1:8080")
	packet := SYN4(src, dst, 1380)
	if Checksum(packet[:20]) != 0 {
		t.Error("the IPv4 header's checksum is wrong")
	}
	pseudo := []byte{10, 9, 0, 2, 10, 9, 0, 1, 0, protoTCP, 0, 24}
	if Checksum(append(pseudo, packet[20:]...)) != 0 {
		t.Error("the TCP checksum is wrong")
	}
	sp, dp, ack, mss, ok := SYNMSS4(packet)
	if !ok || sp != 40000 || dp != 8080 || ack || mss != 1380 {
		t.Errorf("SYNMSS4 = %d, %d, %v, %d, %v", sp, dp, ack, mss, ok)
	}
	synack := append([]byte(nil), packet...)
	synack[20+13] |= 0x10
	if _, _, ack, _, ok := SYNMSS4(synack); !ok || !ack {
		t.Errorf("SYNMSS4 of a SYN-ACK = ack %v, ok %v", ack, ok)
	}
	noMSS := append([]byte(nil), packet...)
	noMSS[20+20] = 1 // a no-operation option, then an option that is cut
	noMSS[20+21] = 1
	noMSS[20+22] = 1
	noMSS[20+23] = 1
	notSYN := append([]byte(nil), packet...)
	notSYN[20+13] = 0x10
	for name, p := range map[string][]byte{
		"an echo request":       EchoRequest4(src.Addr(), dst.Addr(), 1, 1),
		"a SYN with no MSS":     noMSS,
		"not a SYN":             notSYN,
		"cut in the TCP header": packet[:30],
		"nothing":               nil,
	} {
		if _, _, _, _, ok := SYNMSS4(p); ok {
			t.Errorf("%s is read as a SYN with an MSS", name)
		}
	}
}
