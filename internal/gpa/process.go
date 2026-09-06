package gpa

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ProcState string

const (
	ProcNone    ProcState = "none"
	ProcIdle    ProcState = "idle"
	ProcBusy    ProcState = "busy"
	ProcUnknown ProcState = "unknown"
	ProcRunning ProcState = "running"
)

type Presence string

const (
	PresenceNone    Presence = "none"
	PresenceRunning Presence = "running"
	PresenceUnknown Presence = "unknown"
)

type ClientState struct {
	Client     Client    `json:"client"`
	Process    ProcState `json:"process"`
	Presence   Presence  `json:"presence,omitempty"`
	ReasonCode string    `json:"reason_code,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Managed    bool      `json:"managed"`
}

func queryFailed() bool {
	return os.Getenv("GPA_PROC_QUERY") == "fail"
}

// ---- Windows process snapshot ------------------------------------------------
//
// Every WSL -> Windows interop call costs seconds (tasklist.exe ~4s, a CIM query
// ~4s, a bare PowerShell start ~1s). The web UI polls status every few seconds
// and each status request used to run two of those calls per client, serially,
// which is what made the page feel frozen. We now take one PowerShell snapshot
// that answers both "is ChatGPT.exe running" and "which codex processes exist",
// cache it briefly, and let read paths return the cached value while a refresh
// runs in the background. Write paths ask for a fresh snapshot.

type winProcSnapshot struct {
	chatgpt    bool
	codexLines string
	err        error
	at         time.Time
}

var (
	winProcMu         sync.Mutex
	winProcCache      *winProcSnapshot
	winProcRefreshing bool
	winProcFreshFor   = 3 * time.Second  // serve without refreshing
	winProcStaleFor   = 30 * time.Second // serve stale while refreshing in background
	winProcTimeout    = 20 * time.Second
)

const winProcScript = `$ErrorActionPreference='SilentlyContinue'; ` +
	`$app=@(Get-Process -Name ChatGPT); 'CHATGPT=' + $app.Count; ` +
	`Get-Process -Name codex* | ForEach-Object { 'CODEX=' + $_.ProcessName + ' ' + $_.Path }`

func queryWinProcs() winProcSnapshot {
	out, err := cmdOutputTimeout(winProcTimeout, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", winProcScript)
	snap := winProcSnapshot{at: time.Now(), err: err}
	if err != nil {
		return snap
	}
	var codex []string
	sawHeader := false
	for _, line := range strings.Split(strings.ReplaceAll(out, "\x00", ""), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "CHATGPT="):
			sawHeader = true
			n, _ := strconv.Atoi(strings.TrimPrefix(line, "CHATGPT="))
			snap.chatgpt = n > 0
		case strings.HasPrefix(line, "CODEX="):
			codex = append(codex, strings.TrimPrefix(line, "CODEX="))
		}
	}
	if !sawHeader {
		snap.err = fmt.Errorf("unexpected process query output")
	}
	snap.codexLines = strings.Join(codex, "\n")
	return snap
}

// winProcs returns the Windows process snapshot. fresh forces a synchronous
// re-query; otherwise a recent snapshot is returned and, if it is getting old,
// refreshed in the background so the caller never waits.
func winProcs(fresh bool) winProcSnapshot {
	winProcMu.Lock()
	cached := winProcCache
	if !fresh && cached != nil && time.Since(cached.at) < winProcStaleFor {
		if time.Since(cached.at) >= winProcFreshFor && !winProcRefreshing {
			winProcRefreshing = true
			go func() {
				snap := queryWinProcs()
				winProcMu.Lock()
				winProcCache = &snap
				winProcRefreshing = false
				winProcMu.Unlock()
			}()
		}
		winProcMu.Unlock()
		return *cached
	}
	winProcMu.Unlock()
	snap := queryWinProcs()
	winProcMu.Lock()
	winProcCache = &snap
	winProcMu.Unlock()
	return snap
}

func invalidateWinProcs() {
	winProcMu.Lock()
	winProcCache = nil
	winProcMu.Unlock()
}

func cmdOutputTimeout(d time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if !onWindows() {
		if exists("/mnt/c/Windows/System32") {
			cmd.Dir = "/mnt/c/Windows/System32"
		} else {
			cmd.Dir = os.TempDir()
		}
	}
	out, err := cmd.Output()
	text := strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
	if ctx.Err() != nil {
		return text, fmt.Errorf("%s timed out after %s", name, d)
	}
	return text, err
}

// ---- ChatGPT App --------------------------------------------------------------

func chatgptRunning() bool {
	running, err := queryChatGPTFresh()
	return err == nil && running
}

func queryChatGPT() (bool, error)      { return queryChatGPTMode(false) }
func queryChatGPTFresh() (bool, error) { return queryChatGPTMode(true) }

func queryChatGPTMode(fresh bool) (bool, error) {
	if os.Getenv("GPA_FAKE_APP") == "running" {
		return true, nil
	}
	if os.Getenv("GPA_CHATGPT") == "off" {
		return false, nil
	}
	if queryFailed() {
		return false, fmt.Errorf("process query disabled")
	}
	snap := winProcs(fresh)
	if snap.err != nil {
		return false, snap.err
	}
	return snap.chatgpt, nil
}

func stopChatGPT(force bool) {
	if os.Getenv("GPA_CHATGPT") == "off" {
		return
	}
	args := []string{"/IM", "ChatGPT.exe"}
	if force {
		args = append(args, "/F")
	}
	_ = runSilent("taskkill.exe", args...)
	invalidateWinProcs()
	if force {
		return
	}
	deadline := time.Now().Add(stopWait())
	for chatgptRunning() && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
}

func startChatGPT() bool {
	if os.Getenv("GPA_CHATGPT") == "off" {
		return true
	}
	aumid := os.Getenv("GPA_CHATGPT_AUMID")
	if aumid == "" {
		aumid = queryStartAUMID()
	}
	if aumid == "" {
		aumid = "OpenAI.Codex_2p2nqsd0c76g0!App"
	}
	_ = runSilent("powershell.exe", "-NoProfile", "-Command", "Start-Process 'shell:AppsFolder\\"+aumid+"'")
	invalidateWinProcs()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if chatgptRunning() {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return chatgptRunning()
}

var (
	aumidOnce  sync.Once
	aumidValue string
)

// queryStartAUMID resolves the Start-menu AppID once per process; Get-StartApps
// costs ~2s and the answer does not change while the manager runs.
func queryStartAUMID() string {
	aumidOnce.Do(func() {
		out, _ := cmdOutputTimeout(winProcTimeout, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			"Get-StartApps | Where-Object Name -Match 'ChatGPT|Codex' | Select-Object -ExpandProperty AppID")
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "OpenAI.Codex") || strings.Contains(line, "ChatGPT") {
				aumidValue = line
				return
			}
		}
	})
	return aumidValue
}

func stopWait() time.Duration {
	raw := os.Getenv("GPA_STOP_WAIT")
	if raw == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(raw + "s")
	if err != nil {
		return 10 * time.Second
	}
	return d
}

func runSilent(name string, args ...string) error {
	return runCmd(name, args...)
}

func parentLooksLikeApp() bool {
	switch strings.ToLower(os.Getenv("GPA_CALLER")) {
	case "app":
		return true
	case "cli", "human":
		return false
	}
	name := parentProcessName()
	l := strings.ToLower(name)
	return strings.Contains(l, "chatgpt") || strings.Contains(l, "openai.codex")
}

func parentProcessName() string {
	if onWindows() {
		return cmdOutput("powershell.exe", "-NoProfile", "-Command",
			"(Get-CimInstance Win32_Process -Filter \"ProcessId=$PID\").ParentProcessId | ForEach-Object { (Get-CimInstance Win32_Process -Filter \"ProcessId=$_\").Name }")
	}
	// Linux / WSL: walk /proc
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return ""
	}
	ppid := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			ppid = strings.TrimSpace(strings.TrimPrefix(line, "PPid:"))
			break
		}
	}
	if ppid == "" || ppid == "0" {
		return ""
	}
	comm, _ := os.ReadFile("/proc/" + ppid + "/comm")
	return strings.TrimSpace(string(comm))
}

func clientSide(c Client) string {
	if c.Kind == "app" || c.ID == "win-cli" {
		return "windows"
	}
	if strings.HasPrefix(c.ID, "wsl:") {
		return c.ID
	}
	if c.Distro != "" {
		return "wsl:" + c.Distro
	}
	if c.ID == "linux" {
		return "linux"
	}
	return currentSide()
}

// ---- Codex CLI ----------------------------------------------------------------

func unmanagedCodexOn(c Client) bool {
	running, err := queryUnmanagedCodexMode(c, true)
	return err == nil && running
}

func queryUnmanagedCodex(c Client) (bool, error) { return queryUnmanagedCodexMode(c, false) }

func queryUnmanagedCodexMode(c Client, fresh bool) (bool, error) {
	fake := os.Getenv("GPA_FAKE_CLI")
	if fake == "running" || fake == "all" {
		return true, nil
	}
	side := clientSide(c)
	if fake == "win" && side == "windows" {
		return true, nil
	}
	if strings.HasPrefix(fake, "wsl") && strings.HasPrefix(side, "wsl:") {
		return true, nil
	}
	if os.Getenv("GPA_CHATGPT") == "off" || os.Getenv("GPA_IGNORE_CLI") == "1" {
		return false, nil
	}
	if queryFailed() {
		return false, fmt.Errorf("process query disabled")
	}
	home := c.CodexHome()
	switch {
	case side == "windows":
		return windowsCodexRunning(home, fresh)
	case strings.HasPrefix(side, "wsl:"):
		return wslCodexRunning(strings.TrimPrefix(side, "wsl:"), home)
	default:
		out, err := localProcessArgs()
		if err != nil {
			return false, err
		}
		return linuxCodexLines(out, home), nil
	}
}

func windowsCodexRunning(home string, fresh bool) (bool, error) {
	snap := winProcs(fresh)
	if snap.err != nil {
		return false, snap.err
	}
	return linuxCodexLines(snap.codexLines, home), nil
}

// localProcessArgs lists the argv of every process on this POSIX host without
// going through a login shell.
func localProcessArgs() (string, error) {
	return cmdOutputTimeout(10*time.Second, "ps", "-eo", "args")
}

func wslCodexRunning(distro, home string) (bool, error) {
	// Inside WSL the manager already runs in the distro it is asked about;
	// a local ps is ~10ms where a wsl.exe round trip is a couple hundred.
	if distro == "" || (!onWindows() && distro == currentWSLDistro()) {
		out, err := localProcessArgs()
		if err != nil {
			return false, err
		}
		return linuxCodexLines(out, home), nil
	}
	posixHome := home
	if onWindows() {
		posixHome = toWSLPath(home)
		if strings.HasPrefix(home, `\\`) {
			posixHome = home
		}
	}
	out, err := cmdOutputTimeout(winProcTimeout, "wsl.exe", "-d", distro, "-e", "ps", "-eo", "args")
	if err != nil {
		return false, err
	}
	return linuxCodexLines(out, posixHome), nil
}

func linuxCodexLines(out, home string) bool {
	if strings.TrimSpace(out) == "" {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		l := strings.ToLower(line)
		if strings.Contains(l, "gpa") {
			continue
		}
		if !strings.Contains(l, "codex") {
			continue
		}
		if home != "" && strings.Contains(line, home) {
			return true
		}
		return true
	}
	return false
}

// ---- Client state -------------------------------------------------------------

func appState(fresh bool) ClientState {
	if os.Getenv("GPA_CHATGPT") == "off" {
		return ClientState{Process: ProcNone, Presence: PresenceNone, Detail: "GPA_CHATGPT=off"}
	}
	running, err := queryChatGPTMode(fresh)
	if err != nil {
		return ClientState{Process: ProcUnknown, Presence: PresenceUnknown, ReasonCode: "QUERY_FAILED", Detail: "无法查询 ChatGPT.exe: " + err.Error()}
	}
	if running {
		return ClientState{Process: ProcUnknown, Presence: PresenceRunning, ReasonCode: "APP_RUNNING", Detail: "ChatGPT.exe 在运行，无法确认是否空闲"}
	}
	return ClientState{Process: ProcNone, Presence: PresenceNone, Detail: "未运行"}
}

func cliState(c Client, fresh bool) ClientState {
	running, err := queryUnmanagedCodexMode(c, fresh)
	if err != nil {
		return ClientState{Process: ProcUnknown, Presence: PresenceUnknown, ReasonCode: "QUERY_FAILED", Detail: "无法查询 " + clientSide(c) + " 进程: " + err.Error()}
	}
	if running {
		return ClientState{Process: ProcUnknown, Presence: PresenceRunning, ReasonCode: "CLI_RUNNING", Detail: "检测到 " + clientSide(c) + " 上有未托管的 Codex 进程"}
	}
	return ClientState{Process: ProcNone, Presence: PresenceNone}
}

// InspectClient answers from a recent snapshot; use it on read paths such as
// status polling where a few seconds of staleness is acceptable.
func InspectClient(c Client) ClientState { return inspectClient(c, false) }

// InspectClientFresh always re-queries; use it before deciding to write.
func InspectClientFresh(c Client) ClientState { return inspectClient(c, true) }

func inspectClient(c Client, fresh bool) ClientState {
	st := ClientState{Client: c}
	if c.Kind == "app" {
		app := appState(fresh)
		st.Process = app.Process
		st.Presence = app.Presence
		st.ReasonCode = app.ReasonCode
		st.Detail = app.Detail
		return st
	}
	cli := cliState(c, fresh)
	st.Process = cli.Process
	st.Presence = cli.Presence
	st.ReasonCode = cli.ReasonCode
	st.Detail = cli.Detail
	return st
}
