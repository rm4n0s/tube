// SPDX-License-Identifier: GPL-2.0-only

// Package app breaks rule 1 twice and imports what it may.
package app

import (
	_ "charm.land/lipgloss/v2"
	_ "github.com/rm4n0s/tube/internal/core/ok"
	_ "github.com/rm4n0s/tube/internal/tui"
)
