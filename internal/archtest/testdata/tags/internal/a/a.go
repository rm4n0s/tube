// Package a creates errors in every way the scan knows.
package a

import (
	"os"

	"github.com/rm4n0s/errors"
)

// The Tags of this package.
const (
	TagAlpha   = "Alpha"
	tagPrivate = "Private"
	notATag    = "NotATag"
)

// ErrPackageLevel is created outside a function, which is a problem.
var ErrPackageLevel = errors.New("PackageLevel", "created at init")

// ErrSentinel is a correct sentinel and is not a use.
var ErrSentinel = &errors.Error{Tag: "Sentinel"}

// Store is a type with methods that fail.
type Store struct{}

// Literal fails with a literal Tag that a route asserts.
func Literal() error {
	return errors.New("Literal", "literal")
}

// Save fails with a constant Tag that another package's test mentions.
func (s *Store) Save() error {
	_, err := os.Stat("/nonexistent")
	return errors.NewErr(TagAlpha, err).SetMetadata("path", "/nonexistent")
}

// Load fails with a private constant Tag that a comparison asserts.
func (s Store) Load() error {
	return errors.NewErrf(tagPrivate, "load %d", 1)
}

// Compared fails with a Tag that a comparison with .Tag asserts.
func Compared() error {
	return errors.New("Compared", "compared")
}

// Keyed fails with a Tag that a table's wantTag asserts.
func Keyed() error {
	return errors.New("Keyed", "keyed")
}

// Sentinel fails with a Tag that errors.Is with a struct literal asserts.
func Sentinel() error {
	return errors.New("Sentinel", "sentinel")
}

// Untested fails with a Tag that nothing asserts.
func Untested() error {
	return errors.New("Untested", "untested")
}

// Excepted fails with a Tag that the exceptions file lists.
func Excepted() error {
	return errors.New("Excepted", "excepted")
}

// ExceptedButTested fails with a Tag that is both listed and asserted.
func ExceptedButTested() error {
	return errors.New("ExceptedButTested", "both")
}

// Closure fails inside a function literal.
func Closure() error {
	f := func() error {
		return errors.New("InClosure", "closure")
	}
	return f()
}

// Dynamic fails with Tags that are not literals or Tag constants, which are problems.
func Dynamic(tag string) error {
	if tag == "" {
		return errors.New(notATag, "a constant without the Tag prefix")
	}
	return errors.New(tag, "a variable")
}

// Twice fails with a Tag that Literal already uses, which is a problem.
func Twice() error {
	return errors.New("Literal", "again")
}
