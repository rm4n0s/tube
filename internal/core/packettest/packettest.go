// SPDX-License-Identifier: GPL-2.0-only

// Package packettest builds and reads IP packets by hand, for the tests that play a VPN or a
// server: ICMP echo requests and replies, over IPv4 and IPv6.
package packettest

import (
	"encoding/binary"
	"net/netip"
)

// The protocol numbers and ICMP types that the tests use.
const (
	protoICMP      = 1
	protoICMPv6    = 58
	icmpEchoReply  = 0
	icmpEcho       = 8
	icmp6Echo      = 128
	icmp6EchoReply = 129
	ipv4HeaderLen  = 20
	ipv6HeaderLen  = 40
	echoDataLen    = 16
)

// Checksum is the Internet checksum (RFC 1071) of b.
func Checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

// ipv4 builds an IPv4 header for a payload of the protocol.
func ipv4(src, dst netip.Addr, proto byte, payloadLen int) []byte {
	h := make([]byte, ipv4HeaderLen)
	h[0] = 0x45 // version 4, 5 words of header
	binary.BigEndian.PutUint16(h[2:], uint16(ipv4HeaderLen+payloadLen))
	h[8] = 64 // TTL
	h[9] = proto
	s, d := src.As4(), dst.As4()
	copy(h[12:16], s[:])
	copy(h[16:20], d[:])
	binary.BigEndian.PutUint16(h[10:], Checksum(h))
	return h
}

// EchoRequest4 builds an ICMP echo request from src to dst, with 16 bytes of data.
func EchoRequest4(src, dst netip.Addr, id, seq uint16) []byte {
	icmp := make([]byte, 8+echoDataLen)
	icmp[0] = icmpEcho
	binary.BigEndian.PutUint16(icmp[4:], id)
	binary.BigEndian.PutUint16(icmp[6:], seq)
	copy(icmp[8:], "tube packettest.")
	binary.BigEndian.PutUint16(icmp[2:], Checksum(icmp))
	return append(ipv4(src, dst, protoICMP, len(icmp)), icmp...)
}

// icmp4 returns the ICMP message of an IPv4 packet that carries one, or nil.
func icmp4(packet []byte) []byte {
	if Version(packet) != 4 || len(packet) < ipv4HeaderLen || packet[9] != protoICMP {
		return nil
	}
	headerLen := int(packet[0]&0x0f) * 4
	if headerLen < ipv4HeaderLen || len(packet) < headerLen+8 {
		return nil
	}
	return packet[headerLen:]
}

// IsEchoRequest4 says whether the packet is an IPv4 ICMP echo request.
func IsEchoRequest4(packet []byte) bool {
	icmp := icmp4(packet)
	return icmp != nil && icmp[0] == icmpEcho
}

// IsEchoReply4 says whether the packet is an IPv4 ICMP echo reply with the id.
func IsEchoReply4(packet []byte, id uint16) bool {
	icmp := icmp4(packet)
	return icmp != nil && icmp[0] == icmpEchoReply && binary.BigEndian.Uint16(icmp[4:]) == id
}

// EchoReply4 builds the reply to an IPv4 echo request: the addresses swapped, type 0, the same
// id, sequence and data. It returns nil when the packet is not an echo request.
func EchoReply4(request []byte) []byte {
	if !IsEchoRequest4(request) {
		return nil
	}
	headerLen := int(request[0]&0x0f) * 4
	icmp := append([]byte(nil), request[headerLen:]...)
	icmp[0] = icmpEchoReply
	icmp[2], icmp[3] = 0, 0
	binary.BigEndian.PutUint16(icmp[2:], Checksum(icmp))
	return append(ipv4(Dst(request), Src(request), protoICMP, len(icmp)), icmp...)
}

// ipv6 builds an IPv6 header for a payload of the next header.
func ipv6(src, dst netip.Addr, next byte, payloadLen int) []byte {
	h := make([]byte, ipv6HeaderLen)
	h[0] = 0x60
	binary.BigEndian.PutUint16(h[4:], uint16(payloadLen))
	h[6] = next
	h[7] = 64 // hop limit
	s, d := src.As16(), dst.As16()
	copy(h[8:24], s[:])
	copy(h[24:40], d[:])
	return h
}

