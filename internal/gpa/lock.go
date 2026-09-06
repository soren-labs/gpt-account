package gpa

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// mkdir lock works on both NTFS/drvfs and Linux. flock on /mnt/c does not.
type StoreLock struct {
	dir string
}

type lockOwner struct {
	Side string `json:"side"`
	PID  int    `json:"pid"`
	At   string `json:"at"`
}

func currentSide() string {
	if onWindows() {
		return "windows"
	}
	if inWSL() {
		if d := currentWSLDistro(); d != "" {
			return "wsl:" + d
		}
		return "wsl"
	}
	return "linux"
}

func AcquireLock(path string) (*StoreLock, error) {
	dir := path + ".d"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fail("cannot lock " + path + ": " + err.Error())
	}
	for i := 0; i < 2; i++ {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			writeLockOwner(dir)
			return &StoreLock{dir: dir}, nil
		}
		if os.IsExist(err) && staleLock(dir) {
			_ = os.RemoveAll(dir)
			continue
		}
		if os.IsExist(err) {
			return nil, blocked("another gpa command holds " + path)
		}
		return nil, fail("cannot lock " + path + ": " + err.Error())
	}
	return nil, blocked("another gpa command holds " + path)
}

func writeLockOwner(dir string) {
	owner := lockOwner{
		Side: currentSide(),
		PID:  os.Getpid(),
		At:   time.Now().UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(owner)
	_ = os.WriteFile(filepath.Join(dir, "owner.json"), raw, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(owner.PID)), 0o600)
}

func readLockOwner(dir string) lockOwner {
	raw, err := os.ReadFile(filepath.Join(dir, "owner.json"))
	if err == nil {
		var o lockOwner
		if json.Unmarshal(raw, &o) == nil && o.PID > 0 {
			return o
		}
	}
	pidRaw, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		return lockOwner{}
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidRaw)))
	if err != nil || pid <= 0 {
		return lockOwner{}
	}
	// Legacy lock: unknown side. Do not assume it lives in this PID namespace.
	return lockOwner{PID: pid}
}

func staleLock(dir string) bool {
	info, statErr := os.Stat(dir)
	age := time.Duration(0)
	if statErr == nil {
		age = time.Since(info.ModTime())
	}
	owner := readLockOwner(dir)
	if owner.PID <= 0 {
		return statErr == nil && age > 30*time.Minute
	}
	if owner.Side == "" || owner.Side == currentSide() {
		if owner.Side == currentSide() {
			return !localPIDAlive(owner.PID)
		}
		// Unknown side: only expire by age. Never probe this PID locally.
		return age > 30*time.Minute
	}
	alive, ok := remotePIDAlive(owner.Side, owner.PID)
	if ok {
		return !alive
	}
	return age > 30*time.Minute
}

func localPIDAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return signalAlive(proc) == nil
}

func remotePIDAlive(side string, pid int) (alive bool, ok bool) {
	if os.Getenv("GPA_LOCK_REMOTE") == "dead" {
		return false, true
	}
	if os.Getenv("GPA_LOCK_REMOTE") == "alive" {
		return true, true
	}
	if strings.HasPrefix(side, "wsl:") {
		distro := strings.TrimPrefix(side, "wsl:")
		out := cmdOutput("wsl.exe", "-d", distro, "-e", "bash", "-lc", "kill -0 "+strconv.Itoa(pid)+" 2>/dev/null && echo ALIVE || echo DEAD")
		if strings.Contains(out, "ALIVE") {
			return true, true
		}
		if strings.Contains(out, "DEAD") {
			return false, true
		}
		return false, false
	}
	if side == "windows" {
		blob := cmdOutput("tasklist.exe", "/FI", "PID eq "+strconv.Itoa(pid))
		if blob == "" {
			return false, false
		}
		id := strconv.Itoa(pid)
		if strings.Contains(blob, id) && !strings.Contains(strings.ToLower(blob), "no tasks") && !strings.Contains(blob, "没有运行") {
			return true, true
		}
		if strings.Contains(strings.ToLower(blob), "no tasks") || strings.Contains(blob, "没有运行") {
			return false, true
		}
		return false, false
	}
	return false, false
}

func (l *StoreLock) Release() {
	if l == nil || l.dir == "" {
		return
	}
	_ = os.RemoveAll(l.dir)
	l.dir = ""
}
