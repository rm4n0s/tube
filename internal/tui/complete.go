// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

// maxCandidates is the number of names that a completion shows, at most.
const maxCandidates = 20

// complete finishes the last part of a path as far as the names in its folder allow. input is
// what the user typed; home replaces a leading "~". It returns the new input and the names that
// match when there is more than one (at most 20), made safe to draw, folders with a "/".
func complete(input, home string, readDir func(dir string) ([]os.DirEntry, error)) (string, []string) {
	if input == "~" {
		return "~/", nil
	}
	dirPart, namePart := "", input
	if i := strings.LastIndex(input, "/"); i >= 0 {
		dirPart, namePart = input[:i+1], input[i+1:]
	}
	dir := dirPart
	if rest, ok := strings.CutPrefix(dir, "~/"); ok {
		dir = strings.TrimSuffix(home, "/") + "/" + rest
	}
	if dir == "" {
		dir = "."
	}
	entries, err := readDir(dir)
	if err != nil {
		return input, nil
	}
	var names []string
	isDir := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, namePart) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(namePart, ".")) {
			continue
		}
		names = append(names, name)
		isDir[name] = e.IsDir()
	}
	sort.Strings(names)
	switch len(names) {
	case 0:
		return input, nil
	case 1:
		name := names[0]
		if isDir[name] {
			name += "/"
		}
		return dirPart + name, nil
	}
	common := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, common) {
			_, size := utf8.DecodeLastRuneInString(common)
			common = common[:len(common)-size]
		}
	}
	shown := make([]string, 0, min(len(names), maxCandidates))
	for _, n := range names[:min(len(names), maxCandidates)] {
		if isDir[n] {
			n += "/"
		}
		shown = append(shown, clean(n))
	}
	return dirPart + common, shown
}
