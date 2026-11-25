// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package sandbox

import "syscall"

// ExitStatus is how the application ended.
type ExitStatus struct {
	// Code is the exit code, when Signal is 0.
	Code int
	// Signal is the signal that ended it, or 0.
	Signal syscall.Signal
}

// Status is the exit status that Tube ends with: the code, or 128 plus the signal
// (docs/SPEC.md §10).
func (s ExitStatus) Status() int {
	if s.Signal != 0 {
		return 128 + int(s.Signal)
	}
	return s.Code
}
