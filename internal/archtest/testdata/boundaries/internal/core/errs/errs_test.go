// SPDX-License-Identifier: GPL-2.0-only

package errs

import (
	"errors"
	"fmt"
	"testing"
)

// A test may call fmt.Errorf, but it may not import the standard library errors.
func TestFail(t *testing.T) {
	if errors.Is(Fail(), fmt.Errorf("y")) {
		t.Fatal("unexpected")
	}
}
