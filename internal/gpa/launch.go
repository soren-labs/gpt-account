package gpa

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func openNewWindow() error {
	self, err := os.Executable()
	if err != nil {
		return fail("cannot find gpa executable")
	}
	if onWindows() || inWSL() {
		win := windowsBinPath()
		if exists(toWSLPath(win)) || exists(win) {
			self = win
		}
		target := toWindowsPath(self)
		ps := fmt.Sprintf("Start-Process -FilePath %q -WorkingDirectory $env:USERPROFILE", target)
		if err := runSilent("powershell.exe", "-NoProfile", "-Command", ps); err != nil {
			return fail("could not open new window: " + err.Error())
		}
		return nil
	}
	cmd := exec.Command("x-terminal-emulator", "-e", self)
	if err := cmd.Start(); err != nil {
		return fail("could not open a terminal: " + err.Error())
	}
	return nil
}

func windowsBinPath() string {
	if app := windowsLocalAppData(); app != "" {
		return winJoin(app, "gpa", "bin", "gpa.exe")
	}
	return ""
}

func linuxBinPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "gpa")
}

func cmdCodex(store *Store, args []string, stdout, stderr io.Writer) int {
	account := ""
	rest := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--account" && i+1 < len(args) {
			i++
			account = args[i]
			continue
		}
		if strings.HasPrefix(args[i], "--account=") {
			account = strings.TrimPrefix(args[i], "--account=")
			continue
		}
		rest = append(rest, args[i])
	}
	clients := store.LoadClients()
	var cli Client
	for _, c := range clients {
		if c.Kind == "cli" && (!onWindows() && strings.HasPrefix(c.ID, "wsl:") || onWindows() && c.ID == "win-cli") {
			cli = c
			break
		}
	}
	if cli.ID == "" {
		for _, c := range clients {
			if c.Kind == "cli" {
				cli = c
				break
			}
		}
	}
	if cli.ID == "" {
		fmt.Fprintln(stderr, "gpa: no CLI client registered")
		return 1
	}
	if account != "" {
		lock, err := AcquireLock(store.LockPath())
		if err != nil {
			fmt.Fprintln(stderr, "gpa:", err)
			return 1
		}
		res := UseAccount(store, account, cli.ID, false, false, false, false)
		lock.Release()
		if res.Status != "completed" {
			fmt.Fprintln(stderr, "gpa: cannot start managed codex:", res.Error, res.Next)
			return res.ExitCode()
		}
	}
	if unmanagedCodexOn(cli) {
		fmt.Fprintln(stderr, "gpa: unmanaged codex already using", cli.CodexHome())
		return int(StatusBlocked)
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		fmt.Fprintln(stderr, "gpa: codex CLI not on PATH")
		return 1
	}
	cmd := exec.Command(bin, rest...)
	cmd.Env = append(os.Environ(), "CODEX_HOME="+cli.CodexHome())
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	fmt.Fprintf(stderr, "gpa: managed codex  CODEX_HOME=%s\n", cli.CodexHome())
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(stderr, "gpa:", err)
		return 1
	}
	// adopt refreshed tokens after exit
	lock, err := AcquireLock(store.LockPath())
	if err == nil {
		AdoptLives(store, store.LoadClients())
		lock.Release()
	}
	return 0
}
