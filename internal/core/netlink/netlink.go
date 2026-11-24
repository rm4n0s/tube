// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

// Package netlink sends the few rtnetlink requests that Tube needs to set up the network of
// its namespace (docs/SPEC.md §8.3): bringing a link up, its MTU, its addresses and a default
// route, and listing links, addresses and routes for the tests. Tube never runs the ip command.
package netlink

import (
	"bytes"
	"net/netip"
	"syscall"

	"github.com/rm4n0s/errors"
	"golang.org/x/sys/unix"
)

// The Tags of this package's errors.
const (
	TagSocketFailed   = "NetlinkSocketFailed"
	TagBindFailed     = "NetlinkBindFailed"
	TagCloseFailed    = "NetlinkCloseFailed"
	TagSendFailed     = "NetlinkSendFailed"
	TagReceiveFailed  = "NetlinkReceiveFailed"
	TagParseFailed    = "NetlinkParseFailed"
	TagRefused        = "NetlinkRefused"
	TagLinkNotFound   = "NetlinkLinkNotFound"
	TagFamilyInvalid  = "NetlinkFamilyInvalid"
	TagAddressInvalid = "NetlinkAddressInvalid"
)

// receiveBuffer is the size of one read of the socket.
const receiveBuffer = 64 << 10

// Link is a network interface.
type Link struct {
	Index int
	Name  string
	Up    bool
	MTU   int
}

// Route is a route of the main table. A default route has a Dst of 0 bits.
type Route struct {
	Family   int
	Dst      netip.Prefix
	OutIndex int
}

// Conn is a socket of rtnetlink. Its methods must not be called from two goroutines at once.
type Conn struct {
	fd  int
	seq uint32
	// recv reads one answer; tests replace it.
	recv func(fd int, buf []byte) (int, error)
}

// Open opens a socket of rtnetlink in the network namespace of the calling process.
func Open() (*Conn, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, errors.NewErr(TagSocketFailed, err)
	}
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		// The bind's error is the one that matters.
		_ = unix.Close(fd)
		return nil, errors.NewErr(TagBindFailed, err)
	}
	return &Conn{fd: fd, recv: recvfrom}, nil
}

// recvfrom reads one datagram of the socket.
func recvfrom(fd int, buf []byte) (int, error) {
	n, _, err := unix.Recvfrom(fd, buf, 0)
	return n, err
}

// Close closes the socket.
func (c *Conn) Close() error {
	fd := c.fd
	c.fd = -1
	if err := unix.Close(fd); err != nil {
		return errors.NewErr(TagCloseFailed, err)
	}
	return nil
}

// do sends a request that the kernel acknowledges and waits for the acknowledgement.
func (c *Conn) do(op string, msgType, flags uint16, header []byte, attrs ...attr) error {
	_, err := c.roundTrip(op, msgType, flags|unix.NLM_F_REQUEST|unix.NLM_F_ACK, header, attrs...)
	return err
}

// dump sends a request for a list and returns its messages.
func (c *Conn) dump(op string, msgType uint16, header []byte) ([]syscall.NetlinkMessage, error) {
	return c.roundTrip(op, msgType, unix.NLM_F_REQUEST|unix.NLM_F_DUMP, header)
}

// roundTrip sends one request and reads the answers to it: until the acknowledgement, or, for
// a dump, until NLMSG_DONE, collecting the messages before it. An NLMSG_ERROR with an errno is
// the kernel's refusal, which errors.Is finds behind the error.
func (c *Conn) roundTrip(op string, msgType, flags uint16, header []byte, attrs ...attr) ([]syscall.NetlinkMessage, error) {
	c.seq++
	seq := c.seq
	req := request(msgType, flags, seq, header, attrs...)
	if err := unix.Sendto(c.fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, errors.NewErr(TagSendFailed, err).SetMetadata("op", op)
	}
	isDump := flags&unix.NLM_F_DUMP == unix.NLM_F_DUMP
	var out []syscall.NetlinkMessage
	for {
		// A buffer for each read: the messages that are kept point into it.
		buf := make([]byte, receiveBuffer)
		n, err := c.recv(c.fd, buf)
		if err != nil {
			return nil, errors.NewErr(TagReceiveFailed, err).SetMetadata("op", op)
		}
		messages, err := syscall.ParseNetlinkMessage(buf[:n])
		if err == nil && len(messages) == 0 {
			// Fewer bytes than a header: the kernel never sends that, and reading again
			// would not end.
			err = syscall.EBADMSG
		}
		if err != nil {
			return nil, errors.NewErr(TagParseFailed, err).SetMetadata("op", op)
		}
		for _, m := range messages {
			if m.Header.Seq != seq {
				continue // an answer to an earlier request
			}
			switch m.Header.Type {
			case unix.NLMSG_ERROR:
				if errno := ackErrno(m); errno != 0 {
					return nil, errors.NewErr(TagRefused, errno).SetMetadata("op", op, "errno", int(errno))
				}
				if !isDump {
					return nil, nil
				}
			case unix.NLMSG_DONE:
				if isDump {
					return out, nil
				}
			default:
				if isDump {
					out = append(out, m)
				}
			}
		}
	}
}

