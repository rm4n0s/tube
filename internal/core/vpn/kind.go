// SPDX-License-Identifier: GPL-2.0-only

package vpn

import (
	"context"
	"io"

	"github.com/rm4n0s/errors"
	openvpn "github.com/rm4n0s/gopenvpn-conn"
	"github.com/rm4n0s/gopenvpn-conn/ovpn"
)

// Kind is what kind of failure of a connection an error is, for the rest of Tube, which does
// not import gopenvpn-conn.
type Kind int

// The kinds of failure.
const (
	KindOther          Kind = iota // none of the others
	KindAuthFailed                 // the server refused the user name or the password
	KindNoAnswer                   // the server did not answer, or the link to it failed
	KindCertificate                // the server's certificate was not accepted
	KindServerEnded                // the server ended the connection
	KindUnsupported                // the server asks for what the library does not support
	KindCancelled                  // the context was cancelled or its deadline passed
	KindPacketTooLarge             // a packet larger than the tunnel's MTU
)

// kinds maps the Tags of gopenvpn-conn to the kinds.
var kinds = map[string]Kind{
	openvpn.TagAuthFailed:                KindAuthFailed,
	openvpn.TagServerPollTimeout:         KindNoAnswer,
	openvpn.TagDialFailed:                KindNoAnswer,
	openvpn.TagHandshakeTimeout:          KindNoAnswer,
	openvpn.TagLinkFailed:                KindNoAnswer,
	openvpn.TagPingTimeout:               KindNoAnswer,
	openvpn.TagTLSHandshakeFailed:        KindCertificate,
	openvpn.TagServerFingerprintMismatch: KindCertificate,
	openvpn.TagServerCertificateUsage:    KindCertificate,
	openvpn.TagServerNameMismatch:        KindCertificate,
	openvpn.TagServerHalt:                KindServerEnded,
	openvpn.TagServerRestart:             KindServerEnded,
	openvpn.TagServerExit:                KindServerEnded,
	openvpn.TagUnsupportedPushedOption:   KindUnsupported,
	openvpn.TagCipherNegotiationFailed:   KindUnsupported,
	openvpn.TagKeyDerivationUnsupported:  KindUnsupported,
	openvpn.TagPacketTooLarge:            KindPacketTooLarge,
	ovpn.TagCredentialsMissing:           KindOther,
}

// KindOf says what kind of failure of a connection err is.
func KindOf(err error) Kind {
	if err == nil {
		return KindOther
	}
	// First: a dial that was cancelled may carry another Tag with the context's error behind it.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return KindCancelled
	}
	if errors.Is(err, io.EOF) {
		return KindServerEnded
	}
	if appErr, ok := errors.FromError(err); ok {
		return kinds[appErr.Tag]
	}
	return KindOther
}
