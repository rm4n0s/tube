// SPDX-License-Identifier: GPL-2.0-only

// Command tube imports what it may, and breaks rules 3 and 5.
package main

import (
	_ "os/exec"

	_ "charm.land/bubbletea/v2"
	_ "github.com/rm4n0s/gopenvpn-conn/ovpn"
	_ "github.com/rm4n0s/tube/internal/app"
	_ "github.com/rm4n0s/tube/internal/core/ok"
	_ "github.com/rm4n0s/tube/internal/tui"
)
