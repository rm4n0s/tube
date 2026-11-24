package a

import (
	"testing"

	"github.com/rm4n0s/errors"
)

func TestRoutes(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantRoute string
		wantTag   string
	}{
		{name: "literal", err: Literal(), wantRoute: "Literal.Literal"},
		{name: "keyed", err: Keyed(), wantTag: "Keyed"},
		{name: "closure", err: Closure(), wantRoute: "Closure->Closure.func1.InClosure"},
		{name: "both", err: ExceptedButTested(), wantRoute: "ExceptedButTested.ExceptedButTested"},
	}
	for _, tt := range tests {
		appErr, _ := errors.FromError(tt.err)
		if !appErr.HasRoute(tt.wantRoute) || (tt.wantTag != "" && appErr.Tag != tt.wantTag) {
			t.Errorf("%s: %s", tt.name, appErr.Route())
		}
	}
	if appErr, _ := errors.FromError(Compared()); appErr.Tag != "Compared" {
		t.Error("compared")
	}
	if appErr, _ := errors.FromError(Store{}.Load()); tagPrivate != appErr.Tag {
		t.Error("private")
	}
	if !errors.Is(Sentinel(), &errors.Error{Tag: "Sentinel"}) {
		t.Error("sentinel")
	}
	// A plain word that is not a route, a key or a comparison asserts nothing.
	t.Log("Untested")
}
