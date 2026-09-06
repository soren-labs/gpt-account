package gpa

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (c Client) needsWSLBridge() bool {
	if c.Distro == "" || c.ID == "linux" || c.Kind == "app" || c.ID == "win-cli" {
		return false
	}
	if onWindows() {
		return false
	}
	return inWSL() && c.Distro != currentWSLDistro()
}

func (c Client) AuthPath() string {
	if c.needsWSLBridge() {
		if c.WinPath != "" {
			return c.WinPath
		}
		return "wsl:" + c.Distro + ":" + c.WSLPath
	}
	if onWindows() {
		if c.WinPath != "" {
			return c.WinPath
		}
		return toWindowsPath(c.WSLPath)
	}
	if c.WSLPath != "" {
		return c.WSLPath
	}
	return toWSLPath(c.WinPath)
}

func (c Client) posixAuth() string {
	if c.WSLPath != "" {
		return c.WSLPath
	}
	return toWSLPath(c.WinPath)
}

func (c Client) CodexHome() string {
	if c.WSLPath != "" {
		return filepath.Dir(c.WSLPath)
	}
	if c.WinPath != "" {
		n := strings.ReplaceAll(c.WinPath, `\`, "/")
		if i := strings.LastIndex(n, "/"); i > 0 {
			return n[:i]
		}
	}
	return filepath.Dir(c.AuthPath())
}

func LoadClientAuth(c Client) map[string]any { return loadClientAuth(c) }

func loadClientAuth(c Client) map[string]any {
	raw, err := clientReadBytes(c)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var auth map[string]any
	if json.Unmarshal(raw, &auth) != nil {
		return nil
	}
	if !IsChatGPTBundle(InspectAuth(auth)) {
		return nil
	}
	return auth
}

func writeClientAuth(c Client, auth map[string]any) error {
	raw, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return clientWriteBytes(c, raw)
}

func clientExists(c Client) bool {
	if c.needsWSLBridge() {
		_, err := clientReadBytes(c)
		return err == nil
	}
	return existsQuiet(c.AuthPath())
}

func clientReadBytes(c Client) ([]byte, error) {
	if c.needsWSLBridge() {
		return wslReadFile(c.Distro, c.posixAuth())
	}
	return os.ReadFile(c.AuthPath())
}

func clientWriteBytes(c Client, data []byte) error {
	if c.needsWSLBridge() {
		return wslWriteFile(c.Distro, c.posixAuth(), data)
	}
	return atomicWrite(c.AuthPath(), data, 0o600)
}

func clientRemove(c Client) error {
	if c.needsWSLBridge() {
		return wslRemoveFile(c.Distro, c.posixAuth())
	}
	return os.Remove(c.AuthPath())
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func wslCmd(distro string, script string) *exec.Cmd {
	cmd := exec.Command("wsl.exe", "-d", distro, "-e", "bash", "-lc", script)
	if exists("/mnt/c/Windows/System32") {
		cmd.Dir = "/mnt/c/Windows/System32"
	}
	return cmd
}

func wslReadFile(distro, posix string) ([]byte, error) {
	cmd := wslCmd(distro, "cat -- "+shellQuote(posix))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func wslWriteFile(distro, posix string, data []byte) error {
	q := shellQuote(posix)
	script := "set -euo pipefail; umask 077; dir=$(dirname -- " + q + "); mkdir -p -- \"$dir\"; tmp=$(mktemp -- \"$dir/.gpa-XXXXXX\"); trap 'rm -f -- \"$tmp\"' EXIT; cat > \"$tmp\"; mv -f -- \"$tmp\" " + q + "; trap - EXIT"
	cmd := wslCmd(distro, script)
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func wslRemoveFile(distro, posix string) error {
	cmd := wslCmd(distro, "rm -f -- "+shellQuote(posix))
	return cmd.Run()
}
