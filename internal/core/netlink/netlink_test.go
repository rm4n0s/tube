// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package netlink

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/nstest"
	"golang.org/x/sys/unix"
)

func TestMain(m *testing.M) {
	nstest.Main(m, map[string]func() error{
		"loopback": nsLoopback,
		"tun":      nsTun,
		"deladdr":  nsDelAddr,
		"refused":  nsRefused,
		"notfound": nsNotFound,
		"invalid":  nsInvalid,
	})
}

// routeOf is the route of an error, or a sentence that says that it is not an *errors.Error.
// The functions that run in a namespace print it, and the test outside compares it.
func routeOf(err error) string {
	appErr, ok := errors.FromError(err)
	if !ok {
		return fmt.Sprintf("not an *errors.Error: %T %v", err, err)
	}
	return appErr.Route()
}

// closing calls close when a function of the namespace returns, and makes its failure the
// function's when it had none.
func closing(err *error, close func() error) {
	if cerr := close(); cerr != nil && *err == nil {
		*err = cerr
	}
}

// makeTun makes a TUN device called tube0 in the namespace and returns its descriptor; the
// device exists while it is open.
func makeTun() (int, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	ifr, err := unix.NewIfreq("tube0")
	if err != nil {
		return -1, err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		return -1, err
	}
	return fd, nil
}

func nsLoopback() (err error) {
	c, err := Open()
	if err != nil {
		return err
	}
	defer closing(&err, c.Close)
	index, err := c.LinkIndex("lo")
	if err != nil {
		return err
	}
	if err := c.LinkUp(index); err != nil {
		return err
	}
	links, err := c.Links()
	if err != nil {
		return err
	}
	for _, l := range links {
		if l.Name == "lo" && l.Up {
			return nil
		}
	}
	return fmt.Errorf("lo is not up: %+v", links)
}

func nsTun() (err error) {
	fd, err := makeTun()
	if err != nil {
		return err
	}
	defer closing(&err, func() error { return unix.Close(fd) })
	c, err := Open()
	if err != nil {
		return err
	}
	defer closing(&err, c.Close)
	index, err := c.LinkIndex("tube0")
	if err != nil {
		return err
	}
	v4, v6 := netip.MustParsePrefix("10.9.0.2/24"), netip.MustParsePrefix("fd00:9::2/64")
	steps := []func() error{
		func() error { return c.SetMTU(index, 1400) },
		func() error { return c.AddAddr(index, v4) },
		func() error { return c.AddAddr(index, v6) },
		func() error { return c.LinkUp(index) },
		func() error { return c.AddDefaultRoute(index, unix.AF_INET) },
		func() error { return c.AddDefaultRoute(index, unix.AF_INET6) },
		// A second default route of a family is not an error.
		func() error { return c.AddDefaultRoute(index, unix.AF_INET) },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			return fmt.Errorf("step %d: %v (%s)", i+1, err, routeOf(err))
		}
	}
	links, err := c.Links()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(links, func(l Link) bool { return l.Name == "tube0" })
	if i < 0 || links[i].MTU != 1400 || !links[i].Up {
		return fmt.Errorf("tube0 is not up with MTU 1400: %+v", links)
	}
	addrs, err := c.Addrs(index)
	if err != nil {
		return err
	}
	if !slices.Contains(addrs, v4) || !slices.Contains(addrs, v6) {
		return fmt.Errorf("the addresses are %v", addrs)
	}
	routes, err := c.Routes()
	if err != nil {
		return err
	}
	for _, family := range []int{unix.AF_INET, unix.AF_INET6} {
		found := false
		for _, r := range routes {
			if r.Family == family && r.Dst.Bits() == 0 && r.OutIndex == index {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("no default route of the family %d out of tube0: %+v", family, routes)
		}
	}
	return nil
}

func nsDelAddr() (err error) {
	fd, err := makeTun()
	if err != nil {
		return err
	}
	defer closing(&err, func() error { return unix.Close(fd) })
	c, err := Open()
	if err != nil {
		return err
	}
	defer closing(&err, c.Close)
	index, err := c.LinkIndex("tube0")
	if err != nil {
		return err
	}
	prefix := netip.MustParsePrefix("10.9.0.2/24")
	if err := c.AddAddr(index, prefix); err != nil {
		return err
	}
	if err := c.DelAddr(index, prefix); err != nil {
		return err
	}
	addrs, err := c.Addrs(index)
	if err != nil {
		return err
	}
	if slices.Contains(addrs, prefix) {
		return fmt.Errorf("the address is still there: %v", addrs)
	}
	return nil
}

