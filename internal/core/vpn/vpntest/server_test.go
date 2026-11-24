// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_server

package vpntest

import (
	"strings"
	"testing"
)

func TestServerStartsAndStops(t *testing.T) {
	s := StartServer(t, ServerOptions{})
	if !s.Running() {
		t.Fatal("the server does not run after StartServer")
	}
	s.Stop()
	if s.Running() {
		t.Fatal("the server still runs after Stop")
	}
	s.Stop() // a second Stop does nothing
	s.Start()
	if !s.Running() {
		t.Fatal("the server does not run after Start")
	}
	if n := strings.Count(s.Log(), readyLine); n != 2 {
		t.Errorf("the log has %d ready lines, want 2:\n%s", n, s.Log())
	}
}
