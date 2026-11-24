// SPDX-License-Identifier: GPL-2.0-only

// Package insecure breaks rule 11 twice.
package insecure

import "crypto/tls"

// Config returns a configuration that verifies nothing.
func Config() *tls.Config {
	c := &tls.Config{InsecureSkipVerify: true}
	c.InsecureSkipVerify = true
	return c
}
