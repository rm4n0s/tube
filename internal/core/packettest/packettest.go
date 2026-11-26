// SPDX-License-Identifier: GPL-2.0-only

// Package packettest builds and reads IP packets by hand, for the tests that play a VPN or a
// server: ICMP echo requests and replies, over IPv4 and IPv6, and UDP datagrams and TCP SYNs over
// IPv4.
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

// protoUDP is the protocol number of UDP.
const protoUDP = 17

// udpHeaderLen is the length of a UDP header.
const udpHeaderLen = 8

// UDP4 builds an IPv4 packet with a UDP datagram from src to dst. Its UDP checksum is 0, which
// IPv4 allows (no checksum).
func UDP4(src, dst netip.AddrPort, payload []byte) []byte {
	udp := make([]byte, udpHeaderLen+len(payload))
	binary.BigEndian.PutUint16(udp[0:], src.Port())
	binary.BigEndian.PutUint16(udp[2:], dst.Port())
	binary.BigEndian.PutUint16(udp[4:], uint16(len(udp)))
	copy(udp[udpHeaderLen:], payload)
	return append(ipv4(src.Addr(), dst.Addr(), protoUDP, len(udp)), udp...)
}

// ParseUDP4 reads an IPv4 packet that carries a UDP datagram: its source, its destination and
// its payload. ok is false for any other packet, or one cut short.
func ParseUDP4(packet []byte) (src, dst netip.AddrPort, payload []byte, ok bool) {
	if Version(packet) != 4 || packet[9] != protoUDP {
		return src, dst, nil, false
	}
	headerLen := int(packet[0]&0x0f) * 4
	total := int(binary.BigEndian.Uint16(packet[2:]))
	if headerLen < ipv4HeaderLen || total > len(packet) || total < headerLen+udpHeaderLen {
		return src, dst, nil, false
	}
	udp := packet[headerLen:total]
	length := int(binary.BigEndian.Uint16(udp[4:]))
	if length < udpHeaderLen || length > len(udp) {
		return src, dst, nil, false
	}
	src = netip.AddrPortFrom(Src(packet), binary.BigEndian.Uint16(udp[0:]))
	dst = netip.AddrPortFrom(Dst(packet), binary.BigEndian.Uint16(udp[2:]))
	return src, dst, udp[udpHeaderLen:length], true
}

// protoTCP is the protocol number of TCP; tcpSYN is its SYN flag; tcpOptMSS is the kind of the
// maximum segment size option.
const (
	protoTCP  = 6
	tcpSYN    = 0x02
	tcpOptMSS = 2
)

// SYN4 builds an IPv4 packet with a TCP SYN from src to dst that offers the maximum segment size
// mss (the only option), with a correct checksum.
func SYN4(src, dst netip.AddrPort, mss uint16) []byte {
	tcp := make([]byte, 24)
	binary.BigEndian.PutUint16(tcp[0:], src.Port())
	binary.BigEndian.PutUint16(tcp[2:], dst.Port())
	binary.BigEndian.PutUint32(tcp[4:], 1000) // the sequence number
	tcp[12] = 6 << 4                          // 6 words of header
	tcp[13] = tcpSYN
	binary.BigEndian.PutUint16(tcp[14:], 65535) // the window
	tcp[20], tcp[21] = tcpOptMSS, 4
	binary.BigEndian.PutUint16(tcp[22:], mss)
	pseudo := make([]byte, 12, 12+len(tcp))
	s, d := src.Addr().As4(), dst.Addr().As4()
	copy(pseudo[0:4], s[:])
	copy(pseudo[4:8], d[:])
	pseudo[9] = protoTCP
	binary.BigEndian.PutUint16(pseudo[10:], uint16(len(tcp)))
	binary.BigEndian.PutUint16(tcp[16:], Checksum(append(pseudo, tcp...)))
	return append(ipv4(src.Addr(), dst.Addr(), protoTCP, len(tcp)), tcp...)
}

// SYNMSS4 reads an IPv4 packet that is a TCP SYN or SYN-ACK with an MSS option: the source and
// the destination ports, whether the ACK flag is set, and the size. ok is false for any other
// packet, one cut short, or a SYN with no MSS option.
func SYNMSS4(packet []byte) (srcPort, dstPort uint16, ack bool, mss uint16, ok bool) {
	if Version(packet) != 4 || packet[9] != protoTCP {
		return 0, 0, false, 0, false
	}
	headerLen := int(packet[0]&0x0f) * 4
	total := int(binary.BigEndian.Uint16(packet[2:]))
	if headerLen < ipv4HeaderLen || total > len(packet) || total < headerLen+20 {
		return 0, 0, false, 0, false
	}
	tcp := packet[headerLen:total]
	dataOffset := int(tcp[12]>>4) * 4
	if tcp[13]&tcpSYN == 0 || dataOffset < 20 || dataOffset > len(tcp) {
		return 0, 0, false, 0, false
	}
	for opts := tcp[20:dataOffset]; len(opts) > 0; {
		switch kind := opts[0]; {
		case kind == 0: // the end of the options
			return 0, 0, false, 0, false
		case kind == 1: // no operation
			opts = opts[1:]
		case len(opts) < 2 || opts[1] < 2 || int(opts[1]) > len(opts):
			return 0, 0, false, 0, false
		case kind == tcpOptMSS && opts[1] == 4:
			return binary.BigEndian.Uint16(tcp[0:]), binary.BigEndian.Uint16(tcp[2:]), tcp[13]&0x10 != 0,
				binary.BigEndian.Uint16(opts[2:]), true
		default:
			opts = opts[opts[1]:]
		}
	}
	return 0, 0, false, 0, false
}
