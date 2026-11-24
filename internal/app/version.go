// SPDX-License-Identifier: GPL-2.0-only

// Package app is the application behind Tube's terminal UI and its command line: it owns the
// store of profiles, connects to a VPN and runs the session (docs/SPEC.md §12).
package app

// devVersion is the version being developed. It is what Version returns for a build that is not
// a release build. Bump it when a version is published (CLAUDE.md "Git and GitHub").
const devVersion = "0.2.0-dev"

// buildVersion is set by a release build (scripts/build-release.sh, with -ldflags -X) and by
// nothing else: the code never assigns it.
var buildVersion string

// Version returns Tube's version: the one of the release build, or the development version when
// the program was built any other way (docs/SPEC.md §11).
func Version() string {
	return versionFrom(buildVersion)
}

// versionFrom is Version for a given value of the release build.
func versionFrom(build string) string {
	if build == "" {
		return devVersion
	}
	return build
}
