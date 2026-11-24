// SPDX-License-Identifier: GPL-2.0-only

// Package tui imports what it may, and breaks rule 2 once.
package tui

import (
	_ "os/exec"

	_ "charm.land/bubbletea/v2"
	_ "charm.land/lipgloss/v2"
	_ "github.com/rm4n0s/tube/internal/app"
	_ "github.com/rm4n0s/tube/internal/core/clock"
	_ "github.com/rm4n0s/tube/internal/core/profiles"
	_ "github.com/rm4n0s/tube/internal/core/sanitize"
	_ "github.com/rm4n0s/tube/internal/tui/widgets"
)
