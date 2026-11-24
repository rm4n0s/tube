// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/gopenvpn-conn/ovpn"
	"github.com/rm4n0s/tube/internal/core/sanitize"
)

// Problem is what the parser said about a profile, for the rest of Tube, which does not import
// the parser. Every string is safe to draw.
type Problem struct {
	// Kind is the kind of problem; ProblemNone when the error is not the parser's.
	Kind ProblemKind
	// Option is the option's name, or "".
	Option string
	// Line is the line, from 1, or 0.
	Line int
	// Reason is the parser's reason, or "".
	Reason string
}

// ProblemKind is the kind of a Problem.
type ProblemKind int

// The kinds of Problem.
const (
	ProblemNone        ProblemKind = iota // the error is not the parser's
	ProblemUnsupported                    // an option that is not supported
	ProblemBadOption                      // an option whose arguments are wrong
	ProblemMissing                        // an option that the profile needs is not there
	ProblemFile                           // a file of the profile cannot be read or is not right
	ProblemSyntax                         // the profile does not follow the grammar
	ProblemTooLarge                       // the profile is too large
	ProblemUnreadable                     // the profile cannot be read
)

// problemKinds maps the Tags of the parser to the kinds.
var problemKinds = map[string]ProblemKind{
	ovpn.TagUnsupportedOption: ProblemUnsupported,
	ovpn.TagBadOption:         ProblemBadOption,
	ovpn.TagMissingOption:     ProblemMissing,
	ovpn.TagFileUnavailable:   ProblemFile,
	ovpn.TagBadFile:           ProblemFile,
	ovpn.TagSyntax:            ProblemSyntax,
	ovpn.TagProfileTooLarge:   ProblemTooLarge,
	ovpn.TagReadFailed:        ProblemUnreadable,
}

// ProblemOf reads the Tag and the metadata of an error of the parser.
func ProblemOf(err error) Problem {
	appErr, ok := errors.FromError(err)
	if !ok {
		return Problem{}
	}
	kind, ok := problemKinds[appErr.Tag]
	if !ok {
		return Problem{}
	}
	p := Problem{Kind: kind}
	if option, ok := appErr.Metadata["option"].(string); ok {
		p.Option = sanitize.Display(option, 64)
	}
	if line, ok := appErr.Metadata["line"].(int); ok && line > 0 {
		p.Line = line
	}
	if reason, ok := appErr.Metadata["reason"].(string); ok {
		p.Reason = sanitize.Display(reason, 200)
	}
	return p
}