// LinkUp brings the link up.
func (c *Conn) LinkUp(index int) error {
	return c.do("link up", unix.RTM_NEWLINK, 0, ifInfo(index, unix.IFF_UP, unix.IFF_UP))
}

// SetMTU sets the MTU of the link.
func (c *Conn) SetMTU(index, mtu int) error {
	return c.do("set the MTU", unix.RTM_NEWLINK, 0, ifInfo(index, 0, 0), attr{unix.IFLA_MTU, u32(uint32(mtu))})
}

// addrMessage is the header and the attributes of a request about an address. An IPv6
// address is added without duplicate address detection, so that it is usable at once.
func addrMessage(index int, prefix netip.Prefix) ([]byte, []attr, error) {
	addr := prefix.Addr()
	if !prefix.IsValid() || addr.Zone() != "" {
		return nil, nil, errors.New(TagAddressInvalid, "the address cannot be given to a link", "address", prefix.String())
	}
	family, flags := uint8(unix.AF_INET), uint8(0)
	if addr.Is6() && !addr.Is4In6() {
		family, flags = unix.AF_INET6, unix.IFA_F_NODAD
	} else {
		addr = addr.Unmap()
	}
	bits := addr.AsSlice()
	return ifAddr(family, uint8(prefix.Bits()), flags, index), []attr{{unix.IFA_LOCAL, bits}, {unix.IFA_ADDRESS, bits}}, nil
}

// AddAddr adds an address to the link.
func (c *Conn) AddAddr(index int, prefix netip.Prefix) error {
	header, attrs, err := addrMessage(index, prefix)
	if err != nil {
		return err
	}
	return c.do("add an address", unix.RTM_NEWADDR, unix.NLM_F_CREATE|unix.NLM_F_EXCL, header, attrs...)
}

// DelAddr removes an address from the link.
func (c *Conn) DelAddr(index int, prefix netip.Prefix) error {
	header, attrs, err := addrMessage(index, prefix)
	if err != nil {
		return err
	}
	return c.do("remove an address", unix.RTM_DELADDR, 0, header, attrs...)
}

// AddDefaultRoute adds a default route of the family (unix.AF_INET or unix.AF_INET6) out of
// the link. A default route that is there already is not an error.
func (c *Conn) AddDefaultRoute(index, family int) error {
	var scope uint8
	switch family {
	case unix.AF_INET:
		scope = unix.RT_SCOPE_LINK
	case unix.AF_INET6:
		scope = unix.RT_SCOPE_UNIVERSE
	default:
		return errors.New(TagFamilyInvalid, "a default route is of IPv4 or IPv6", "family", family)
	}
	header := rtMsg(uint8(family), 0, unix.RT_TABLE_MAIN, unix.RTPROT_STATIC, scope, unix.RTN_UNICAST)
	err := c.do("add a default route", unix.RTM_NEWROUTE, unix.NLM_F_CREATE|unix.NLM_F_EXCL, header,
		attr{unix.RTA_OIF, u32(uint32(index))})
	if errors.Is(err, unix.EEXIST) {
		return nil
	}
	return err
}

