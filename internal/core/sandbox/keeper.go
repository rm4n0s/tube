// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/netlink"
	"golang.org/x/sys/unix"
)

// KeeperArg0 is the argv[0] under which Tube starts itself as the keeper.
const KeeperArg0 = "tube-keeper"

// keeperFD is the descriptor of the keeper's end of the socket: the first of ExtraFiles.
const keeperFD = 3

// maxSignal is the largest signal number that Tube may ask the keeper to send.
const maxSignal = 64

// The Tags of the keeper's errors, which reach Tube in a message of type failed.
const (
	TagKeeperMountFailed      = "KeeperMountPrivateFailed"
	TagKeeperUnknownMessage   = "KeeperUnknownMessage"
	TagKeeperAppNotFound      = "KeeperAppNotFound"
	TagKeeperAppNotExecutable = "KeeperAppNotExecutable"
	TagKeeperAppStartFailed   = "KeeperAppStartFailed"
	TagKeeperAlreadyRunning   = "KeeperAlreadyRunning"
	TagKeeperNoApplication    = "KeeperNoApplication"
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
	// The connection has its own copy of the descriptor, which is close-on-exec: the
	// application never gets the socket.
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
	// sendMu keeps the answers of the loop and the end of the application apart.
	sendMu sync.Mutex
	// nl is opened by the first configure and kept; dns are the files of the names.
	nl  *netlink.Conn
	dns *dnsFiles
	// app is the application once it started; appEnded gets its state when it ends (nil when
	// it could not be waited for).
	app      *exec.Cmd
	appEnded chan *os.ProcessState
	// killDelay is the delay of the run message, kept for the end of the application.
	killDelay time.Duration
}

// incoming is what the keeper's reader got: a message or the error that ended the reading.
type incoming struct {
	m   message
	err error
}

// send sends a message to Tube.
func (k *keeper) send(m message) error {
	k.sendMu.Lock()
	defer k.sendMu.Unlock()
	return send(k.conn, m, -1)
}

// fail tells Tube about an error and returns the exit status of a keeper that failed.
func (k *keeper) fail(err error) int {
	// When even this cannot be sent, Tube learns it from the socket closing.
	_ = k.send(failedMessage(err, 0))
	return 1
}

// run makes the namespace ready, hands the device to Tube and serves Tube's messages until
// the socket is closed or the application has ended.
func (k *keeper) run() int {
	// The signals of the terminal reach the application directly: the keeper only must not die
	// of them. SIGTERM and SIGHUP are passed on to the application. They are caught, never
	// ignored: an ignored signal would stay ignored in the application (trap 13).
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
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
	defer k.closeNetlink()
	in := make(chan incoming)
	go k.read(in)
	for {
		select {
		case got := <-in:
			if got.err != nil {
				k.endApp()
				if appErr, ok := errors.FromError(got.err); ok && appErr.Tag == TagSocketClosed {
					return 0
				}
				return 1
			}
			if err := k.handle(got.m); err != nil {
				k.endApp()
				return 1
			}
		case state := <-k.appEnded:
			if state == nil {
				return 1 // Tube learns it from the socket closing: the keeper is lost
			}
			return k.exited(state)
		case sig := <-sigs:
			if sig == syscall.SIGTERM || sig == syscall.SIGHUP {
				k.signalApp(sig.(syscall.Signal))
			}
		}
	}
}

// read hands every message of Tube to the loop, and then the error that ended the reading. It
// ends with the keeper's process.
func (k *keeper) read(in chan<- incoming) {
	for {
		m, fd, err := receive(k.conn)
		if fd >= 0 {
			_ = unix.Close(fd) // Tube never sends a descriptor
		}
		in <- incoming{m, err}
		if err != nil {
			return
		}
	}
}

// closeNetlink closes the netlink socket when there is one; the keeper is ending, so a failure
// changes nothing.
func (k *keeper) closeNetlink() {
	if k.nl != nil {
		_ = k.nl.Close()
	}
}

// signalApp sends the signal to the application, when one runs.
func (k *keeper) signalApp(sig syscall.Signal) {
	if k.app != nil {
		// An application that has just ended cannot get it, and needs not.
		_ = k.app.Process.Signal(sig)
	}
}

