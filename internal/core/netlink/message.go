// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package netlink

import (
	"encoding/binary"
	"syscall"

	"golang.org/x/sys/unix"
)

// attr is one route attribute of a request.
type attr struct {
	typ  uint16
	data []byte
}

// align4 rounds n up to a multiple of 4, as netlink pads every part of a message.
func align4(n int) int { return (n + 3) &^ 3 }

// request builds a netlink message: the header with its length filled in, the family header,
// and each attribute, every part padded to 4 bytes. The numbers are in the machine's byte
// order, as netlink wants.
func request(msgType, flags uint16, seq uint32, header []byte, attrs ...attr) []byte {
	b := make([]byte, unix.SizeofNlMsghdr, 128)
	b = append(b, header...)
	b = append(b, make([]byte, align4(len(b))-len(b))...)
	for _, a := range attrs {
		size := unix.SizeofRtAttr + len(a.data)
		b = binary.NativeEndian.AppendUint16(b, uint16(size))
		b = binary.NativeEndian.AppendUint16(b, a.typ)
		b = append(b, a.data...)
		b = append(b, make([]byte, align4(len(b))-len(b))...)
	}
	binary.NativeEndian.PutUint32(b[0:], uint32(len(b)))
	binary.NativeEndian.PutUint16(b[4:], msgType)
	binary.NativeEndian.PutUint16(b[6:], flags)
	binary.NativeEndian.PutUint32(b[8:], seq)
	return b
}

// ifInfo is an IfInfomsg in the machine's byte order.
func ifInfo(index int, flags, change uint32) []byte {
	b := make([]byte, unix.SizeofIfInfomsg)
	b[0] = unix.AF_UNSPEC
	binary.NativeEndian.PutUint32(b[4:], uint32(int32(index)))
	binary.NativeEndian.PutUint32(b[8:], flags)
	binary.NativeEndian.PutUint32(b[12:], change)
	return b
}

// ifAddr is an IfAddrmsg in the machine's byte order.
func ifAddr(family, prefixLen, flags uint8, index int) []byte {
	b := make([]byte, unix.SizeofIfAddrmsg)
	b[0], b[1], b[2] = family, prefixLen, flags
	binary.NativeEndian.PutUint32(b[4:], uint32(index))
	return b
}

// rtMsg is an RtMsg in the machine's byte order.
func rtMsg(family, dstLen, table, protocol, scope, typ uint8) []byte {
	b := make([]byte, unix.SizeofRtMsg)
	b[0], b[1], b[4], b[5], b[6], b[7] = family, dstLen, table, protocol, scope, typ
	return b
}

// u32 is a uint32 in the machine's byte order, the form of IFLA_MTU and RTA_OIF.
func u32(v uint32) []byte { return binary.NativeEndian.AppendUint32(nil, v) }

// ackErrno reads the error of an NLMSG_ERROR message: 0 for an acknowledgement, else the
// errno that the kernel answered with. A message too short to hold it is EBADMSG.
func ackErrno(m syscall.NetlinkMessage) syscall.Errno {
	if len(m.Data) < 4 {
		return syscall.EBADMSG
	}
	return syscall.Errno(-int32(binary.NativeEndian.Uint32(m.Data)))
}

// nativeU32 reads a uint32 in the machine's byte order.
func nativeU32(b []byte) uint32 { return binary.NativeEndian.Uint32(b) }
