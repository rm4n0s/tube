// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"os"

	"github.com/rm4n0s/errors"
	"golang.org/x/sys/unix"
)

// The Tags of the errors of the TUN device.
const (
	TagTunOpenFailed  = "SandboxTunOpenFailed"
	TagTunSetupFailed = "SandboxTunSetupFailed"
)

// tunPath is the device of TUN interfaces; the variable TUBE_TUN_DEVICE replaces it, for tests.
const tunPath = "/dev/net/tun"

// tunDeviceVar is the variable that replaces tunPath.
const tunDeviceVar = "TUBE_TUN_DEVICE"

// deviceName is the name of the TUN device in the namespace.
const deviceName = "tube0"

// tunDevicePath is the path of the device of TUN interfaces.
func tunDevicePath() string {
	if path := os.Getenv(tunDeviceVar); path != "" {
		return path
	}
	return tunPath
}

// openTun makes a TUN device called name, without packet information (each read and write is
// one IP packet), and returns its descriptor, close-on-exec. The device exists while the
// descriptor, or a copy of it, is open.
func openTun(path, name string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, errors.NewErr(TagTunOpenFailed, err).SetMetadata("path", path)
	}
	if err := setTun(fd, name); err != nil {
		// The setup's error is the one that matters.
		_ = unix.Close(fd)
		return -1, errors.NewErr(TagTunSetupFailed, err).SetMetadata("path", path, "name", name)
	}
	return fd, nil
}

// setTun makes fd the TUN device called name. It returns the system's error as it is.
func setTun(fd int, name string) error {
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		return err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	return unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr)
}
