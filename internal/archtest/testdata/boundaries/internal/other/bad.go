// SPDX-License-Identifier: GPL-2.0-only

// Package other breaks rule 2 twice.
package other

import (
	_ "charm.land/lipgloss/v2"
	_ "github.com/rm4n0s/tube/internal/tui"
)
