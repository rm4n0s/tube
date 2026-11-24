// SPDX-License-Identifier: GPL-2.0-only

// Package ok breaks no rule.
package ok

import (
	"fmt"

	"github.com/rm4n0s/errors"
	_ "golang.org/x/sys/unix"
)

// Hello formats without making an error.
func Hello() (string, error) {
	return fmt.Sprintf("hello"), errors.New("OkTag", "ok")
}
