// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package pty

import (
	"bufio"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"golang.org/x/sys/unix"
)

// open opens a pseudo-terminal whose sides are closed when the test ends.
func open(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, slave, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// A test may have closed a side already.
		_ = master.Close()
		_ = slave.Close()
	})
	return master, slave
}

// readUntil reads the master side until want has come, for at most two seconds.
func readUntil(t *testing.T, master *os.File, want string) string {
	t.Helper()
	if err := master.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	buf := make([]byte, 256)
	for !strings.Contains(got.String(), want) {
		n, err := master.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			t.Fatalf("read %q, then: %v", got.String(), err)
		}
	}
	return got.String()
}

func TestOpen(t *testing.T) {
	master, slave := open(t)
	if _, err := slave.WriteString("from the program\n"); err != nil {
		t.Fatal(err)
	}
	// The line discipline turns "\n" into "\r\n".
	readUntil(t, master, "from the program\r\n")
	if _, err := master.WriteString("typed by the user\n"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(slave).ReadString('\n')
	if err != nil || line != "typed by the user\n" {
		t.Errorf("the slave side read %q, %v", line, err)
	}
	for name, f := range map[string]*os.File{"master": master, "slave": slave} {
		flags, err := unix.FcntlInt(f.Fd(), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Errorf("the %s side is not close-on-exec: %d, %v", name, flags, err)
		}
	}
}

func TestSetSize(t *testing.T) {
	master, slave := open(t)
	if err := SetSize(master, 120, 40); err != nil {
		t.Fatal(err)
	}
	ws, err := unix.IoctlGetWinsize(int(slave.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col != 120 || ws.Row != 40 {
		t.Errorf("the size is %+v, %v", ws, err)
	}
	// The master side still has deadlines: a read with nothing to read ends at the deadline.
	if err := master.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("the master side lost its deadlines: %v", err)
	}
	if _, err := master.Read(make([]byte, 8)); !os.IsTimeout(err) {
		t.Errorf("a read of the master side: %v, want a timeout", err)
	}
}

func TestSetSizeFails(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // only read
	errtest.Route(t, SetSize(f, 80, 24), "SetSize.PtySizeFailed")
}

func TestTheEnd(t *testing.T) {
	master, slave := open(t)
	if err := slave.Close(); err != nil {
		t.Fatal(err)
	}
	if err := master.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err := master.Read(make([]byte, 8))
	if !errors.Is(err, syscall.EIO) {
		t.Errorf("a read of the master side after the end: %v, want EIO", err)
	}
}
