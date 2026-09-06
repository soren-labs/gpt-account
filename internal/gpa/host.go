package gpa

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var winDrive = regexp.MustCompile(`(?i)^([A-Za-z]):[/\\](.*)$`)

func onWindows() bool { return runtime.GOOS == "windows" }

func inWSL() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	raw, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		raw, err = os.ReadFile("/proc/version")
	}
	if err != nil {
		return false
	}
	s := strings.ToLower(string(raw))
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}

func isWinAbs(p string) bool {
	return winDrive.MatchString(strings.ReplaceAll(p, "\\", "/"))
}

func lastWinPath(blob string) string {
	blob = strings.ReplaceAll(blob, "\x00", "")
	blob = strings.ReplaceAll(blob, "\r", "")
	lines := strings.Split(blob, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		line = strings.Trim(line, `"'`)
		if isWinAbs(line) && !strings.Contains(line, "\n") {
			return line
		}
	}
	return ""
}

func winJoin(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	first := strings.ReplaceAll(parts[0], "/", `\`)
	out := []string{strings.TrimRight(first, `\`)}
	for _, p := range parts[1:] {
		p = strings.ReplaceAll(p, "/", `\`)
		p = strings.Trim(p, `\`)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, `\`)
}

func winDir(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	i := strings.LastIndex(p, `\`)
	if i <= 0 {
		return p
	}
	return p[:i]
}

func cmdOutput(name string, args ...string) string {
	out, _ := cmdOutputErr(name, args...)
	return out
}

func cmdOutputErr(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if !onWindows() {
		if exists("/mnt/c/Windows/System32") {
			cmd.Dir = "/mnt/c/Windows/System32"
		} else {
			cmd.Dir = os.TempDir()
		}
	}
	out, err := cmd.Output()
	text := strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
	if err != nil {
		return text, err
	}
	return text, nil
}

func windowsUserProfile() string {
	if onWindows() {
		if p := os.Getenv("USERPROFILE"); p != "" {
			return p
		}
	}
	if p := os.Getenv("USERPROFILE"); isWinAbs(p) {
		return p
	}
	if p := lastWinPath(cmdOutput("cmd.exe", "/c", "echo %USERPROFILE%")); p != "" {
		return p
	}
	user := os.Getenv("USERNAME")
	if user == "" {
		user = os.Getenv("USER")
	}
	if user != "" && !skipWinUser(user) {
		return `C:\Users\` + user
	}
	return ""
}

func windowsLocalAppData() string {
	if onWindows() {
		if p := os.Getenv("LOCALAPPDATA"); p != "" {
			return p
		}
	}
	if p := os.Getenv("LOCALAPPDATA"); isWinAbs(p) {
		return p
	}
	if p := lastWinPath(cmdOutput("cmd.exe", "/c", "echo %LOCALAPPDATA%")); p != "" {
		return p
	}
	if home := windowsUserProfile(); home != "" {
		return winJoin(home, "AppData", "Local")
	}
	return ""
}

func skipWinUser(name string) bool {
	switch strings.ToLower(name) {
	case "public", "default", "default user", "all users", "codexsandboxoffline":
		return true
	}
	return false
}

func toWSLPath(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/mnt/") || strings.HasPrefix(raw, "/") {
		return filepath.Clean(raw)
	}
	text := strings.ReplaceAll(raw, "\\", "/")
	m := winDrive.FindStringSubmatch(text)
	if m == nil {
		return raw
	}
	return "/mnt/" + strings.ToLower(m[1]) + "/" + m[2]
}

func toWindowsPath(raw string) string {
	if raw == "" {
		return ""
	}
	text := strings.ReplaceAll(raw, "\\", "/")
	if m := winDrive.FindStringSubmatch(text); m != nil {
		return strings.ToUpper(m[1]) + `:\` + strings.ReplaceAll(m[2], "/", `\`)
	}
	if strings.HasPrefix(text, "/mnt/") && len(text) > 6 && text[6] == '/' {
		return strings.ToUpper(string(text[5])) + `:\` + strings.ReplaceAll(text[7:], "/", `\`)
	}
	return raw
}

func nativePath(raw string) string {
	if onWindows() {
		return toWindowsPath(raw)
	}
	if strings.HasPrefix(raw, `\\`) {
		return raw
	}
	return toWSLPath(raw)
}

func currentWSLDistro() string {
	if name := os.Getenv("WSL_DISTRO_NAME"); name != "" {
		return name
	}
	if !inWSL() {
		return ""
	}
	return "Ubuntu"
}

func listWSLDistros() []string {
	out := cmdOutput("wsl.exe", "-l", "-q")
	if out == "" {
		return nil
	}
	// wsl -l often emits UTF-16. cmdOutput may already be mangled; try again with iconv-less cleanup.
	cleaned := strings.ReplaceAll(out, "\x00", "")
	var names []string
	for _, line := range strings.Split(cleaned, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Windows Subsystem") {
			continue
		}
		names = append(names, line)
	}
	return names
}

func wslHome(distro string) string {
	if distro == "" || distro == currentWSLDistro() {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
	}
	out := cmdOutput("wsl.exe", "-d", distro, "-e", "bash", "-lc", "printf %s \"$HOME\"")
	return strings.TrimSpace(out)
}

func wslUNC(distro, posix string) string {
	p := strings.TrimPrefix(posix, "/")
	return `\\wsl.localhost\` + distro + `\` + strings.ReplaceAll(p, "/", `\`)
}
