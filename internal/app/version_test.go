// SPDX-License-Identifier: GPL-2.0-only

package app

import "testing"

func TestVersionFrom(t *testing.T) {
	tests := []struct {
		name  string
		build string
		want  string
	}{
		{"not a release build", "", devVersion},
		{"a release build", "0.7.0", "0.7.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionFrom(tt.build); got != tt.want {
				t.Errorf("versionFrom(%q) = %q, want %q", tt.build, got, tt.want)
			}
		})
	}
}

func TestVersionIsTheDevelopmentVersionInTests(t *testing.T) {
	if got := Version(); got != devVersion {
		t.Errorf("Version() = %q, want %q", got, devVersion)
	}
}
