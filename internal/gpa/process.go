package gpa

import (
	"fmt"
	"os"
	"strings"
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
	Client       Client    `json:"client"`
	Process      ProcState `json:"process"`
	Presence     Presence  `json:"presence,omitempty"`
	ReasonCode   string    `json:"reason_code,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	Managed      bool      `json:"managed"`
}

func queryFailed() bool {
	return os.Getenv("GPA_PROC_QUERY") == "fail"
}

func chatgptRunning() bool {
	running, err := queryChatGPT()
	return err == nil && running
}

func queryChatGPT() (bool, error) {
	if os.Getenv("GPA_FAKE_APP") == "running" {
		return true, nil
	}
	if os.Getenv("GPA_CHATGPT") == "off" {
		return false, nil
	}
	if queryFailed() {
		return false, fmt.Errorf("process query disabled")
	}
	blob, err := cmdOutputErr("tasklist.exe", "/FI", "IMAGENAME eq ChatGPT.exe")
	if err != nil {
		return false, err
	}
	return strings.Contains(blob, "ChatGPT.exe"), nil
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
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if chatgptRunning() {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return chatgptRunning()
}

func queryStartAUMID() string {
	out := cmdOutput("powershell.exe", "-NoProfile", "-Command",
		"Get-StartApps | Where-Object Name -Match 'ChatGPT|Codex' | Select-Object -ExpandProperty AppID")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "OpenAI.Codex") || strings.Contains(line, "ChatGPT") {
			return line
		}
	}
	return ""
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

func unmanagedCodexOn(c Client) bool {
	running, err := queryUnmanagedCodex(c)
	return err == nil && running
}

func queryUnmanagedCodex(c Client) (bool, error) {
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
		return windowsCodexRunning(home)
	case strings.HasPrefix(side, "wsl:"):
		return wslCodexRunning(strings.TrimPrefix(side, "wsl:"), home)
	default:
		out, err := cmdOutputErr("bash", "-lc", "ps -eo args")
		if err != nil {
			return false, err
		}
		return linuxCodexLines(out, home), nil
	}
}

func windowsCodexRunning(home string) (bool, error) {
	out, err := cmdOutputErr("powershell.exe", "-NoProfile", "-Command",
		"Get-CimInstance Win32_Process | Where-Object { $_.Name -match 'codex' } | Select-Object -ExpandProperty CommandLine")
	if err != nil {
		return false, err
	}
	return linuxCodexLines(out, home), nil
}

func wslCodexRunning(distro, home string) (bool, error) {
	if distro == "" {
		out, err := cmdOutputErr("bash", "-lc", "ps -eo args")
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
	out, err := cmdOutputErr("wsl.exe", "-d", distro, "-e", "bash", "-lc", "ps -eo args")
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

func appState() ClientState {
	if os.Getenv("GPA_CHATGPT") == "off" {
		return ClientState{Process: ProcNone, Presence: PresenceNone, Detail: "GPA_CHATGPT=off"}
	}
	running, err := queryChatGPT()
	if err != nil {
		return ClientState{Process: ProcUnknown, Presence: PresenceUnknown, ReasonCode: "QUERY_FAILED", Detail: "无法查询 ChatGPT.exe: " + err.Error()}
	}
	if running {
		return ClientState{Process: ProcUnknown, Presence: PresenceRunning, ReasonCode: "APP_RUNNING", Detail: "ChatGPT.exe 在运行，无法确认是否空闲"}
	}
	return ClientState{Process: ProcNone, Presence: PresenceNone, Detail: "未运行"}
}

func cliState(c Client) ClientState {
	running, err := queryUnmanagedCodex(c)
	if err != nil {
		return ClientState{Process: ProcUnknown, Presence: PresenceUnknown, ReasonCode: "QUERY_FAILED", Detail: "无法查询 " + clientSide(c) + " 进程: " + err.Error()}
	}
	if running {
		return ClientState{Process: ProcUnknown, Presence: PresenceRunning, ReasonCode: "CLI_RUNNING", Detail: "检测到 " + clientSide(c) + " 上有未托管的 Codex 进程"}
	}
	return ClientState{Process: ProcNone, Presence: PresenceNone}
}

func InspectClient(c Client) ClientState { return inspectClient(c) }

func inspectClient(c Client) ClientState {
	st := ClientState{Client: c}
	if c.Kind == "app" {
		app := appState()
		st.Process = app.Process
		st.Detail = app.Detail
		return st
	}
	cli := cliState(c)
	st.Process = cli.Process
	st.Detail = cli.Detail
	return st
}
