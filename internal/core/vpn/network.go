// SPDX-License-Identifier: GPL-2.0-only

package vpn

import (
	"net/netip"

	"github.com/rm4n0s/errors"
	openvpn "github.com/rm4n0s/gopenvpn-conn"
	"github.com/rm4n0s/tube/internal/core/sanitize"
)

// The limits of what NetworkFromState keeps.
const (
	// MinMTU is the smallest MTU of a tunnel, IPv4's minimum.
	MinMTU = 576
	// MaxMTU is the largest MTU of a tunnel.
	MaxMTU = 65535
	// maxDNSServers is how many pushed DNS servers are kept.
	maxDNSServers = 8
	// maxSearchDomains is how many pushed search domains are kept.
	maxSearchDomains = 6
)

// The Tags of NetworkFromState's errors.
const (
	TagNoAddress        = "VPNNoAddress"
	TagTooManyAddresses = "VPNTooManyAddresses"
	TagBadMTU           = "VPNBadMTU"
)

// Network is what the namespace needs to know of a tunnel.
type Network struct {
	// Addrs are the tunnel's addresses: at most one IPv4 and one IPv6, IPv4 first.
	Addrs []netip.Prefix
	// DNSServers are the DNS servers that the server pushed.
	DNSServers []netip.Addr
	// SearchDomains are the search domains that the server pushed, only valid host names.
	SearchDomains []string
	// MTU is the MTU of the tunnel, from MinMTU to MaxMTU.
	MTU int
}

// NetworkFromState checks what the server pushed and keeps what the namespace may use. What
// is written into a file of the namespace later (the search domains) is only what
// sanitize.HostName accepts.
func NetworkFromState(state openvpn.ConnectionState) (Network, error) {
	var v4, v6 []netip.Prefix
	for _, p := range state.Addrs {
		if !p.IsValid() {
			continue
		}
		if p.Addr().Is4In6() && p.Bits() >= 96 {
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		if p.Addr().Is4() {
			v4 = append(v4, p)
		} else {
			v6 = append(v6, p)
		}
	}
	if len(v4)+len(v6) == 0 {
		return Network{}, errors.New(TagNoAddress, "the server gave the tunnel no address")
	}
	if len(v4) > 1 || len(v6) > 1 {
		return Network{}, errors.New(TagTooManyAddresses, "the server gave the tunnel two addresses of one family",
			"ipv4", len(v4), "ipv6", len(v6))
	}
	if state.MTU < MinMTU || state.MTU > MaxMTU {
		return Network{}, errors.New(TagBadMTU, "the MTU of the tunnel is not one that Tube can use", "mtu", state.MTU)
	}
	n := Network{Addrs: append(v4, v6...), MTU: state.MTU}
	for _, addr := range state.DNSServers {
		addr = addr.Unmap()
		if addr.IsValid() && !addr.IsUnspecified() && len(n.DNSServers) < maxDNSServers {
			n.DNSServers = append(n.DNSServers, addr)
		}
	}
	for _, name := range state.SearchDomains {
		if sanitize.HostName(name) && len(n.SearchDomains) < maxSearchDomains {
			n.SearchDomains = append(n.SearchDomains, name)
		}
	}
	return n, nil
}
