//go:build windows

package gpa

import (
	"golang.org/x/sys/windows"
	"os"
)

// Enable ANSI rendering on the output handle as well as raw input.
func prepareTerminal(out *os.File) (func(), error) {
	h := windows.Handle(out.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(h, mode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return nil, err
	}
	return func() { _ = windows.SetConsoleMode(h, mode) }, nil
}
