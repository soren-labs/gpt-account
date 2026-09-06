//go:build !windows

package gpa

import (
	"os"
	"syscall"
)

func signalAlive(p *os.Process) error {
	return p.Signal(syscall.Signal(0))
}
