// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/rm4n0s/tube/internal/app"
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
		{"the credentials", []string{"-p", "work", "--username", "ada", "--password", "secret", "ping"},
			flags{profile: "work", username: "ada", password: "secret", target: []string{"ping"}}, ""},
		{"--dns and --save", []string{"-p", "work", "--dns", "9.9.9.9,1.1.1.1", "--save", "ping"},
			flags{profile: "work", dns: "9.9.9.9,1.1.1.1", dnsGiven: true, save: true, target: []string{"ping"}}, ""},
		{"an empty --dns is given too", []string{"-p", "work", "--dns", "", "ping"}, flags{profile: "work", dnsGiven: true, target: []string{"ping"}}, ""},
		{"--dns=", []string{"-p", "work", "--dns=", "ping"}, flags{profile: "work", dnsGiven: true, target: []string{"ping"}}, ""},
		{"a file as the profile", []string{"-p", "~/vpn/lab.ovpn", "ping"}, flags{profile: "~/vpn/lab.ovpn", target: []string{"ping"}}, ""},
		{"the options of the application are not Tube's", []string{"-p", "work", "curl", "--dns", "x", "--save"},
			flags{profile: "work", target: []string{"curl", "--dns", "x", "--save"}}, ""},
		{"--version", []string{"--version"}, flags{version: true, target: nil}, ""},
		{"-h", []string{"-h"}, flags{help: true, target: nil}, ""},
		{"an option that is not known", []string{"--nosuch"}, flags{}, "parseFlags.TubeUsage"},
		{"--profile without a value", []string{"--profile"}, flags{}, "parseFlags.TubeUsage"},
		{"--profile without an application", []string{"--profile", "work"}, flags{}, "parseFlags.TubeProfileWithoutApplication"},
		{"--auth-file without --profile", []string{"--auth-file", "f", "ping"}, flags{}, "parseFlags.TubeAuthFileWithoutProfile"},
		{"--username without --profile", []string{"--username", "ada", "ping"}, flags{}, "parseFlags.TubeOptionWithoutProfile"},
		{"--password without --profile", []string{"--password", "hunter2", "ping"}, flags{}, "parseFlags.TubeOptionWithoutProfile"},
		{"--dns without --profile", []string{"--dns", "9.9.9.9", "ping"}, flags{}, "parseFlags.TubeOptionWithoutProfile"},
		{"--save without --profile", []string{"--save", "ping"}, flags{}, "parseFlags.TubeOptionWithoutProfile"},
		{"--auth-file with --username", []string{"-p", "work", "--auth-file", "f", "--username", "ada", "ping"}, flags{}, "parseFlags.TubeAuthFileWithCredentials"},
		{"--auth-file with --password", []string{"-p", "work", "--auth-file", "f", "--password", "hunter2", "ping"}, flags{}, "parseFlags.TubeAuthFileWithCredentials"},
		{"--password without a value", []string{"-p", "work", "--password"}, flags{}, "parseFlags.TubeUsage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlags(tt.args)
			if tt.route != "" {
				errtest.Route(t, err, tt.route)
				// The value of an option may be a password: an error names the option only.
				if text := fmt.Sprintf("%v %+v", app.Explain(err), err); strings.Contains(text, "hunter2") {
					t.Errorf("the error has the value of the option: %s", text)
				}
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
