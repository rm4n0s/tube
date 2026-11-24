// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package netlink

import (
	"bytes"
	"encoding/binary"
	"syscall"
	"testing"
)

// header is a netlink header written by hand, in the machine's byte order.
func header(length uint32, typ, flags uint16, seq uint32) []byte {
	b := binary.NativeEndian.AppendUint32(nil, length)
	b = binary.NativeEndian.AppendUint16(b, typ)
	b = binary.NativeEndian.AppendUint16(b, flags)
	b = binary.NativeEndian.AppendUint32(b, seq)
	return binary.NativeEndian.AppendUint32(b, 0) // pid
}

func TestRequest(t *testing.T) {
	family := []byte{1, 2, 3, 4}
	tests := []struct {
		name  string
		attrs []attr
		want  []byte
	}{
		{
			name: "no attribute",
			want: append(header(20, 16, 5, 7), family...),
		},
		{
			name:  "an attribute of 4 bytes",
			attrs: []attr{{typ: 4, data: []byte{9, 9, 9, 9}}},
			want:  append(append(header(28, 16, 5, 7), family...), attrBytes(8, 4, []byte{9, 9, 9, 9})...),
		},
		{
			name:  "an attribute of 5 bytes is padded to 8",
			attrs: []attr{{typ: 3, data: []byte("tube0")}},
			want:  append(append(header(32, 16, 5, 7), family...), attrBytes(9, 3, []byte("tube0\x00\x00\x00"))...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := request(16, 5, 7, family, tt.attrs...)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("request =\n%x\nwant\n%x", got, tt.want)
			}
			if len(got)%4 != 0 {
				t.Errorf("the length %d is not a multiple of 4", len(got))
			}
		})
	}
}

// attrBytes is an attribute's header (its length without the padding, its type) and its data.
func attrBytes(length, typ uint16, data []byte) []byte {
	b := binary.NativeEndian.AppendUint16(nil, length)
	b = binary.NativeEndian.AppendUint16(b, typ)
	return append(b, data...)
}

func TestAckErrno(t *testing.T) {
	errMessage := func(code int32) syscall.NetlinkMessage {
		return syscall.NetlinkMessage{Data: binary.NativeEndian.AppendUint32(nil, uint32(code))}
	}
	tests := []struct {
		name string
		m    syscall.NetlinkMessage
		want syscall.Errno
	}{
		{"an acknowledgement", errMessage(0), 0},
		{"a refusal", errMessage(-int32(syscall.EPERM)), syscall.EPERM},
		{"a message cut short", syscall.NetlinkMessage{Data: []byte{1, 2}}, syscall.EBADMSG},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ackErrno(tt.m); got != tt.want {
				t.Errorf("ackErrno = %v, want %v", got, tt.want)
			}
		})
	}
}
