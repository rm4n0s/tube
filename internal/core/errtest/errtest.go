// SPDX-License-Identifier: GPL-2.0-only

// Package errtest has the assertion every test of an error path makes: that the error is an
// *errors.Error and came the expected way (CLAUDE.md "Testing errors").
package errtest

import (
	"testing"

	"github.com/rm4n0s/errors"
)

// Route fails the test unless err is an *errors.Error whose route contains every given part,
// and returns the error's metadata. A part is a piece of a route that ends with the Tag, such as
// "Store.Set.SettingInvalid"; several parts are for a route with a library's frames in the
// middle. With a nil err, or an error of another type, the test stops.
func Route(t testing.TB, err error, parts ...string) map[string]any {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with the route %v, got nil", parts)
	}
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	for _, part := range parts {
		if !appErr.HasRoute(part) {
			t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), part)
		}
	}
	return appErr.Metadata
}
