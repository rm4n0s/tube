// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package pump

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/core/nstest"
	"github.com/rm4n0s/tube/internal/core/packettest"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sandbox/sandboxtest"
	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

func TestMain(m *testing.M) {
	if sandbox.IsKeeper() {
		os.Exit(sandbox.KeeperMain())
	}
	sandboxtest.HandleProbe()
	nstest.Main(m, nil)
}

func TestPingThroughThePump(t *testing.T) {
	nstest.Require(t)
	ping, err := exec.LookPath("ping")
	if err != nil {
		t.Skip("ping is not installed")
	}
	s, err := sandbox.Start(context.Background(), sandbox.Options{
		Path: ping, Argv: []string{"ping", "-c", "2", "-W", "2", "10.9.0.1"},
		TempDir: t.TempDir(), KillDelay: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() }) // the result is checked below
	if _, err := s.Configure(context.Background(), network, nil); err != nil {
		t.Fatal(err)
	}
	tun := vpntest.NewTunnel(network)
	tun.OnWrite(func(packet []byte) [][]byte {
		if packettest.IsEchoRequest4(packet) {
			return [][]byte{packettest.EchoReply4(packet)}
		}
		return nil
	})
	p := Start(s.Device(), tun, network.MTU)
	if err := s.Run(context.Background()); err != nil {
		p.Stop()
		t.Fatal(err)
	}
	st, err := s.Wait()
	c := p.Stop()
	if err != nil || st.Status() != 0 {
		t.Fatalf("ping ended with %+v, %v", st, err)
	}
	if c.ToTunnel.Packets < 2 || c.FromTunnel.Packets < 2 || p.Err() != nil {
		t.Errorf("Counters = %+v, Err = %v", c, p.Err())
	}
	if err := s.Close(); err != nil {
		t.Error(err)
	}
}
