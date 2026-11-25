// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/sanitize"
)

// TagLookPathFailed is the Tag of an application that is not found in PATH.
const TagLookPathFailed = "SandboxLookPathFailed"

// The exit statuses of Tube for an application that cannot be started (docs/SPEC.md §10).
const (
	StatusNotFound      = 127
	StatusNotExecutable = 126
	StatusTubeFailed    = 125
)

// maxNameInError is the length of an application's name in an error.
const maxNameInError = 255

// LookPath finds the application in PATH, as a shell would, and returns its absolute path. A
// name with a slash is a path, which is only made absolute: whether it exists and can be
// executed is told when it is started (SandboxAppNotFound, SandboxAppNotExecutable). A
// relative folder in PATH is never used.
func LookPath(name string) (string, error) {
	var path string
	var err error
	if strings.Contains(name, "/") {
		path, err = filepath.Abs(name)
	} else {
		path, err = exec.LookPath(name)
		if err == nil {
			path, err = filepath.Abs(path)
		}
	}
	if err != nil {
		// The cause is not kept: its text has the name as it is, which may hold anything.
		return "", errors.New(TagLookPathFailed, "the application is not found", "name", sanitize.Display(name, maxNameInError))
	}
	return path, nil
}

// ExitCodeOf is the exit status that Tube ends with for an error of starting the application:
// 127 when it was not found, 126 when it cannot be executed, else 125.
func ExitCodeOf(err error) int {
	appErr, ok := errors.FromError(err)
	switch {
	case ok && (appErr.Tag == TagLookPathFailed || appErr.Tag == TagAppNotFound):
		return StatusNotFound
	case ok && appErr.Tag == TagAppNotExecutable:
		return StatusNotExecutable
	}
	return StatusTubeFailed
}
