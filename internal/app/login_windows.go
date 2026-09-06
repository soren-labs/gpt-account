//go:build windows

package app

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func newLoginCommand(ctx context.Context, args []string) (*exec.Cmd, error) {
	p, e := exec.LookPath("codex")
	if e != nil {
		return nil, fmt.Errorf("未找到 Codex CLI，请安装后重试")
	}
	if strings.HasSuffix(strings.ToLower(p), ".cmd") || strings.HasSuffix(strings.ToLower(p), ".bat") {
		// Only the trusted resolved launcher and fixed flags enter cmd.exe; account
		// names, paths and credentials are not interpolated into the command string.
		return exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", `""`+p+`" login --device-auth -c cli_auth_credentials_store=\"file\""`), nil
	}
	return exec.CommandContext(ctx, p, args...), nil
}
func configureLoginProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Cancel = func() error {
		return exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
	}
}
