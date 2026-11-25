// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

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
	if s := sentenceOfTag(appErr); s != "" {
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

// tagSentence is a sentence of a Tag of Tube's: a format whose arguments are the values of the
// error's metadata under keys, and the sentence for an error that lacks them ("" when the format
// needs none).
type tagSentence struct {
	tags    []string
	format  string
	keys    []string
	without string
}

// tagSentences are the sentences of the Tags that Tube knows, besides the kinds of failure of a
// connection and the problems of the parser.
var tagSentences = []tagSentence{
	{tags: []string{vpn.TagCredentialsNeeded}, format: "This profile needs a user name and a password."},
	{
		tags: []string{profiles.TagProfileRunsScripts}, format: "This profile runs scripts (%s). Tube never runs them.",
		keys: []string{"options"}, without: "This profile runs scripts. Tube never runs them.",
	},
	{tags: []string{profiles.TagProfileNameTaken}, format: "Another profile is already called %q.", keys: []string{"name"}},
	{tags: []string{profiles.TagProfileNameEmpty}, format: "A profile needs a name."},
	{tags: []string{profiles.TagProfileNameTooLong}, format: "The name is too long (" + strconv.Itoa(profiles.MaxNameLen) + " characters at most)."},
	{tags: []string{profiles.TagProfileFileTooLarge}, format: "The file %q of the profile is too large.", keys: []string{"file"}},
	{tags: []string{profiles.TagProfileDNSInvalid}, format: "%q is not an IP address.", keys: []string{"value"}},
	{
		tags:   []string{profiles.TagProfileDNSTooMany, profiles.TagProfileTooManyDNS},
		format: "A profile can have " + strconv.Itoa(profiles.MaxDNS) + " DNS servers at most.",
	},
	{tags: []string{TagProfileNotFound}, format: "There is no profile called %q.", keys: []string{"name"}},
	{
		tags:   []string{TagProfileAmbiguous},
		format: "More than one profile is called %q when the case is ignored. Write the name exactly.", keys: []string{"name"},
	},
	{
		tags:   []string{sandbox.TagUserNamespaceDenied},
		format: `This machine does not let an ordinary user make a user namespace, which Tube needs. See docs/GUIDE.md ("What Tube needs").`,
	},
	{tags: []string{sandbox.TagTunUnavailable}, format: "Tube cannot open /dev/net/tun."},
	{
		tags: []string{sandbox.TagLookPathFailed, sandbox.TagAppNotFound}, format: "The program %q was not found.",
		keys: []string{"name"}, without: "The program was not found.",
	},
	{tags: []string{sandbox.TagAppNotExecutable}, format: "The program cannot be executed."},
}

// sentenceOfTag is the sentence of a Tag of Tube's, or "" for another.
func sentenceOfTag(appErr *errors.Error) string {
	for _, ts := range tagSentences {
		if !slices.Contains(ts.tags, appErr.Tag) {
			continue
		}
		args := make([]any, len(ts.keys))
		for i, k := range ts.keys {
			v := meta(appErr, k)
			if v == "" && ts.without != "" {
				return ts.without
			}
			args[i] = v
		}
		if len(args) == 0 {
			return ts.format
		}
		return fmt.Sprintf(ts.format, args...)
	}
	return ""
}

// Sentences are the sentences that Explain gives for the Tags it knows, with "<key>" where a
// piece of the error goes: for docs/GUIDE.md and its test.
func Sentences() []string {
	var out []string
	for _, kind := range []vpn.Kind{vpn.KindAuthFailed, vpn.KindNoAnswer, vpn.KindCertificate, vpn.KindServerEnded, vpn.KindUnsupported, vpn.KindCancelled} {
		out = append(out, kindSentences[kind])
	}
	for _, ts := range tagSentences {
		args := make([]any, len(ts.keys))
		for i, k := range ts.keys {
			args[i] = "<" + k + ">"
		}
		if len(args) == 0 {
			out = append(out, ts.format)
		} else {
			out = append(out, fmt.Sprintf(ts.format, args...))
		}
		if ts.without != "" {
			out = append(out, ts.without)
		}
	}
	for _, kind := range []profiles.ProblemKind{
		profiles.ProblemUnsupported, profiles.ProblemBadOption, profiles.ProblemMissing, profiles.ProblemFile,
		profiles.ProblemSyntax, profiles.ProblemTooLarge,
	} {
		sentence := problemSentence(profiles.Problem{Kind: kind, Option: "<option>", Line: 1, Reason: "<reason>"})
		out = append(out, strings.ReplaceAll(sentence, "line 1", "line <line>"))
	}
	return out
}
