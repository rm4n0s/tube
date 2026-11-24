// SPDX-License-Identifier: GPL-2.0-only

// Package vpntest is test support: it may start programs and import gopenvpn-conn.
package vpntest

import (
	_ "os/exec"

	_ "github.com/rm4n0s/gopenvpn-conn"
)
