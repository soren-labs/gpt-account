//go:build !windows

package gpa

import "os"

func prepareTerminal(out *os.File) (func(), error) { return func() {}, nil }
