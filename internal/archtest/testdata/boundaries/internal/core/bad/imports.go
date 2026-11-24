// SPDX-License-Identifier: GPL-2.0-only

// Package bad breaks rule 1 three times.
package bad

import (
	_ "charm.land/bubbletea/v2"
	_ "github.com/rm4n0s/tube/internal/app"
	_ "github.com/rm4n0s/tube/internal/tui/widgets"
)
