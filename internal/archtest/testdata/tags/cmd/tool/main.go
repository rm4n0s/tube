// Command tool creates an error that nothing tests.
package main

import "github.com/rm4n0s/errors"

func main() {
	panic(errors.New("ToolFailed", "tool"))
}
