// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"flag"
	"io"

	"github.com/rm4n0s/errors"
)

// The Tags of the mistakes in the arguments; they end Tube with status 2.
const (
	TagUsage                     = "TubeUsage"
	TagProfileWithoutApplication = "TubeProfileWithoutApplication"
	TagAuthFileWithoutProfile    = "TubeAuthFileWithoutProfile"
)

// flags are the options of the command line (docs/SPEC.md §11).
type flags struct {
	profile  string
	authFile string
	quiet    bool
	version  bool
	help     bool
	// target is the application and its arguments.
	target []string
}

// parseFlags reads the options. They end at the first argument that is not one, or at "--":
// everything after is the application and its arguments, untouched. "-profile" and "--profile"
// are the same.
func parseFlags(args []string) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("tube", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.profile, "profile", "", "")
	fs.StringVar(&f.profile, "p", "", "")
	fs.StringVar(&f.authFile, "auth-file", "", "")
	fs.BoolVar(&f.quiet, "quiet", false, "")
	fs.BoolVar(&f.quiet, "q", false, "")
	fs.BoolVar(&f.version, "version", false, "")
	fs.BoolVar(&f.help, "help", false, "")
	fs.BoolVar(&f.help, "h", false, "")
	if err := fs.Parse(args); err != nil {
		return flags{}, errors.NewErr(TagUsage, err)
	}
	f.target = fs.Args()
	switch {
	case f.help || f.version:
		return f, nil
	case f.profile != "" && len(f.target) == 0:
		return flags{}, errors.New(TagProfileWithoutApplication, "--profile needs an application to run")
	case f.authFile != "" && f.profile == "":
		return flags{}, errors.New(TagAuthFileWithoutProfile, "--auth-file goes with --profile")
	}
	return f, nil
}
