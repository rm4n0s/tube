// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"fmt"
	"strconv"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// The lengths of the pieces of a sentence.
const (
	maxPiece   = 64
	maxMessage = 300
)

// kindSentences are the sentences of the kinds of a failure of a connection.
var kindSentences = map[vpn.Kind]string{
	vpn.KindAuthFailed:  "The VPN server refused the user name or the password.",
	vpn.KindNoAnswer:    "The VPN server did not answer.",
	vpn.KindCertificate: "The certificate of the VPN server was not accepted.",
	vpn.KindServerEnded: "The VPN server closed the connection.",
	vpn.KindUnsupported: "The VPN server asks for something that Tube does not support.",
	vpn.KindCancelled:   "Cancelled.",
}

// Explain is the sentence for the user about an error (docs/SPEC.md §12.3), followed by the
// error's Tag in parentheses when it has one. It never contains a secret: the sentences of the
// known errors take only what their metadata says is safe to show, and an unknown error shows
// its message, into which no code of Tube puts a secret.
func Explain(err error) string {
	if err == nil {
		return ""
	}
	sentence := sentenceOf(err)
	if appErr, ok := errors.FromError(err); ok && appErr.Tag != "" {
		sentence += " (" + sanitize.Display(appErr.Tag, maxPiece) + ")"
	}
	return sentence
}

// ExitCode is the exit status that Tube ends with after an error of its own (docs/SPEC.md §10).
func ExitCode(err error) int { return sandbox.ExitCodeOf(err) }

// sentenceOf is the sentence of Explain, without the Tag.
func sentenceOf(err error) string {
	if p := profiles.ProblemOf(err); p.Kind != profiles.ProblemNone {
		return problemSentence(p)
	}
	if s, ok := kindSentences[vpn.KindOf(err)]; ok {
		return s
	}
	appErr, ok := errors.FromError(err)
	if !ok {
		return sanitize.Display(err.Error(), maxMessage)
	}
	if s := tagSentence(appErr); s != "" {
		return s
	}
	return sanitize.Display(appErr.Message, maxMessage)
}

// problemSentence is the sentence of a problem of the parser; a piece that is missing leaves its
// part of the sentence out.
func problemSentence(p profiles.Problem) string {
	onLine := ""
	if p.Line > 0 {
		onLine = " on line " + strconv.Itoa(p.Line)
	}
	because := ""
	if p.Reason != "" {
		because = ": " + p.Reason
	}
	switch p.Kind {
	case profiles.ProblemUnsupported:
		return fmt.Sprintf("The profile uses the option %q%s, which Tube does not support%s.", p.Option, onLine, because)
	case profiles.ProblemBadOption:
		return fmt.Sprintf("The option %q%s of the profile is not valid%s.", p.Option, onLine, because)
	case profiles.ProblemMissing:
		return fmt.Sprintf("The profile has no %q.", p.Option)
	case profiles.ProblemFile:
		return fmt.Sprintf("A file that the profile names with %q cannot be used. Put it next to the profile, or inside the profile.", p.Option)
	case profiles.ProblemSyntax:
		if p.Line > 0 {
			return fmt.Sprintf("The profile cannot be read: a mistake on line %d.", p.Line)
		}
		return "The profile cannot be read: a mistake."
	case profiles.ProblemTooLarge:
		return "The profile is too large."
	}
	return "The profile cannot be read."
}

// meta is a value of the error's metadata as text that is safe to show, or "".
func meta(appErr *errors.Error, key string) string {
	v, ok := appErr.Metadata[key]
	if !ok {
		return ""
	}
	return sanitize.Display(fmt.Sprint(v), maxPiece)
}

// tagSentence is the sentence of a Tag of Tube's, or "" for another.
func tagSentence(appErr *errors.Error) string {
	switch appErr.Tag {
	case vpn.TagCredentialsNeeded:
		return "This profile needs a user name and a password."
	case profiles.TagProfileRunsScripts:
		if options := meta(appErr, "options"); options != "" {
			return "This profile runs scripts (" + options + "). Tube never runs them."
		}
		return "This profile runs scripts. Tube never runs them."
	case profiles.TagProfileNameTaken:
		return fmt.Sprintf("Another profile is already called %q.", meta(appErr, "name"))
	case profiles.TagProfileNameEmpty:
		return "A profile needs a name."
	case profiles.TagProfileNameTooLong:
		return fmt.Sprintf("The name is too long (%d characters at most).", profiles.MaxNameLen)
	case profiles.TagProfileFileTooLarge:
		return fmt.Sprintf("The file %q of the profile is too large.", meta(appErr, "file"))
	case profiles.TagProfileDNSInvalid:
		return fmt.Sprintf("%q is not an IP address.", meta(appErr, "value"))
	case profiles.TagProfileDNSTooMany, profiles.TagProfileTooManyDNS:
		return fmt.Sprintf("A profile can have %d DNS servers at most.", profiles.MaxDNS)
	case TagProfileNotFound:
		return fmt.Sprintf("There is no profile called %q.", meta(appErr, "name"))
	case TagProfileAmbiguous:
		return fmt.Sprintf("More than one profile is called %q when the case is ignored. Write the name exactly.", meta(appErr, "name"))
	case sandbox.TagUserNamespaceDenied:
		return `This machine does not let an ordinary user make a user namespace, which Tube needs. See docs/GUIDE.md ("What Tube needs").`
	case sandbox.TagTunUnavailable:
		return "Tube cannot open /dev/net/tun."
	case sandbox.TagLookPathFailed, sandbox.TagAppNotFound:
		if name := meta(appErr, "name"); name != "" {
			return fmt.Sprintf("The program %q was not found.", name)
		}
		return "The program was not found."
	case sandbox.TagAppNotExecutable:
		return "The program cannot be executed."
	}
	return ""
}
