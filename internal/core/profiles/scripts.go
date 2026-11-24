// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"bytes"

	"github.com/rm4n0s/errors"
)

// MaxScriptLines is the largest number of lines that withoutScripts turns into comments.
const MaxScriptLines = 16

// The Tags of the errors of reading a profile to add it.
const (
	TagProfileFolderUnreadable = "ProfileFolderUnreadable"
	TagProfileUnreadable       = "ProfileUnreadable"
	TagProfileTooManyScripts   = "ProfileTooManyScripts"
)

// scriptOptions are the options that name a program to run. Tube never runs one.
var scriptOptions = map[string]bool{"up": true, "down": true, "up-restart": true, "down-pre": true,
	"route-up": true, "route-pre-down": true, "ipchange": true, "tls-verify": true}

// disabledPrefix is put in front of a line that withoutScripts turns into a comment.
const disabledPrefix = "# tube: disabled: "

// withoutScripts reads the profile called file in dir and parses it again and again, each time
// turning the line of the script option that the parser refused into a comment. It returns the
// text that parses, the files that it names, and the names of the options that were disabled,
// each once, in the order of their lines. When nothing was disabled, the text is the file's.
func withoutScripts(dir, file string) (text []byte, names []string, disabled []string, err error) {
	contents, failed, err := readFiles(dir, []string{file})
	if err != nil {
		if failed == "" {
			return nil, nil, nil, errors.NewErr(TagProfileFolderUnreadable, err).SetMetadata("dir", dir)
		}
		return nil, nil, nil, errors.NewErr(TagProfileUnreadable, err).SetMetadata("file", file)
	}
	text = contents[0]
	seen := map[string]bool{}
	for count := 0; ; count++ {
		_, names, err := loadWith(dir, file, text)
		if err == nil {
			return text, names, disabled, nil
		}
		problem := ProblemOf(err)
		lines := bytes.Split(text, []byte("\n"))
		if problem.Kind != ProblemUnsupported || !scriptOptions[problem.Option] || problem.Line < 1 || problem.Line > len(lines) {
			return nil, nil, nil, err
		}
		if count == MaxScriptLines {
			return nil, nil, nil, errors.New(TagProfileTooManyScripts, "the profile runs too many scripts", "max", MaxScriptLines)
		}
		lines[problem.Line-1] = append([]byte(disabledPrefix), lines[problem.Line-1]...)
		text = bytes.Join(lines, []byte("\n"))
		if !seen[problem.Option] {
			seen[problem.Option] = true
			disabled = append(disabled, problem.Option)
		}
	}
}
