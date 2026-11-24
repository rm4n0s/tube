// SPDX-License-Identifier: GPL-2.0-only

package insecure

import (
	"crypto/tls"
	"testing"
)

func TestConfig(t *testing.T) {
	_ = &tls.Config{InsecureSkipVerify: true}
}
