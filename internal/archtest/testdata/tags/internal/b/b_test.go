package b_test

import (
	"testing"

	"example.com/fixture/internal/a"
	"example.com/fixture/internal/b"
	"github.com/rm4n0s/errors"
)

func TestTags(t *testing.T) {
	if appErr, _ := errors.FromError((&a.Store{}).Save()); appErr.Tag != a.TagAlpha {
		t.Error("alpha")
	}
	for _, err := range []error{b.Aliased(), b.Open(), b.Seek()} {
		appErr, _ := errors.FromError(err)
		if !appErr.HasRoute("Aliased.Aliased") && !appErr.HasRoute("Open.OpenFailed") && !appErr.HasRoute("Seek.SeekFailed") {
			t.Error(appErr.Route())
		}
	}
}
