// SPDX-License-Identifier: GPL-2.0-only

// Package vpn may import gopenvpn-conn, but not its vpnnet (rule 3).
package vpn

import (
	_ "github.com/rm4n0s/gopenvpn-conn"
	_ "github.com/rm4n0s/gopenvpn-conn/ovpn"
	_ "github.com/rm4n0s/gopenvpn-conn/vpnnet"
)
