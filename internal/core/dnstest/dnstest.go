// SPDX-License-Identifier: GPL-2.0-only

// Package dnstest builds and reads the DNS messages of the tests (RFC 1035), by hand: a query,
// its question, and an answer of one A or AAAA record.
package dnstest

import (
	"encoding/binary"
	"net/netip"
	"strings"
)

// The types of records, and the class of the Internet.
const (
	TypeA     = 1
	TypeAAAA  = 28
	classIN   = 1
	headerLen = 12
	// flagQR marks an answer; flagRD and flagRA say that recursion was desired and is
	// available; rcodeNXDomain is the code of a name that does not exist.
	flagQR        = 1 << 15
	flagRD        = 1 << 8
	flagRA        = 1 << 7
	rcodeNXDomain = 3
	answerTTL     = 60
)

// Query builds a query for the name and the type.
func Query(id uint16, name string, qtype uint16) []byte {
	m := make([]byte, headerLen, headerLen+len(name)+6)
	binary.BigEndian.PutUint16(m[0:], id)
	binary.BigEndian.PutUint16(m[2:], flagRD)
	binary.BigEndian.PutUint16(m[4:], 1) // one question
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		m = append(m, byte(len(label)))
		m = append(m, label...)
	}
	m = append(m, 0)
	m = binary.BigEndian.AppendUint16(m, qtype)
	return binary.BigEndian.AppendUint16(m, classIN)
}

// question reads the first question of a message: the name, the type, and where the question
// ends.
func question(m []byte) (name string, qtype uint16, end int, ok bool) {
	if len(m) < headerLen || binary.BigEndian.Uint16(m[4:]) == 0 {
		return "", 0, 0, false
	}
	var labels []string
	i := headerLen
	for {
		if i >= len(m) {
			return "", 0, 0, false
		}
		n := int(m[i])
		i++
		if n == 0 {
			break
		}
		// A pointer (the two top bits) never appears in the question of a query.
		if n > 63 || i+n > len(m) {
			return "", 0, 0, false
		}
		labels = append(labels, strings.ToLower(string(m[i:i+n])))
		i += n
	}
	if i+4 > len(m) {
		return "", 0, 0, false
	}
	return strings.Join(labels, "."), binary.BigEndian.Uint16(m[i:]), i + 4, true
}

// Question reads the first question of a DNS query: the name (lower case, no ending dot), the
// type (1 is A, 28 is AAAA) and the query's id.
func Question(query []byte) (name string, qtype uint16, id uint16, ok bool) {
	name, qtype, _, ok = question(query)
	if !ok {
		return "", 0, 0, false
	}
	return name, qtype, binary.BigEndian.Uint16(query[0:]), true
}

// Answer builds the answer to a query: one A record with the address when the query asks for A
// and addr is IPv4 (one AAAA record for AAAA and IPv6), an empty answer (no error, no record)
// for another type or an invalid addr, and NXDOMAIN when found is false. A query that cannot be
// read gets nil.
func Answer(query []byte, addr netip.Addr, found bool) []byte {
	_, qtype, end, ok := question(query)
	if !ok {
		return nil
	}
	m := append([]byte(nil), query[:end]...)
	flags := flagQR | flagRA | binary.BigEndian.Uint16(query[2:])&flagRD
	var records uint16
	var rdata []byte
	switch {
	case !found:
		flags |= rcodeNXDomain
	case qtype == TypeA && addr.Is4():
		a := addr.As4()
		records, rdata = 1, a[:]
	case qtype == TypeAAAA && addr.Is6() && !addr.Is4In6():
		a := addr.As16()
		records, rdata = 1, a[:]
	}
	binary.BigEndian.PutUint16(m[2:], flags)
	binary.BigEndian.PutUint16(m[4:], 1)       // the question
	binary.BigEndian.PutUint16(m[6:], records) // the answers
	binary.BigEndian.PutUint16(m[8:], 0)       // no authority
	binary.BigEndian.PutUint16(m[10:], 0)      // no additional record (the query's EDNS is dropped)
	if records == 1 {
		m = binary.BigEndian.AppendUint16(m, 0xc000|headerLen) // the name of the question
		m = binary.BigEndian.AppendUint16(m, qtype)
		m = binary.BigEndian.AppendUint16(m, classIN)
		m = binary.BigEndian.AppendUint32(m, answerTTL)
		m = binary.BigEndian.AppendUint16(m, uint16(len(rdata)))
		m = append(m, rdata...)
	}
	return m
}
