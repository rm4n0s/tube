// Package b creates errors with another import name, and one per platform.
package b

import cerr "github.com/rm4n0s/errors"

// Aliased fails through an aliased import.
func Aliased() error {
	return cerr.New("Aliased", "aliased")
}
