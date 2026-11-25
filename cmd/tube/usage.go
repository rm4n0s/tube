// SPDX-License-Identifier: GPL-2.0-only

package main

// usage is the help of the command line (docs/SPEC.md §11).
const usage = `tube [options] [--] <application> [arguments...]    run the application through a VPN
tube [options]                                      only keep the profiles (the TUI)

  -p, --profile <name>    use this profile and do not show the TUI
      --auth-file <path>  with --profile: a file with the user name on the first line and
                          the password on the second
  -q, --quiet             with --profile: write no lines of Tube's own while the application runs
      --version           print the version
  -h, --help              print this help
`
