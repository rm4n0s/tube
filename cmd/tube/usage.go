// SPDX-License-Identifier: GPL-2.0-only

package main

// usage is the help of the command line (docs/SPEC.md §11).
const usage = `tube [options] [--] <application> [arguments...]    run the application through a VPN
tube [options]                                      only keep the profiles (the TUI)

  -p, --profile <name|file>  use this profile and do not show the TUI: the name of a profile,
                             or an .ovpn file (".ovpn" may be left out), which is added when
                             it is not a profile yet. What is missing is asked on the terminal
      --username <name>      with --profile: the user name of the VPN
      --password <password>  with --profile: the password of the VPN. The other users of this
                             machine can see it in the list of processes: --auth-file is safer
      --auth-file <path>     with --profile: a file with the user name on the first line and
                             the password on the second
      --dns <addresses>      with --profile: the DNS servers of the application, in place of
                             the profile's ("9.9.9.9,1.1.1.1"; "" is the ones of the VPN)
      --save                 with --profile: keep the user name, the password and the DNS
                             servers of this run with the profile, without asking
  -q, --quiet                with --profile: write no lines of Tube's own while the
                             application runs
      --version              print the version
  -h, --help                 print this help
`
