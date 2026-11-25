// SPDX-License-Identifier: GPL-2.0-only

package dnstest

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"testing"
)

func TestQuery(t *testing.T) {
	q := Query(0x1234, "Tube-Test.Example.", TypeAAAA)
	want := []byte{
		0x12, 0x34, 0x01, 0x00, 0, 1, 0, 0, 0, 0, 0, 0, // id, RD, one question
		9, 'T', 'u', 'b', 'e', '-', 'T', 'e', 's', 't',
		7, 'E', 'x', 'a', 'm', 'p', 'l', 'e', 0,
		0, 28, 0, 1, // AAAA, IN
	}
	if !bytes.Equal(q, want) {
		t.Errorf("Query = % x\nwant    % x", q, want)
	}
	name, qtype, id, ok := Question(q)
	if !ok || name != "tube-test.example" || qtype != TypeAAAA || id != 0x1234 {
		t.Errorf("Question = %q, %d, %#x, %v", name, qtype, id, ok)
	}
}

func TestQuestionOfBrokenQueries(t *testing.T) {
	q := Query(1, "a.example", TypeA)
	noQuestion := append([]byte(nil), q...)
	binary.BigEndian.PutUint16(noQuestion[4:], 0)
	tests := map[string][]byte{
		"empty":                  nil,
		"only a header":          q[:headerLen],
		"cut inside a label":     q[:headerLen+3],
		"cut before the type":    q[:len(q)-3],
		"no question":            noQuestion,
		"a label longer than 63": append(append([]byte(nil), q[:headerLen]...), 64, 'x'),
	}
	for name, m := range tests {
		if _, _, _, ok := Question(m); ok {
			t.Errorf("%s: Question is ok", name)
		}
		if Answer(m, netip.MustParseAddr("192.0.2.7"), true) != nil {
			t.Errorf("%s: Answer is not nil", name)
		}
	}
}

// header is the header of a message: id, flags, and the four counts.
func header(m []byte) [6]uint16 {
	var h [6]uint16
	for i := range h {
		h[i] = binary.BigEndian.Uint16(m[2*i:])
	}
	return h
}

func TestAnswer(t *testing.T) {
	v4 := netip.MustParseAddr("192.0.2.7")
	v6 := netip.MustParseAddr("2001:db8::7")
	tests := []struct {
		name  string
		qtype uint16
		addr  netip.Addr
		found bool
		flags uint16
		rdata []byte
	}{
		{"A", TypeA, v4, true, 0x8180, v4.AsSlice()},
		{"AAAA", TypeAAAA, v6, true, 0x8180, v6.AsSlice()},
		{"AAAA of an IPv4 address", TypeAAAA, v4, true, 0x8180, nil},
		{"A of an IPv6 address", TypeA, v6, true, 0x8180, nil},
		{"another type", 16, v4, true, 0x8180, nil},
		{"an invalid address", TypeA, netip.Addr{}, true, 0x8180, nil},
		{"NXDOMAIN", TypeA, v4, false, 0x8183, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query(77, "tube-test.example", tt.qtype)
			a := Answer(q, tt.addr, tt.found)
			records := uint16(0)
			if tt.rdata != nil {
				records = 1
			}
			if got, want := header(a), [6]uint16{77, tt.flags, 1, records, 0, 0}; got != want {
				t.Errorf("header = %v, want %v", got, want)
			}
			if !bytes.Equal(a[headerLen:len(q)], q[headerLen:]) {
				t.Error("the question is not the query's")
			}
			rest := a[len(q):]
			if tt.rdata == nil {
				if len(rest) != 0 {
					t.Errorf("records where there should be none: % x", rest)
				}
				return
			}
			want := []byte{0xc0, 12, 0, byte(tt.qtype), 0, 1, 0, 0, 0, 60, 0, byte(len(tt.rdata))}
			want = append(want, tt.rdata...)
			if !bytes.Equal(rest, want) {
				t.Errorf("the record is % x\nwant          % x", rest, want)
			}
		})
	}
}
