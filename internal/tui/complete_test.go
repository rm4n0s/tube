// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"io/fs"
	"os"
	"slices"
	"testing"
)

// dirEntry is an entry of a fake folder.
type dirEntry struct {
	name string
	dir  bool
}

func (e dirEntry) Name() string { return e.name }
func (e dirEntry) IsDir() bool  { return e.dir }
func (e dirEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e dirEntry) Info() (fs.FileInfo, error) { return nil, os.ErrNotExist }

// fakeDirs is a readDir over folders in memory.
func fakeDirs(dirs map[string][]dirEntry) func(string) ([]os.DirEntry, error) {
	return func(dir string) ([]os.DirEntry, error) {
		entries, ok := dirs[dir]
		if !ok {
			return nil, os.ErrPermission
		}
		out := make([]os.DirEntry, len(entries))
		for i, e := range entries {
			out[i] = e
		}
		return out, nil
	}
}

func TestComplete(t *testing.T) {
	dirs := fakeDirs(map[string][]dirEntry{
		"/vpn/":      {{"work.ovpn", false}, {"home.ovpn", false}, {"homelab.ovpn", false}, {"keys", true}, {".hidden.ovpn", false}, {"a\x1bb.ovpn", false}},
		"/home/ada/": {{"Downloads", true}},
		".":          {{"local.ovpn", false}, {"notes", true}},
	})
	tests := []struct {
		name, input, want string
		names             []string
	}{
		{"one file", "/vpn/w", "/vpn/work.ovpn", nil},
		{"one folder", "/vpn/k", "/vpn/keys/", nil},
		{"several with a common beginning", "/vpn/h", "/vpn/home", []string{"home.ovpn", "homelab.ovpn"}},
		{"several without one", "/vpn/", "/vpn/", []string{"a b.ovpn", "home.ovpn", "homelab.ovpn", "keys/", "work.ovpn"}},
		{"no match", "/vpn/z", "/vpn/z", nil},
		{"the home folder", "~/D", "~/Downloads/", nil},
		{"a tilde alone", "~", "~/", nil},
		{"a hidden file", "/vpn/.h", "/vpn/.hidden.ovpn", nil},
		{"a folder that cannot be read", "/root/x", "/root/x", nil},
		{"an empty input", "", "", []string{"local.ovpn", "notes/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, names := complete(tt.input, "/home/ada", dirs)
			if got != tt.want || !slices.Equal(names, tt.names) {
				t.Errorf("complete(%q) = %q, %q; want %q, %q", tt.input, got, names, tt.want, tt.names)
			}
		})
	}
}
