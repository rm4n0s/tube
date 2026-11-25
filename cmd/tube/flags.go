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
	TagOptionWithoutProfile      = "TubeOptionWithoutProfile"
	TagAuthFileWithCredentials   = "TubeAuthFileWithCredentials"
)

// flags are the options of the command line (docs/SPEC.md §11).
type flags struct {
	// profile is the name of a profile, or the path of a profile file.
	profile  string
	authFile string
	// username and password are the credentials of the VPN; empty is not given.
	username string
	password string
	// dns are the DNS servers of the application, when dnsGiven: an empty --dns is the ones
	// of the VPN, and is not the same as no --dns.
	dns      string
	dnsGiven bool
	// save keeps what this run was given with the profile, without asking.
	save    bool
	quiet   bool
	version bool
	help    bool
	// target is the application and its arguments.
	target []string
}

// parseFlags reads the options. They end at the first argument that is not one, or at "--":
// everything after is the application and its arguments, untouched. "-profile" and "--profile"
// are the same. An error never has the value of an option in it, only its name.
func parseFlags(args []string) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("tube", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.profile, "profile", "", "")
	fs.StringVar(&f.profile, "p", "", "")
	fs.StringVar(&f.authFile, "auth-file", "", "")
	fs.StringVar(&f.username, "username", "", "")
	fs.StringVar(&f.password, "password", "", "")
	fs.StringVar(&f.dns, "dns", "", "")
	fs.BoolVar(&f.save, "save", false, "")
	fs.BoolVar(&f.quiet, "quiet", false, "")
	fs.BoolVar(&f.quiet, "q", false, "")
	fs.BoolVar(&f.version, "version", false, "")
	fs.BoolVar(&f.help, "help", false, "")
	fs.BoolVar(&f.help, "h", false, "")
	if err := fs.Parse(args); err != nil {
		return flags{}, errors.NewErr(TagUsage, err)
	}
	f.target = fs.Args()
	// given are the options that go with --profile and were written, in the order of the help.
	var given []string
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "dns":
			f.dnsGiven = true
			given = append(given, fl.Name)
		case "username", "password", "save":
			given = append(given, fl.Name)
		}
	})
	switch {
	case f.help || f.version:
		return f, nil
	case f.profile != "" && len(f.target) == 0:
		return flags{}, errors.New(TagProfileWithoutApplication, "--profile needs an application to run")
	case f.authFile != "" && f.profile == "":
		return flags{}, errors.New(TagAuthFileWithoutProfile, "--auth-file goes with --profile")
	case len(given) > 0 && f.profile == "":
		// The value of the option is not told: it may be a password.
		return flags{}, errors.NewErrf(TagOptionWithoutProfile, "--%s goes with --profile", given[0]).SetMetadata("option", given[0])
	case f.authFile != "" && (f.username != "" || f.password != ""):
		return flags{}, errors.New(TagAuthFileWithCredentials, "--auth-file has the user name and the password: leave --username and --password out")
	}
	return f, nil
}
