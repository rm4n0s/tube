// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"encoding/json"
	"io"
	"net"
	"syscall"

	"github.com/rm4n0s/errors"
	"golang.org/x/sys/unix"
)

// maxMessage is the largest message, in bytes.
const maxMessage = 32 << 10

// The types of the messages (docs/SPEC.md §8.1).
const (
	msgReady      = "ready"
	msgConfigure  = "configure"
	msgConfigured = "configured"
	msgRun        = "run"
	msgStarted    = "started"
	msgSignal     = "signal"
	msgExited     = "exited"
	msgFailed     = "failed"
)

// The Tags of the errors of the messages.
const (
	TagSocketClosed      = "SandboxSocketClosed"
	TagMessageInvalid    = "SandboxMessageInvalid"
	TagMessageTooLarge   = "SandboxMessageTooLarge"
	TagSendFailed        = "SandboxSendFailed"
	TagReceiveFailed     = "SandboxReceiveFailed"
	TagMessageEncodeFail = "SandboxMessageEncodeFailed"
)

// message is one message between Tube and the keeper, sent as JSON in one packet of the
// socket. A message of type failed carries the Tag, the route, the message and the metadata
// (as text) of the error that the keeper met. The keeper answers a request with the request's Seq, so that
// Tube never takes a late answer for the answer to a later request.
type message struct {
	Type     string            `json:"type"`
	Seq      int               `json:"seq,omitempty"`
	Network  *networkMessage   `json:"network,omitempty"`  // configure
	UID      int               `json:"uid,omitempty"`      // run: the user's own uid
	GID      int               `json:"gid,omitempty"`      // run: the user's own gid
	KillMS   int               `json:"kill_ms,omitempty"`  // run: the delay before SIGKILL
	PID      int               `json:"pid,omitempty"`      // started
	Signal   int               `json:"signal,omitempty"`   // signal; exited by a signal
	Code     int               `json:"code,omitempty"`     // exited
	Warnings []string          `json:"warnings,omitempty"` // configured
	Tag      string            `json:"tag,omitempty"`      // failed
	Route    string            `json:"route,omitempty"`    // failed
	Text     string            `json:"text,omitempty"`     // failed
	Meta     map[string]string `json:"meta,omitempty"`     // failed
}

// networkMessage is the network of a configure message: addresses and servers as text, which
// the keeper parses again.
type networkMessage struct {
	Addrs  []string `json:"addrs"`
	MTU    int      `json:"mtu"`
	DNS    []string `json:"dns,omitempty"`
	Search []string `json:"search,omitempty"`
}

// send sends a message, and with it the file descriptor fd when it is not negative.
func send(conn *net.UnixConn, m message, fd int) error {
	data, err := json.Marshal(m)
	if err != nil {
		return errors.NewErr(TagMessageEncodeFail, err).SetMetadata("type", m.Type)
	}
	if len(data) > maxMessage {
		return errors.New(TagMessageTooLarge, "the message is too large", "type", m.Type, "size", len(data), "max", maxMessage)
	}
	var oob []byte
	if fd >= 0 {
		oob = unix.UnixRights(fd)
	}
	if _, _, err := conn.WriteMsgUnix(data, oob, nil); err != nil {
		return errors.NewErr(TagSendFailed, err).SetMetadata("type", m.Type)
	}
	return nil
}

// receive waits for the next message. It returns the file descriptor that came with it, or -1;
// a descriptor that came is close-on-exec and non-blocking. Closing conn ends the wait.
func receive(conn *net.UnixConn) (message, int, error) {
	buf := make([]byte, maxMessage+1)
	oob := make([]byte, unix.CmsgSpace(4*4))
	n, oobn, flags, _, err := conn.ReadMsgUnix(buf, oob)
	// A socket pair of SOCK_SEQPACKET reads 0 bytes when the other end is closed.
	if (err == nil && n == 0 && oobn == 0) || errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, net.ErrClosed) {
		return message{}, -1, errors.New(TagSocketClosed, "the other end of the socket is closed")
	}
	if err != nil {
		return message{}, -1, errors.NewErr(TagReceiveFailed, err)
	}
	m, fd, reason := decode(buf[:n], oob[:oobn], flags)
	if reason != "" {
		return message{}, -1, errors.New(TagMessageInvalid, "the message cannot be read", "reason", reason)
	}
	return m, fd, nil
}

// decode reads a message and the descriptors that came with it. It keeps the first descriptor
// and closes the others. When the message cannot be read, it says why, and closes every
// descriptor.
func decode(data, oob []byte, flags int) (message, int, string) {
	var fds []int
	if len(oob) > 0 {
		cmsgs, err := unix.ParseSocketControlMessage(oob)
		if err != nil {
			return message{}, -1, "the control message cannot be read"
		}
		for i := range cmsgs {
			rights, err := unix.ParseUnixRights(&cmsgs[i])
			if err == nil {
				fds = append(fds, rights...)
			}
		}
	}
	fail := func(reason string) (message, int, string) {
		for _, fd := range fds {
			// A descriptor of a message that is refused is not wanted; its close does not matter.
			_ = unix.Close(fd)
		}
		return message{}, -1, reason
	}
	if flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 {
		return fail("the message is too large")
	}
	var m message
	if err := json.Unmarshal(data, &m); err != nil {
		return fail("the message is not JSON")
	}
	if m.Type == "" {
		return fail("the message has no type")
	}
	fd := -1
	if len(fds) > 0 {
		fd = fds[0]
		for _, extra := range fds[1:] {
			// Only one descriptor is ever sent; more are not wanted.
			_ = unix.Close(extra)
		}
		unix.CloseOnExec(fd)
		if err := unix.SetNonblock(fd, true); err != nil {
			return fail("the descriptor cannot be made non-blocking")
		}
	}
	return m, fd, ""
}

// failedMessage is the message that tells the other side about an error, as the answer to the
// request seq.
func failedMessage(err error, seq int) message {
	m := message{Type: msgFailed, Seq: seq, Text: err.Error()}
	if appErr, ok := errors.FromError(err); ok {
		m.Tag = appErr.Tag
		m.Route = appErr.Route()
		if len(appErr.Metadata) > 0 {
			m.Meta = map[string]string{}
			for k, v := range appErr.Metadata {
				m.Meta[k] = toText(v)
			}
		}
	}
	return m
}
