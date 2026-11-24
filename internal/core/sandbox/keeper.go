// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/rm4n0s/errors"
	"golang.org/x/sys/unix"
)

// KeeperArg0 is the argv[0] under which Tube starts itself as the keeper.
const KeeperArg0 = "tube-keeper"

// keeperFD is the descriptor of the keeper's end of the socket: the first of ExtraFiles.
const keeperFD = 3

// The Tags of the keeper's errors, which reach Tube in a message of type failed.
const (
	TagKeeperMountFailed    = "KeeperMountPrivateFailed"
	TagKeeperUnknownMessage = "KeeperUnknownMessage"
)

// IsKeeper says whether this process was started as the keeper.
func IsKeeper() bool {
	return filepath.Base(os.Args[0]) == KeeperArg0
}

// toText is a value of an error's metadata as text, for a message.
func toText(v any) string { return fmt.Sprint(v) }

// KeeperMain is the keeper (docs/SPEC.md §8.1). It runs inside the new namespaces, as their
// root, and talks to Tube over the socket of descriptor 3. It writes nothing to its standard
// output or error, which are the user's terminal. It returns the exit status of the process.
func KeeperMain() int {
	f := os.NewFile(keeperFD, "tube-keeper-socket")
	c, err := net.FileConn(f)
	// The connection has its own copy of the descriptor.
	_ = f.Close()
	if err != nil {
		return 2 // there is no socket to tell Tube on
	}
	conn, ok := c.(*net.UnixConn)
	if !ok {
		return 2
	}
	k := &keeper{conn: conn}
	return k.run()
}

// keeper is the state of the keeper process.
type keeper struct {
	conn *net.UnixConn
}

// fail tells Tube about an error and returns the exit status of a keeper that failed.
func (k *keeper) fail(err error) int {
	// When even this cannot be sent, Tube learns it from the socket closing.
	_ = send(k.conn, failedMessage(err), -1)
	return 1
}

// run makes the namespace ready, hands the device to Tube and serves Tube's messages until
// the socket is closed.
func (k *keeper) run() int {
	// Nothing that the keeper mounts may be seen outside its mount namespace.
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return k.fail(errors.NewErr(TagKeeperMountFailed, err))
	}
	fd, err := openTun(tunDevicePath(), deviceName)
	if err != nil {
		return k.fail(err)
	}
	err = send(k.conn, message{Type: msgReady}, fd)
	// Tube holds the only reference from now on: when Tube ends, the device goes.
	_ = unix.Close(fd)
	if err != nil {
		return 1
	}
	for {
		m, received, err := receive(k.conn, time.Time{})
		if received >= 0 {
			_ = unix.Close(received) // Tube never sends a descriptor
		}
		if err != nil {
			if appErr, ok := errors.FromError(err); ok && appErr.Tag == TagSocketClosed {
				return 0
			}
			return 1
		}
		// TODO(step 2.3): configure and run.
		if err := send(k.conn, failedMessage(errors.New(TagKeeperUnknownMessage, "the keeper does not know this message", "type", m.Type)), -1); err != nil {
			return 1
		}
	}
}
