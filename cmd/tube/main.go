// SPDX-License-Identifier: GPL-2.0-only

// Command tube runs one application through a VPN, and only that application
// (docs/SPEC.md §1, §11).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/sandbox"
)

const (
	// program is the name of this program, as the messages say it.
	program = "tube"
	// statusOK is the exit status of a run that did what it was asked.
	statusOK = 0
	// statusFailed is the exit status of a failure of Tube itself (docs/SPEC.md §10).
	statusFailed = 125
)

func main() {
	// The keeper is this same binary, started again inside the namespace (docs/SPEC.md §8.1).
	if sandbox.IsKeeper() {
		os.Exit(sandbox.KeeperMain())
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the program with its arguments and streams given, so that tests can call it. It
// returns the exit status.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		if _, err := fmt.Fprintln(stdout, program, app.Version()); err != nil {
			return statusFailed // there is nowhere to tell: standard output is closed
		}
		return statusOK
	}
	// TODO(step 3.2): the options of docs/SPEC.md §11 and --profile.
	// TODO(step 4.2): the TUI.
	if _, err := fmt.Fprintln(stderr, program+": not implemented yet"); err != nil {
		return statusFailed
	}
	return statusFailed
}
