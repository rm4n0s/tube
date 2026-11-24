package b

import "github.com/rm4n0s/errors"

// Open fails on Windows with the Tag of the Linux version, which is allowed.
func Open() error {
	return errors.New("OpenFailed", "windows")
}

// Tell fails with the Tag of another function, which is a problem.
func Tell() error {
	return errors.New("SeekFailed", "windows")
}
