// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"net/netip"
	"os"
	"path/filepath"
	"slices"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/netlink"
	"golang.org/x/sys/unix"
)

// The Tags of the keeper's configuring.
const (
	TagKeeperNetworkInvalid = "KeeperNetworkInvalid"
	TagKeeperSysctlFailed   = "KeeperSysctlFailed"
)

// The files of the kernel's settings that the keeper writes, in the namespace's own /proc/sys/net.
const (
	disableIPv6File  = "/proc/sys/net/ipv6/conf/" + deviceName + "/disable_ipv6"
	pingGroupRange   = "/proc/sys/net/ipv4/ping_group_range"
	pingGroupAnybody = "0 0"
)

// parsedNetwork is a networkMessage whose addresses were parsed.
type parsedNetwork struct {
	addrs []netip.Prefix
	mtu   int
	dns   []netip.Addr
}

// parseNetwork parses the addresses of a configure message.
func parseNetwork(n *networkMessage) (parsedNetwork, error) {
	p, reason, value := parseNetworkText(n)
	if reason != "" {
		return parsedNetwork{}, errors.New(TagKeeperNetworkInvalid, "the network of a configure message cannot be read", "reason", reason, "value", value)
	}
	return p, nil
}

// parseNetworkText parses the addresses of a configure message, or says what cannot be read.
func parseNetworkText(n *networkMessage) (p parsedNetwork, reason, value string) {
	if n == nil {
		return parsedNetwork{}, "no network", ""
	}
	p.mtu = n.MTU
	for _, text := range n.Addrs {
		prefix, err := netip.ParsePrefix(text)
		if err != nil {
			return parsedNetwork{}, "an address", text
		}
		p.addrs = append(p.addrs, prefix)
	}
	for _, text := range n.DNS {
		addr, err := netip.ParseAddr(text)
		if err != nil {
			return parsedNetwork{}, "a DNS server", text
		}
		p.dns = append(p.dns, addr)
	}
	return p, "", ""
}

// writeSetting writes a setting of the kernel into its existing file, which it never creates.
// A file that does not exist (a kernel without IPv6) is not an error when missingOK.
func writeSetting(path, value string, missingOK bool) error {
	err := writeExisting(path, value)
	if err == nil || (missingOK && os.IsNotExist(err)) {
		return nil
	}
	return errors.NewErr(TagKeeperSysctlFailed, err).SetMetadata("file", filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path))
}

// writeExisting writes value into the file at path, which must exist.
func writeExisting(path, value string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	_, err = f.WriteString(value)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// configureNetwork sets the network of the namespace (docs/SPEC.md §8.3): the loopback up; IPv6
// off on the device when there is no IPv6 address, before the device is up, so that the kernel
// sends nothing of its own into the tunnel; the device's MTU and addresses; the device up; a
// default route for each family that has an address; ping for everybody.
func configureNetwork(nl *netlink.Conn, n parsedNetwork) error {
	lo, err := nl.LinkIndex("lo")
	if err != nil {
		return err
	}
	if err := nl.LinkUp(lo); err != nil {
		return err
	}
	has4 := slices.ContainsFunc(n.addrs, func(p netip.Prefix) bool { return p.Addr().Is4() })
	has6 := slices.ContainsFunc(n.addrs, func(p netip.Prefix) bool { return p.Addr().Is6() })
	disable := "1"
	if has6 {
		disable = "0"
	}
	if err := writeSetting(disableIPv6File, disable, true); err != nil {
		return err
	}
	dev, err := nl.LinkIndex(deviceName)
	if err != nil {
		return err
	}
	if err := nl.SetMTU(dev, n.mtu); err != nil {
		return err
	}
	current, err := nl.Addrs(dev)
	if err != nil {
		return err
	}
	for _, p := range current {
		if !slices.Contains(n.addrs, p) && !p.Addr().IsLinkLocalUnicast() {
			if err := nl.DelAddr(dev, p); err != nil {
				return err
			}
		}
	}
	for _, p := range n.addrs {
		if !slices.Contains(current, p) {
			if err := nl.AddAddr(dev, p); err != nil {
				return err
			}
		}
	}
	if err := nl.LinkUp(dev); err != nil {
		return err
	}
	if has4 {
		if err := nl.AddDefaultRoute(dev, unix.AF_INET); err != nil {
			return err
		}
	}
	if has6 {
		if err := nl.AddDefaultRoute(dev, unix.AF_INET6); err != nil {
			return err
		}
	}
	return writeSetting(pingGroupRange, pingGroupAnybody, false)
}