// checksum6 is the ICMPv6 checksum of the message, over the pseudo-header of RFC 8200 §8.1.
func checksum6(src, dst netip.Addr, icmp []byte) uint16 {
	pseudo := make([]byte, 0, 40+len(icmp))
	s, d := src.As16(), dst.As16()
	pseudo = append(pseudo, s[:]...)
	pseudo = append(pseudo, d[:]...)
	pseudo = binary.BigEndian.AppendUint32(pseudo, uint32(len(icmp)))
	pseudo = append(pseudo, 0, 0, 0, protoICMPv6)
	pseudo = append(pseudo, icmp...)
	return Checksum(pseudo)
}

// EchoRequest6 builds an ICMPv6 echo request from src to dst, with 16 bytes of data.
func EchoRequest6(src, dst netip.Addr, id, seq uint16) []byte {
	icmp := make([]byte, 8+echoDataLen)
	icmp[0] = icmp6Echo
	binary.BigEndian.PutUint16(icmp[4:], id)
	binary.BigEndian.PutUint16(icmp[6:], seq)
	copy(icmp[8:], "tube packettest.")
	binary.BigEndian.PutUint16(icmp[2:], checksum6(src, dst, icmp))
	return append(ipv6(src, dst, protoICMPv6, len(icmp)), icmp...)
}

// icmp6 returns the ICMPv6 message of an IPv6 packet whose next header is ICMPv6, or nil.
func icmp6(packet []byte) []byte {
	if Version(packet) != 6 || len(packet) < ipv6HeaderLen+8 || packet[6] != protoICMPv6 {
		return nil
	}
	return packet[ipv6HeaderLen:]
}

// IsEchoRequest6 says whether the packet is an ICMPv6 echo request.
func IsEchoRequest6(packet []byte) bool {
	icmp := icmp6(packet)
	return icmp != nil && icmp[0] == icmp6Echo
}

// IsEchoReply6 says whether the packet is an ICMPv6 echo reply with the id.
func IsEchoReply6(packet []byte, id uint16) bool {
	icmp := icmp6(packet)
	return icmp != nil && icmp[0] == icmp6EchoReply && binary.BigEndian.Uint16(icmp[4:]) == id
}

// EchoReply6 builds the reply to an ICMPv6 echo request, or returns nil when the packet is not
// one.
func EchoReply6(request []byte) []byte {
	if !IsEchoRequest6(request) {
		return nil
	}
	src, dst := Dst(request), Src(request)
	icmp := append([]byte(nil), request[ipv6HeaderLen:]...)
	icmp[0] = icmp6EchoReply
	icmp[2], icmp[3] = 0, 0
	binary.BigEndian.PutUint16(icmp[2:], checksum6(src, dst, icmp))
	return append(ipv6(src, dst, protoICMPv6, len(icmp)), icmp...)
}

// Version is the IP version of the packet: 4, 6, or 0 when it is neither.
func Version(packet []byte) int {
	if len(packet) == 0 {
		return 0
	}
	switch packet[0] >> 4 {
	case 4:
		if len(packet) >= ipv4HeaderLen {
			return 4
		}
	case 6:
		if len(packet) >= ipv6HeaderLen {
			return 6
		}
	}
	return 0
}

// Src is the source address of the packet, or the zero Addr.
func Src(packet []byte) netip.Addr {
	switch Version(packet) {
	case 4:
		return netip.AddrFrom4([4]byte(packet[12:16]))
	case 6:
		return netip.AddrFrom16([16]byte(packet[8:24]))
	}
	return netip.Addr{}
}

// Dst is the destination address of the packet, or the zero Addr.
func Dst(packet []byte) netip.Addr {
	switch Version(packet) {
	case 4:
		return netip.AddrFrom4([4]byte(packet[16:20]))
	case 6:
		return netip.AddrFrom16([16]byte(packet[24:40]))
	}
	return netip.Addr{}
}
