// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

// Package pty makes pseudo-terminals: the application of the TUI runs on one, and Tube holds its
// other side (docs/SPEC.md §8.5).
package pty

import (
	"os"
	"strconv"
	"syscall"

	"github.com/rm4n0s/errors"
	"golang.org/x/sys/unix"
)

// The Tags of the pseudo-terminal's errors.
const (
	TagOpenFailed      = "PtyOpenFailed"
	TagUnlockFailed    = "PtyUnlockFailed"
	TagNumberFailed    = "PtyNumberFailed"
	TagSlaveOpenFailed = "PtySlaveOpenFailed"
	TagNonblockFailed  = "PtyNonblockFailed"
	TagSizeFailed      = "PtySizeFailed"
)

// ptmx is the multiplexer of pseudo-terminals; ptsDir holds their slave sides.
const (
	ptmx   = "/dev/ptmx"
	ptsDir = "/dev/pts/"
)

// Open makes a pseudo-terminal and returns its two sides: what is written to one is read from
// the other, through the terminal's line discipline. The master side is non-blocking, so it has
// deadlines; a read of it fails with EIO once every program that had the slave side open is
// gone. Neither side is passed on to a program that is started, unless it is given to it.
func Open() (master, slave *os.File, err error) {
	fd, err := unix.Open(ptmx, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, errors.NewErr(TagOpenFailed, err).SetMetadata("path", ptmx)
	}
	closeFD := func() {
		// The error that is returned is the one that matters.
		_ = unix.Close(fd)
	}
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		closeFD()
		return nil, nil, errors.NewErr(TagUnlockFailed, err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		closeFD()
		return nil, nil, errors.NewErr(TagNumberFailed, err)
	}
	path := ptsDir + strconv.Itoa(n)
	slave, err = os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		closeFD()
		return nil, nil, errors.NewErr(TagSlaveOpenFailed, err).SetMetadata("path", path)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		closeFD()
		// The error of the master side is the one that matters.
		_ = slave.Close()
		return nil, nil, errors.NewErr(TagNonblockFailed, err)
	}
	return os.NewFile(uintptr(fd), ptmx), slave, nil
}

// SetSize tells the terminal its size, in columns and rows, through either side. It keeps the
// side non-blocking (File.Fd would not).
func SetSize(f *os.File, cols, rows int) error {
	ws := &unix.Winsize{Col: uint16(cols), Row: uint16(rows)}
	rc, err := f.SyscallConn()
	if err == nil {
		controlErr := rc.Control(func(fd uintptr) { err = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, ws) })
		if controlErr != nil {
			err = controlErr
		}
	}
	if err != nil {
		return errors.NewErr(TagSizeFailed, err).SetMetadata("cols", cols, "rows", rows)
	}
	return nil
}
