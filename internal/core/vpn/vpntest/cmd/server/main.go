// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

// Command server starts the test server of vpntest outside a test, for the proofs: it makes the
// keys and the configuration in the folder of --dir, starts openvpn on the loopback, writes the
// client profile to <dir>/test.ovpn, prints "ready <dir>/test.ovpn <the server's tunnel
// address>", and stops the server on SIGTERM. Run it only under scripts/vpn-ns.sh.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/rm4n0s/tube/internal/core/vpn/vpntest"
)

// tb is the testing.TB that StartServer needs, outside a test: a failure or a skip ends the
// program, the temporary folders are in the folder of --dir, and the cleanups run on stop. The
// methods that StartServer does not call are those of the nil interface, and are never called.
type tb struct {
	testing.TB
	dir      string
	temps    int
	cleanups []func()
}

func (*tb) Helper() {}

func (b *tb) Fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "server: "+format+"\n", args...)
	b.stop()
	os.Exit(1)
}

func (b *tb) Skip(args ...any) {
	fmt.Fprintln(os.Stderr, append([]any{"server: skipped:"}, args...)...)
	b.stop()
	os.Exit(1)
}

func (b *tb) Logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "server: "+format+"\n", args...)
}

func (b *tb) TempDir() string {
	b.temps++
	dir := filepath.Join(b.dir, "server-"+strconv.Itoa(b.temps))
	if err := os.Mkdir(dir, 0o700); err != nil {
		b.Fatalf("%v", err)
	}
	return dir
}

func (b *tb) Cleanup(f func()) { b.cleanups = append(b.cleanups, f) }

// stop runs the cleanups, the last one first.
func (b *tb) stop() {
	for i := len(b.cleanups) - 1; i >= 0; i-- {
		b.cleanups[i]()
	}
	b.cleanups = nil
}

func main() {
	dir := flag.String("dir", "", "the folder for the keys, the configuration and test.ovpn")
	flag.Parse()
	if *dir == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: server --dir <folder>")
		os.Exit(2)
	}
	b := &tb{dir: *dir}
	// SIGTERM is asked for before the server starts, so that it is never lost.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	server := vpntest.StartServer(b, vpntest.ServerOptions{})
	// The profile and the files next to it, into the folder of --dir.
	src := filepath.Dir(server.ProfilePath())
	entries, err := os.ReadDir(src)
	if err != nil {
		b.Fatalf("%v", err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			b.Fatalf("%v", err)
		}
		if err := os.WriteFile(filepath.Join(*dir, e.Name()), data, 0o600); err != nil {
			b.Fatalf("%v", err)
		}
	}
	fmt.Println("ready", filepath.Join(*dir, "test.ovpn"), server.TunnelAddr())
	<-stop
	b.stop()
}