func nsRefused() (err error) {
	c, err := Open()
	if err != nil {
		return err
	}
	defer closing(&err, c.Close)
	err = c.LinkUp(9999)
	fmt.Println(routeOf(err))
	if !errors.Is(err, unix.ENODEV) {
		return fmt.Errorf("the cause is not ENODEV: %v", err)
	}
	return nil
}

func nsNotFound() (err error) {
	c, err := Open()
	if err != nil {
		return err
	}
	defer closing(&err, c.Close)
	_, err = c.LinkIndex("nosuch0")
	fmt.Println(routeOf(err))
	return nil
}

func nsInvalid() (err error) {
	c, err := Open()
	if err != nil {
		return err
	}
	defer closing(&err, c.Close)
	fmt.Println(routeOf(c.AddAddr(1, netip.Prefix{})))
	fmt.Println(routeOf(c.DelAddr(1, netip.PrefixFrom(netip.MustParseAddr("10.9.0.2"), 33))))
	fmt.Println(routeOf(c.AddDefaultRoute(1, 99)))
	return nil
}

func TestLoopback(t *testing.T) { nstest.Run(t, "loopback") }

func TestTun(t *testing.T) { nstest.Run(t, "tun") }

func TestDelAddr(t *testing.T) { nstest.Run(t, "deladdr") }

func TestRefused(t *testing.T) {
	out := nstest.Run(t, "refused")
	if !strings.Contains(out, "Conn.LinkUp->Conn.do->Conn.roundTrip.NetlinkRefused") {
		t.Errorf("the route is %q", out)
	}
}

func TestLinkNotFound(t *testing.T) {
	out := nstest.Run(t, "notfound")
	if !strings.Contains(out, "Conn.LinkIndex.NetlinkLinkNotFound") {
		t.Errorf("the route is %q", out)
	}
}

func TestInvalidArguments(t *testing.T) {
	out := nstest.Run(t, "invalid")
	for _, want := range []string{
		"Conn.AddAddr->addrMessage.NetlinkAddressInvalid",
		"Conn.DelAddr->addrMessage.NetlinkAddressInvalid",
		"Conn.AddDefaultRoute.NetlinkFamilyInvalid",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the routes are %q, want %s in them", out, want)
		}
	}
}

func TestOpenAndClose(t *testing.T) {
	c, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Links(); err != nil {
		t.Errorf("listing the links of the machine: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	t.Run("a closed socket", func(t *testing.T) {
		errtest.Route(t, c.LinkUp(1), "Conn.LinkUp->Conn.do->Conn.roundTrip.NetlinkSendFailed")
		errtest.Route(t, c.Close(), "Conn.Close.NetlinkCloseFailed")
	})
}

func TestAnswersThatCannotBeRead(t *testing.T) {
	c, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()
	t.Run("the read fails", func(t *testing.T) {
		c.recv = func(int, []byte) (int, error) { return 0, syscall.EINTR }
		_, err := c.Links()
		errtest.Route(t, err, "Conn.Links->Conn.dump->Conn.roundTrip.NetlinkReceiveFailed")
	})
	t.Run("the answer is cut short", func(t *testing.T) {
		c.recv = func(_ int, buf []byte) (int, error) { return copy(buf, []byte{1, 2, 3}), nil }
		_, err := c.Links()
		errtest.Route(t, err, "Conn.Links->Conn.dump->Conn.roundTrip.NetlinkParseFailed")
	})
	t.Run("a header that claims more than there is", func(t *testing.T) {
		c.recv = func(_ int, buf []byte) (int, error) {
			b := request(unix.NLMSG_DONE, 0, 1, []byte{1, 2, 3, 4})
			b[0] = 200 // the length says 200 bytes; there are 20
			return copy(buf, b), nil
		}
		_, err := c.Links()
		errtest.Route(t, err, "Conn.Links->Conn.dump->Conn.roundTrip.NetlinkParseFailed")
	})
}
