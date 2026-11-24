// SPDX-License-Identifier: GPL-2.0-only

// Command errortags lists the error Tags of Tube's non-test code and, with --untested, the Tags
// that no test asserts (docs/SPEC.md §4.2 rule 7). scripts/error-tags.sh runs it.
//
//	errortags <root>                    every Tag: Tag, file:line, function
//	errortags --untested <root>         the Tags no test asserts, except those of docs/ERROR-TAGS-UNTESTED.md
//	errortags --untested --all <root>   the same, with the excepted ones kept in
//
// The exit status is 1 when --untested printed something, or when a Tag is not a string literal
// or a Tag constant, or an error is created outside a function.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/archtest"
)

const (
	// tagErrortagsUsage is the error of a command line that errortags does not understand.
	tagErrortagsUsage = "ErrortagsUsage"
	// tagErrortagsRootMissing is the error of a command line without exactly one module root.
	tagErrortagsRootMissing = "ErrortagsRootMissing"
	// tagErrortagsProblemWriteFailed is the error of a problem that could not be printed.
	tagErrortagsProblemWriteFailed = "ErrortagsProblemWriteFailed"
	// tagErrortagsListWriteFailed is the error of a list that could not be printed.
	tagErrortagsListWriteFailed = "ErrortagsListWriteFailed"
	// tagErrortagsFound is the error that makes the exit status 1 after the findings are printed.
	tagErrortagsFound = "ErrortagsFound"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		appErr, ok := errors.FromError(err)
		if !ok || appErr.Tag != tagErrortagsFound {
			fmt.Fprintln(os.Stderr, "errortags:", err)
		}
		if ok && (appErr.Tag == tagErrortagsUsage || appErr.Tag == tagErrortagsRootMissing) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// run is main with its arguments and streams given, so that tests can call it.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("errortags", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	untested := fs.Bool("untested", false, "list only the Tags that no test asserts")
	all := fs.Bool("all", false, "with --untested, keep the Tags of the exceptions file in")
	if err := fs.Parse(args); err != nil {
		return errors.NewErr(tagErrortagsUsage, err)
	}
	if fs.NArg() != 1 {
		return errors.New(tagErrortagsRootMissing, "usage: errortags [--untested [--all]] <module root>")
	}
	report, err := archtest.ScanTags(fs.Arg(0))
	if err != nil {
		return err
	}
	for _, p := range report.Problems {
		if _, err := fmt.Fprintf(stderr, "%s:%d: %s\n", p.File, p.Line, p.Message); err != nil {
			return errors.NewErr(tagErrortagsProblemWriteFailed, err)
		}
	}
	uses := report.Uses
	if *untested {
		uses = report.Untested(*all)
	}
	for _, use := range uses {
		if _, err := fmt.Fprintf(stdout, "%s\t%s:%d\t%s\n", use.Tag, use.File, use.Line, use.Func); err != nil {
			return errors.NewErr(tagErrortagsListWriteFailed, err)
		}
	}
	if len(report.Problems) > 0 || (*untested && len(uses) > 0) {
		return errors.New(tagErrortagsFound, "untested or malformed error Tags were found",
			"problems", len(report.Problems), "untested", len(uses))
	}
	return nil
}
