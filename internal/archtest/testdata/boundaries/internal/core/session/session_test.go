// SPDX-License-Identifier: GPL-2.0-only

package session

// A test may start programs; it may not import gopenvpn-conn here (rule 3).
import (
	_ "os/exec"

	_ "github.com/rm4n0s/gopenvpn-conn/ovpn"
)
