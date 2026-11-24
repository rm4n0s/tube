// SPDX-License-Identifier: GPL-2.0-only

// Package errs breaks rule 4 twice.
package errs

import (
	"errors"
	"fmt"
)

// Fail uses both forbidden ways of making an error.
func Fail() error {
	return fmt.Errorf("wrapped: %w", errors.New("x"))
}
