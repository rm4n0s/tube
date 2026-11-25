// SPDX-License-Identifier: GPL-2.0-only

//go:build ignore

// make-three-profiles writes the configuration folder of the proofs of the TUI,
// scripts/testdata/three-profiles: the profiles Home, Work (which asks for a password) and Lab,
// with keys made for the proofs only.
//
//	go run scripts/testdata/make-three-profiles.go scripts/testdata/three-profiles
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/tube/internal/core/clock"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

// fatal is the testing.TB that profilestest needs, outside a test: a failure ends the program.
// The methods that profilestest does not call are those of the nil interface, and are never
// called.
type fatal struct{ testing.TB }

func (fatal) Helper() {}

func (fatal) Fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "make-three-profiles: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run scripts/testdata/make-three-profiles.go <config dir>")
		os.Exit(2)
	}
	config := os.Args[1]
	if _, err := os.Stat(config); err == nil {
		fmt.Fprintf(os.Stderr, "make-three-profiles: %s exists already; remove it first\n", config)
		os.Exit(1)
	}
	src, err := os.MkdirTemp("", "three-profiles-")
	if err != nil {
		fatal{}.Fatalf("%v", err)
	}
	defer func() { _ = os.RemoveAll(src) }() // only the sources, copied into the store
	pki := profilestest.NewPKI(fatal{})
	store := profiles.NewStore(config, clock.System())
	for _, p := range []struct {
		name, remote string
		password     bool
	}{
		{"Home", "home.example.net 1194", false},
		{"Work", "vpn.example.com 443", true},
		{"Lab", "lab.example.org 1194", false},
	} {
		path := profilestest.WriteProfile(fatal{}, filepath.Join(src, p.name), p.name+".ovpn", pki, profilestest.Options{Remote: p.remote, Password: p.password})
		if _, err := store.Add(context.Background(), path, p.name, profiles.AddOptions{}); err != nil {
			fatal{}.Fatalf("%s: %v", p.name, err)
		}
	}
}
