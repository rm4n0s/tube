// SPDX-License-Identifier: GPL-2.0-only

package vpn

import (
	"context"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	openvpn "github.com/rm4n0s/gopenvpn-conn"
	"github.com/rm4n0s/gopenvpn-conn/ovpn"
	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
	"github.com/rm4n0s/tube/internal/core/storage/storagetest"
)

func TestNetworkFromState(t *testing.T) {
	v4 := netip.MustParsePrefix("10.8.0.2/24")
	v6 := netip.MustParsePrefix("fd00:8::1000/64")
	tenDNS := make([]netip.Addr, 10)
	for i := range tenDNS {
		tenDNS[i] = netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)})
	}
	eightDomains := []string{"a.example", "b.example", "c.example", "d.example", "e.example", "f.example", "g.example", "h.example"}
	tests := []struct {
		name      string
		state     openvpn.ConnectionState
		want      Network
		wantRoute string
	}{
		{
			name:  "one IPv4 address",
			state: openvpn.ConnectionState{Addrs: []netip.Prefix{v4}, MTU: 1500},
			want:  Network{Addrs: []netip.Prefix{v4}, MTU: 1500},
		},
		{
			name:  "IPv4 and IPv6, IPv4 first",
			state: openvpn.ConnectionState{Addrs: []netip.Prefix{v6, v4}, MTU: 1500},
			want:  Network{Addrs: []netip.Prefix{v4, v6}, MTU: 1500},
		},
		{
			name:  "an IPv4-mapped IPv6 address becomes IPv4",
			state: openvpn.ConnectionState{Addrs: []netip.Prefix{netip.MustParsePrefix("::ffff:10.8.0.2/120")}, MTU: 1500},
			want:  Network{Addrs: []netip.Prefix{v4}, MTU: 1500},
		},
		{
			name:      "no address",
			state:     openvpn.ConnectionState{MTU: 1500},
			wantRoute: "NetworkFromState.VPNNoAddress",
		},
		{
			name:      "two IPv4 addresses",
			state:     openvpn.ConnectionState{Addrs: []netip.Prefix{v4, netip.MustParsePrefix("10.9.0.2/24")}, MTU: 1500},
			wantRoute: "NetworkFromState.VPNTooManyAddresses",
		},
		{
			name:      "an MTU of 575",
			state:     openvpn.ConnectionState{Addrs: []netip.Prefix{v4}, MTU: 575},
			wantRoute: "NetworkFromState.VPNBadMTU",
		},
		{
			name:  "an MTU of 576",
			state: openvpn.ConnectionState{Addrs: []netip.Prefix{v4}, MTU: 576},
			want:  Network{Addrs: []netip.Prefix{v4}, MTU: 576},
		},
		{
			name:  "an MTU of 65535",
			state: openvpn.ConnectionState{Addrs: []netip.Prefix{v4}, MTU: 65535},
			want:  Network{Addrs: []netip.Prefix{v4}, MTU: 65535},
		},
		{
			name:      "an MTU of 65536",
			state:     openvpn.ConnectionState{Addrs: []netip.Prefix{v4}, MTU: 65536},
			wantRoute: "NetworkFromState.VPNBadMTU",
		},
		{
			name: "DNS servers: the invalid and the unspecified are dropped, eight are kept",
			state: openvpn.ConnectionState{Addrs: []netip.Prefix{v4}, MTU: 1500,
				DNSServers: append([]netip.Addr{{}, netip.IPv4Unspecified()}, tenDNS...)},
			want: Network{Addrs: []netip.Prefix{v4}, MTU: 1500, DNSServers: tenDNS[:8]},
		},
		{
			name: "search domains: only host names, six are kept",
			state: openvpn.ConnectionState{Addrs: []netip.Prefix{v4}, MTU: 1500,
				SearchDomains: append([]string{"bad domain", "a\nnameserver 6.6.6.6"}, eightDomains...)},
			want: Network{Addrs: []netip.Prefix{v4}, MTU: 1500, SearchDomains: eightDomains[:6]},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NetworkFromState(tt.state)
			if tt.wantRoute != "" {
				errtest.Route(t, err, tt.wantRoute)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.Addrs, tt.want.Addrs) || got.MTU != tt.want.MTU ||
				!slices.Equal(got.DNSServers, tt.want.DNSServers) || !slices.Equal(got.SearchDomains, tt.want.SearchDomains) {
				t.Errorf("NetworkFromState = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// silentServer listens on a UDP port of the loopback and never answers, so that a client waits
// for its answer. It returns the port.
func silentServer(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	return conn.LocalAddr().(*net.UDPAddr).Port
}

// addProfile adds a profile of the given options to a new isolated store.
func addProfile(t *testing.T, opts profilestest.Options) profiles.Profile {
	t.Helper()
	dirs := storagetest.Isolate(t)
	store := profiles.NewStore(dirs.Config, clock.NewFake(time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)))
	path := profilestest.WriteProfile(t, t.TempDir(), "test.ovpn", profilestest.NewPKI(t), opts)
	p, err := store.Add(context.Background(), path, "test", profiles.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDial(t *testing.T) {
	t.Run("a broken profile", func(t *testing.T) {
		_, err := Dial(context.Background(), profiles.Profile{ID: "x", Broken: "it is broken"}, nil)
		errtest.Route(t, err, "Dial.VPNProfileBroken")
	})
	t.Run("a profile that needs a password, and none", func(t *testing.T) {
		p := addProfile(t, profilestest.Options{Password: true})
		_, err := Dial(context.Background(), p, nil)
		errtest.Route(t, err, "Dial.VPNCredentialsNeeded")
	})
	t.Run("a server that does not answer", func(t *testing.T) {
		port := silentServer(t)
		p := addProfile(t, profilestest.Options{Remote: "127.0.0.1 " + strconv.Itoa(port), Extra: []string{"connect-timeout 1"}})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := time.Now()
		_, err := Dial(ctx, p, nil)
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("Dial took %v", elapsed)
		}
		if _, ok := errors.FromError(err); !ok {
			t.Fatalf("err = %T %v, want an *errors.Error", err, err)
		}
		if got := KindOf(err); got != KindNoAnswer {
			t.Errorf("KindOf(%v) = %d, want KindNoAnswer", err, got)
		}
	})
	t.Run("a dial that is cancelled", func(t *testing.T) {
		port := silentServer(t)
		p := addProfile(t, profilestest.Options{Remote: "127.0.0.1 " + strconv.Itoa(port), Extra: []string{"connect-timeout 30"}})
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)
		start := time.Now()
		_, err := Dial(ctx, p, nil)
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("Dial took %v after the cancel", elapsed)
		}
		if got := KindOf(err); got != KindCancelled {
			t.Errorf("KindOf(%v) = %d, want KindCancelled", err, got)
		}
	})
}

func TestKindOf(t *testing.T) {
	tests := []struct {
		err  error
		want Kind
	}{
		{&errors.Error{Tag: openvpn.TagAuthFailed}, KindAuthFailed},
		{&errors.Error{Tag: openvpn.TagServerPollTimeout}, KindNoAnswer},
		{&errors.Error{Tag: openvpn.TagDialFailed}, KindNoAnswer},
		{&errors.Error{Tag: openvpn.TagHandshakeTimeout}, KindNoAnswer},
		{&errors.Error{Tag: openvpn.TagLinkFailed}, KindNoAnswer},
		{&errors.Error{Tag: openvpn.TagPingTimeout}, KindNoAnswer},
		{&errors.Error{Tag: openvpn.TagTLSHandshakeFailed}, KindCertificate},
		{&errors.Error{Tag: openvpn.TagServerFingerprintMismatch}, KindCertificate},
		{&errors.Error{Tag: openvpn.TagServerCertificateUsage}, KindCertificate},
		{&errors.Error{Tag: openvpn.TagServerNameMismatch}, KindCertificate},
		{&errors.Error{Tag: openvpn.TagServerHalt}, KindServerEnded},
		{&errors.Error{Tag: openvpn.TagServerRestart}, KindServerEnded},
		{&errors.Error{Tag: openvpn.TagServerExit}, KindServerEnded},
		{&errors.Error{Tag: openvpn.TagUnsupportedPushedOption}, KindUnsupported},
		{&errors.Error{Tag: openvpn.TagCipherNegotiationFailed}, KindUnsupported},
		{&errors.Error{Tag: openvpn.TagKeyDerivationUnsupported}, KindUnsupported},
		{&errors.Error{Tag: openvpn.TagPacketTooLarge}, KindPacketTooLarge},
		{&errors.Error{Tag: ovpn.TagCredentialsMissing}, KindOther},
		{io.EOF, KindServerEnded},
		{context.Canceled, KindCancelled},
		{errors.NewErr(openvpn.TagServerPollTimeout, context.DeadlineExceeded), KindCancelled},
		{errors.New("Something", "else"), KindOther},
		{net.ErrClosed, KindOther},
		{nil, KindOther},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
			if appErr, ok := errors.FromError(tt.err); ok && appErr.Tag != "" {
				name = appErr.Tag
			}
		}
		t.Run(strings.ReplaceAll(name, " ", "_"), func(t *testing.T) {
			if got := KindOf(tt.err); got != tt.want {
				t.Errorf("KindOf(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
