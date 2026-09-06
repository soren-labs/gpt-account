//go:build windows

package gpa

import "os"

func signalAlive(p *os.Process) error {
	// Windows FindProcess succeeds for dead PIDs; treat missing OpenProcess as stale in switcher.
	if p == nil {
		return os.ErrProcessDone
	}
	return nil
}
