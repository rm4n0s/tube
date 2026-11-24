// SPDX-License-Identifier: GPL-2.0-only

//go:build linux

package nstest

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	Main(m, map[string]func() error{
		"whoami": func() error {
			ifaces, err := net.Interfaces()
			if err != nil {
				return err
			}
			var names []string
			for _, i := range ifaces {
				names = append(names, i.Name)
			}
			fmt.Printf("%d %s\n", os.Getuid(), strings.Join(names, ","))
			return nil
		},
		"env": func() error {
			fmt.Print(os.Getenv("NSTEST_VALUE"))
			return nil
		},
		"fail": func() error {
			fmt.Print("before the failure")
			return fmt.Errorf("the function failed on purpose")
		},
	})
}

func TestRun(t *testing.T) {
	got := Run(t, "whoami")
	if got != "0 lo\n" {
		t.Errorf("the function printed %q, want uid 0 and only the loopback", got)
	}
}

func TestRunPassesTheEnvironment(t *testing.T) {
	if got := Run(t, "env", "NSTEST_VALUE=hello"); got != "hello" {
		t.Errorf("the function printed %q", got)
	}
}

// recorder is a testing.TB that records a fatal failure instead of stopping the test.
type recorder struct {
	testing.TB
	message string
}

func (r *recorder) Helper() {}
func (r *recorder) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
	panic(r) // Fatalf does not return
}
func (r *recorder) Skipf(format string, args ...any) {
	r.message = "skipped: " + fmt.Sprintf(format, args...)
	panic(r)
}

// runRecorded calls Run with a recorder and returns what it recorded.
func runRecorded(name string) (message string) {
	r := &recorder{}
	defer func() {
		if v := recover(); v != nil && v != any(r) {
			panic(v)
		}
		message = r.message
	}()
	Run(r, name)
	return r.message
}

func TestRunFailsTheTest(t *testing.T) {
	Require(t)
	tests := []struct {
		name string
		want []string
	}{
		{"fail", []string{"the function fail failed", "before the failure", "the function failed on purpose"}},
		{"no-such-function", []string{"no function called"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := runRecorded(tt.name)
			for _, want := range tt.want {
				if !strings.Contains(message, want) {
					t.Errorf("the failure says %q, want %q in it", message, want)
				}
			}
		})
	}
}
