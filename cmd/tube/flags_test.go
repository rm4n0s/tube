// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"reflect"
	"testing"

	"github.com/rm4n0s/tube/internal/core/errtest"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		want  flags
		route string
	}{
		{"no arguments", nil, flags{target: nil}, ""},
		{"an application", []string{"firefox"}, flags{target: []string{"firefox"}}, ""},
		{"the options of the application are its own", []string{"ping", "-c", "3", "host"}, flags{target: []string{"ping", "-c", "3", "host"}}, ""},
		{"-p", []string{"-p", "work", "ping"}, flags{profile: "work", target: []string{"ping"}}, ""},
		{"--profile and --", []string{"--profile", "work", "--", "ping", "-c", "1", "host"}, flags{profile: "work", target: []string{"ping", "-c", "1", "host"}}, ""},
		{"--profile=", []string{"--profile=work", "ping"}, flags{profile: "work", target: []string{"ping"}}, ""},
		{"-q", []string{"-q", "-p", "work", "ping"}, flags{profile: "work", quiet: true, target: []string{"ping"}}, ""},
		{"--auth-file", []string{"--auth-file", "f", "-p", "work", "ping"}, flags{profile: "work", authFile: "f", target: []string{"ping"}}, ""},
		{"--version", []string{"--version"}, flags{version: true, target: nil}, ""},
		{"-h", []string{"-h"}, flags{help: true, target: nil}, ""},
		{"an option that is not known", []string{"--nosuch"}, flags{}, "parseFlags.TubeUsage"},
		{"--profile without a value", []string{"--profile"}, flags{}, "parseFlags.TubeUsage"},
		{"--profile without an application", []string{"--profile", "work"}, flags{}, "parseFlags.TubeProfileWithoutApplication"},
		{"--auth-file without --profile", []string{"--auth-file", "f", "ping"}, flags{}, "parseFlags.TubeAuthFileWithoutProfile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlags(tt.args)
			if tt.route != "" {
				errtest.Route(t, err, tt.route)
				return
			}
			if len(got.target) == 0 {
				got.target = nil // none is none
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFlags = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}
