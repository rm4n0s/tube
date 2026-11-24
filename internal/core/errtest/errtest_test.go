// SPDX-License-Identifier: GPL-2.0-only

package errtest

import (
	"io"
	"testing"

	"github.com/rm4n0s/errors"
)

// recorder is a testing.TB that records failures instead of failing.
type recorder struct {
	testing.TB
	errors int
	fatal  bool
}

func (r *recorder) Helper()               {}
func (r *recorder) Errorf(string, ...any) { r.errors++ }
func (r *recorder) Fatalf(string, ...any) {
	r.fatal = true
	panic(r) // Fatalf does not return
}

// produce makes an error whose route is "produce.ErrtestSample".
func produce() error {
	return errors.New("ErrtestSample", "sample", "key", "value")
}

// run calls Route with a recorder and reports what it recorded.
func run(err error, parts ...string) (meta map[string]any, failures int, fatal bool) {
	r := &recorder{}
	defer func() {
		if v := recover(); v != nil && v != any(r) {
			panic(v)
		}
		failures, fatal = r.errors, r.fatal
	}()
	meta = Route(r, err, parts...)
	return meta, r.errors, r.fatal
}

func TestRoute(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		parts        []string
		wantFailures int
		wantFatal    bool
	}{
		{"the route matches", produce(), []string{"produce.ErrtestSample"}, 0, false},
		{"every part matches", produce(), []string{"produce.", ".ErrtestSample"}, 0, false},
		{"another Tag", produce(), []string{"produce.OtherTag"}, 1, false},
		{"one part of two is wrong", produce(), []string{"produce.ErrtestSample", "elsewhere.ErrtestSample"}, 1, false},
		{"no error", nil, []string{"produce.ErrtestSample"}, 0, true},
		{"an error of another type", io.EOF, []string{"produce.ErrtestSample"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, failures, fatal := run(tt.err, tt.parts...)
			if failures != tt.wantFailures || fatal != tt.wantFatal {
				t.Errorf("failures = %d, fatal = %v; want %d, %v", failures, fatal, tt.wantFailures, tt.wantFatal)
			}
			if !tt.wantFatal && meta["key"] != "value" {
				t.Errorf("metadata = %v", meta)
			}
		})
	}
}
