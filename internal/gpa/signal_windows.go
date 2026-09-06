//go:build windows

package gpa

import (
	"golang.org/x/sys/windows"
	"os"
)

func signalAlive(p *os.Process) error {
	if p == nil {
		return os.ErrProcessDone
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return err
	}
	if code != 259 {
		return os.ErrProcessDone
	}
	return nil
}
