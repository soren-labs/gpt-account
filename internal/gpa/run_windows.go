//go:build windows

package gpa

import (
	"os/exec"
	"syscall"
)

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
