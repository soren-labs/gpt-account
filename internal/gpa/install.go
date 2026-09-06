package gpa

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func cmdInstall(store *Store, args []string, asJSON bool, stdout, stderr io.Writer) int {
	winBin := ""
	linuxBin := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--windows-bin" && i+1 < len(args):
			i++
			winBin = args[i]
		case args[i] == "--linux-bin" && i+1 < len(args):
			i++
			linuxBin = args[i]
		case strings.HasPrefix(args[i], "--windows-bin="):
			winBin = strings.TrimPrefix(args[i], "--windows-bin=")
		case strings.HasPrefix(args[i], "--linux-bin="):
			linuxBin = strings.TrimPrefix(args[i], "--linux-bin=")
		}
	}
	self, _ := os.Executable()
	if linuxBin == "" && !onWindows() {
		linuxBin = self
	}
	if winBin == "" && onWindows() {
		winBin = self
	}

	steps := []string{}
	destStore := defaultSharedStore()
	store.Root = destStore
	store.Cfg.Store = destStore
	if err := store.Ensure(); err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	clients := DiscoverClients(store.Cfg)
	if err := store.SaveClients(clients); err != nil {
		return emitErr(asJSON, stdout, stderr, err)
	}
	steps = append(steps, "store "+destStore)

	if linuxBin != "" && exists(linuxBin) {
		dest := linuxBinPath()
		if sameInstallFile(linuxBin, dest) {
			steps = append(steps, "linux "+dest+" (already in place)")
		} else {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return emitErr(asJSON, stdout, stderr, err)
			}
			data, err := os.ReadFile(linuxBin)
			if err != nil {
				return emitErr(asJSON, stdout, stderr, err)
			}
			if err := atomicWrite(dest, data, 0o755); err != nil {
				return emitErr(asJSON, stdout, stderr, err)
			}
			steps = append(steps, "linux "+dest)
		}
	}

	if winBin != "" && exists(winBin) {
		destWin := windowsBinPath()
		dest := destWin
		if !onWindows() {
			dest = toWSLPath(destWin)
		}
		if sameInstallFile(winBin, dest) {
			steps = append(steps, "windows "+dest+" (already in place)")
		} else {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return emitErr(asJSON, stdout, stderr, err)
			}
			data, err := os.ReadFile(winBin)
			if err != nil {
				return emitErr(asJSON, stdout, stderr, err)
			}
			if err := atomicWrite(dest, data, 0o755); err != nil {
				return emitErr(asJSON, stdout, stderr, err)
			}
			steps = append(steps, "windows "+dest)
		}
		if err := addWindowsUserPath(winDir(destWin)); err != nil {
			steps = append(steps, "windows PATH warning: "+err.Error())
		} else {
			steps = append(steps, "windows PATH "+winDir(destWin))
		}
		if err := createStartMenu(destWin); err != nil {
			steps = append(steps, "start menu warning: "+err.Error())
		} else {
			steps = append(steps, "start menu GPA")
		}
	}

	imported := []string{}
	if len(store.Names()) == 0 {
		for _, src := range legacyStores() {
			if src == destStore {
				continue
			}
			res, err := MigrateFrom(store, src, false)
			if err == nil {
				imported = append(imported, asStringSlice(res["imported"])...)
				steps = append(steps, "imported from "+src)
				break
			}
		}
	}

	out := map[string]any{
		"status":   "completed",
		"store":    destStore,
		"steps":    steps,
		"clients":  clients,
		"imported": imported,
		"next":     "重新打开 PowerShell 后输入 gpa",
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		fmt.Fprintln(stdout, "GPA installed")
		for _, s := range steps {
			fmt.Fprintln(stdout, " ", s)
		}
		if len(imported) > 0 {
			fmt.Fprintln(stdout, "imported", strings.Join(imported, ", "))
		}
		fmt.Fprintln(stdout, "重新打开 PowerShell 后输入 gpa")
		fmt.Fprintln(stdout, "WSL 里同样输入 gpa，两边看到同一批账号")
	}
	return 0
}

func sameInstallFile(a, b string) bool {
	na := normalizeInstallPath(a)
	nb := normalizeInstallPath(b)
	return na != "" && na == nb
}

func normalizeInstallPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if isWinAbs(p) || strings.HasPrefix(strings.ReplaceAll(p, `\`, "/"), "/mnt/") {
		win := toWindowsPath(p)
		return strings.ToLower(strings.ReplaceAll(win, "/", `\`))
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return strings.ToLower(strings.ReplaceAll(toWindowsPath(p), "/", `\`))
}

func addWindowsUserPath(dir string) error {
	dir = toWindowsPath(dir)
	script := `
$dir = '` + dir + `'
$cur = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($null -eq $cur) { $cur = '' }
$parts = $cur -split ';' | Where-Object { $_ -and $_.Trim() -ne '' }
if ($parts -contains $dir) { 'already' ; exit 0 }
$new = if ($cur -eq '') { $dir } else { $cur.TrimEnd(';') + ';' + $dir }
[Environment]::SetEnvironmentVariable('Path', $new, 'User')
'updated'
`
	out := cmdOutput("powershell.exe", "-NoProfile", "-Command", script)
	if strings.Contains(out, "updated") || strings.Contains(out, "already") {
		return nil
	}
	if out == "" {
		return fail("could not update user PATH")
	}
	return nil
}

func createStartMenu(exe string) error {
	exe = toWindowsPath(exe)
	script := `
$Wsh = New-Object -ComObject WScript.Shell
$dir = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
$sc = $Wsh.CreateShortcut((Join-Path $dir 'GPA.lnk'))
$sc.TargetPath = '` + exe + `'
$sc.WorkingDirectory = $env:USERPROFILE
$sc.Description = 'Switch ChatGPT / Codex accounts'
$sc.Save()
'ok'
`
	out := cmdOutput("powershell.exe", "-NoProfile", "-Command", script)
	if !strings.Contains(out, "ok") {
		return fail("start menu: " + out)
	}
	return nil
}
