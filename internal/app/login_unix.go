//go:build !windows

package app

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

func newLoginCommand(ctx context.Context, args []string) (*exec.Cmd, error) {
	p, e := exec.LookPath("codex")
	if e != nil {
		return nil, fmt.Errorf("未找到 Codex CLI，请安装后重试")
	}
	return exec.CommandContext(ctx, p, args...), nil
}
func configureLoginProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