// Links lists the links of the namespace.
func (c *Conn) Links() ([]Link, error) {
	messages, err := c.dump("list the links", unix.RTM_GETLINK, ifInfo(0, 0, 0))
	if err != nil {
		return nil, err
	}
	var out []Link
	for _, m := range messages {
		if m.Header.Type != unix.RTM_NEWLINK || len(m.Data) < unix.SizeofIfInfomsg {
			continue
		}
		info := unix.IfInfomsg{Index: int32(nativeU32(m.Data[4:])), Flags: nativeU32(m.Data[8:])}
		link := Link{Index: int(info.Index), Up: info.Flags&unix.IFF_UP != 0}
		attrs, err := syscall.ParseNetlinkRouteAttr(&m)
		if err != nil {
			continue // a message that cannot be read is not one of the links Tube looks for
		}
		for _, a := range attrs {
			switch a.Attr.Type {
			case unix.IFLA_IFNAME:
				link.Name = string(bytes.TrimRight(a.Value, "\x00"))
			case unix.IFLA_MTU:
				if len(a.Value) >= 4 {
					link.MTU = int(nativeU32(a.Value))
				}
			}
		}
		out = append(out, link)
	}
	return out, nil
}

// LinkIndex returns the index of the link with the name.
func (c *Conn) LinkIndex(name string) (int, error) {
	links, err := c.Links()
	if err != nil {
		return 0, err
	}
	for _, l := range links {
		if l.Name == name {
			return l.Index, nil
		}
	}
	return 0, errors.New(TagLinkNotFound, "there is no link with this name", "name", name)
}

// Addrs lists the addresses of the link.
func (c *Conn) Addrs(index int) ([]netip.Prefix, error) {
	messages, err := c.dump("list the addresses", unix.RTM_GETADDR, ifAddr(unix.AF_UNSPEC, 0, 0, 0))
	if err != nil {
		return nil, err
	}
	var out []netip.Prefix
	for _, m := range messages {
		if m.Header.Type != unix.RTM_NEWADDR || len(m.Data) < unix.SizeofIfAddrmsg || int(nativeU32(m.Data[4:])) != index {
			continue
		}
		bits := int(m.Data[1])
		attrs, err := syscall.ParseNetlinkRouteAttr(&m)
		if err != nil {
			continue
		}
		var local, address netip.Addr
		for _, a := range attrs {
			addr, ok := netip.AddrFromSlice(a.Value)
			if !ok {
				continue
			}
			switch a.Attr.Type {
			case unix.IFA_LOCAL:
				local = addr
			case unix.IFA_ADDRESS:
				address = addr
			}
		}
		if !local.IsValid() {
			local = address
		}
		if local.IsValid() {
			out = append(out, netip.PrefixFrom(local, bits))
		}
	}
	return out, nil
}

// Routes lists the routes of the main table.
func (c *Conn) Routes() ([]Route, error) {
	messages, err := c.dump("list the routes", unix.RTM_GETROUTE, rtMsg(unix.AF_UNSPEC, 0, 0, 0, 0, 0))
	if err != nil {
		return nil, err
	}
	var out []Route
	for _, m := range messages {
		if m.Header.Type != unix.RTM_NEWROUTE || len(m.Data) < unix.SizeofRtMsg {
			continue
		}
		family, dstLen, table := int(m.Data[0]), int(m.Data[1]), uint32(m.Data[4])
		attrs, err := syscall.ParseNetlinkRouteAttr(&m)
		if err != nil {
			continue
		}
		r := Route{Family: family}
		var dst netip.Addr
		for _, a := range attrs {
			switch a.Attr.Type {
			case unix.RTA_TABLE:
				if len(a.Value) >= 4 {
					table = nativeU32(a.Value)
				}
			case unix.RTA_DST:
				dst, _ = netip.AddrFromSlice(a.Value)
			case unix.RTA_OIF:
				if len(a.Value) >= 4 {
					r.OutIndex = int(nativeU32(a.Value))
				}
			}
		}
		if table != unix.RT_TABLE_MAIN {
			continue
		}
		if !dst.IsValid() {
			if family == unix.AF_INET6 {
				dst = netip.IPv6Unspecified()
			} else {
				dst = netip.IPv4Unspecified()
			}
		}
		r.Dst = netip.PrefixFrom(dst, dstLen)
		out = append(out, r)
	}
	return out, nil
}
