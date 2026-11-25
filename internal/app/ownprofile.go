// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/tube/internal/core/storage"
)

// ownProfilePrograms are the programs that get a profile of their own from Tube (docs/SPEC.md
// §8.7): started a second time on a profile that is already open, they hand the start over to
// the program that runs, which is outside the VPN, and no setting of theirs stops that. On
// another profile they start by themselves, inside the namespace.
var ownProfilePrograms = []string{"firefox", "thunderbird"}

// ownProfileProgram says whether the application is one of ownProfilePrograms, and which: the
// name of its file is the program's ("firefox"), or begins with it and a "-" ("firefox-esr").
func ownProfileProgram(argv0 string) (string, bool) {
	base := strings.ToLower(filepath.Base(argv0))
	for _, name := range ownProfilePrograms {
		if base == name || strings.HasPrefix(base, name+"-") {
			return name, true
		}
	}
	return "", false
}

// choosesProfile says whether the arguments choose a profile themselves: -P, --profile or
// --ProfileManager, with one "-" or two, in any case, also as "--profile=...".
func choosesProfile(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		option, _, _ := strings.Cut(strings.ToLower(strings.TrimLeft(arg, "-")), "=")
		switch option {
		case "p", "profile", "profilemanager":
			return true
		}
	}
	return false
}

// ownProfileDir is the folder of the profile that Tube keeps for the program, in the state
// folder.
func (a *App) ownProfileDir(name string) string {
	return filepath.Join(a.dirs.State, name+"-profile")
}

// applicationArgv is the argv that the application is started with, and a line for the user
// about it, or "". Firefox and Thunderbird get "--no-remote --profile <Tube's own folder>"
// after their name, unless the user's arguments choose a profile. When the folder cannot be
// made the application is not started: on the user's profile it could open outside the VPN.
func (a *App) applicationArgv(ctx context.Context) ([]string, string, error) {
	argv := a.opts.Argv
	name, ok := ownProfileProgram(argv[0])
	if !ok || choosesProfile(argv[1:]) {
		return argv, "", nil
	}
	dir := a.ownProfileDir(name)
	if err := storage.EnsureDir(ctx, dir); err != nil {
		return nil, "", err
	}
	own := append([]string{argv[0], "--no-remote", "--profile", dir}, argv[1:]...)
	return own, name + " runs on a profile of its own, which Tube keeps: it is apart from a " + name + " that is already open", nil
}