// handle answers one message of Tube; a signal needs no answer. An error is returned only when
// the answer cannot be sent.
func (k *keeper) handle(m message) error {
	if m.Type == msgSignal {
		if m.Signal > 0 && m.Signal <= maxSignal {
			k.signalApp(syscall.Signal(m.Signal))
		}
		return nil
	}
	var err error
	answer := message{Seq: m.Seq}
	switch m.Type {
	case msgConfigure:
		answer.Type = msgConfigured
		answer.Warnings, err = k.configure(m)
	case msgRun:
		answer.Type = msgStarted
		answer.PID, err = k.startApp(m)
	default:
		err = errors.New(TagKeeperUnknownMessage, "the keeper does not know this message", "type", m.Type)
	}
	if err != nil {
		answer = failedMessage(err, m.Seq)
	}
	return k.send(answer)
}

// configure sets the network and the names of the namespace to the ones of the message, and
// returns the warnings about the names.
func (k *keeper) configure(m message) ([]string, error) {
	n, err := parseNetwork(m.Network)
	if err != nil {
		return nil, err
	}
	if k.nl == nil {
		nl, err := netlink.Open()
		if err != nil {
			return nil, err
		}
		k.nl = nl
		k.dns = newDNSFiles()
	}
	if err := configureNetwork(k.nl, n); err != nil {
		return nil, err
	}
	return k.dns.apply(n.dns, m.Network.Search)
}

// startApp starts the application that the keeper was started with, in a nested user
// namespace where it is the user again and has no rights over the network (trap 18), and
// returns its pid.
func (k *keeper) startApp(m message) (int, error) {
	if k.app != nil {
		return 0, errors.New(TagKeeperAlreadyRunning, "the application has been started already", "pid", k.app.Process.Pid)
	}
	if len(os.Args) < 2 {
		return 0, errors.New(TagKeeperNoApplication, "the keeper was started without an application")
	}
	cmd := exec.Command(os.Args[1])
	cmd.Args = os.Args[2:]
	cmd.Env = withoutKeeperVars(os.Environ())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: m.UID, HostID: 0, Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: m.GID, HostID: 0, Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	if m.Terminal {
		// The leader of a new session, whose controlling terminal is its standard input.
		cmd.SysProcAttr.Setsid, cmd.SysProcAttr.Setctty, cmd.SysProcAttr.Ctty = true, true, 0
	}
	if err := cmd.Start(); err != nil {
		switch {
		case errors.Is(err, syscall.ENOENT):
			return 0, errors.NewErr(TagKeeperAppNotFound, err)
		case errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOEXEC) || errors.Is(err, syscall.EISDIR):
			return 0, errors.NewErr(TagKeeperAppNotExecutable, err)
		}
		return 0, errors.NewErr(TagKeeperAppStartFailed, err)
	}
	k.app = cmd
	k.killDelay = time.Duration(m.KillMS) * time.Millisecond
	if k.killDelay <= 0 {
		k.killDelay = defaultKillDelay
	}
	k.appEnded = make(chan *os.ProcessState, 1)
	go func() {
		// The state says how it ended; an error with no state means it cannot be known.
		_ = cmd.Wait()
		k.appEnded <- cmd.ProcessState
	}()
	return cmd.Process.Pid, nil
}

// withoutKeeperVars is the environment without the variables that only the keeper reads.
func withoutKeeperVars(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(v string) bool { return strings.HasPrefix(v, keeperVarPrefix) })
}

// exited tells Tube how the application ended, and returns the keeper's exit status.
func (k *keeper) exited(state *os.ProcessState) int {
	m := message{Type: msgExited, Code: state.ExitCode()}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		m.Code, m.Signal = 0, int(ws.Signal())
	}
	if err := k.send(m); err != nil {
		return 1
	}
	return 0
}

// endApp ends the application, when one runs, and waits for it: Tube is gone, and nothing of
// the namespace may stay behind. SIGTERM first, and SIGKILL after the kill delay.
func (k *keeper) endApp() {
	if k.app == nil {
		return
	}
	// A signal to an application that just ended fails; the waits below are what matter.
	_ = k.app.Process.Signal(syscall.SIGTERM)
	select {
	case <-k.appEnded:
		return
	case <-time.After(k.killDelay):
	}
	_ = k.app.Process.Kill()
	<-k.appEnded
}
