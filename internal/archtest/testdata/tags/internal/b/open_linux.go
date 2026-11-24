package b

import "github.com/rm4n0s/errors"

// Open fails on Linux.
func Open() error {
	return errors.New("OpenFailed", "linux")
}

// Seek fails on Linux, with a Tag that the Windows file uses in another function.
func Seek() error {
	return errors.New("SeekFailed", "linux")
}
