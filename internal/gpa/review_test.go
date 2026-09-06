package gpa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewNewSlotMustNotDowngrade(t *testing.T) {
	s := testEnv(t)
	fresh := FakeAuth("plus@example.com", "user-plus", "plus", "ws-plus", "fresh")
	fresh["last_refresh"] = "2026-09-05T12:00:00Z"
	s.Put("plus", fresh, "login", true)
	AdoptLives(s, s.LoadClients())
	if asString(asMap(storeMust(t, s, "plus").Auth["tokens"])["refresh_token"]) != "fresh" {
		t.Fatal("newly authenticated slot overwritten by older live credentials")
	}
}

func TestReviewEqualTimeConflict(t *testing.T) {
	s := testEnv(t)
	a := FakeAuth("plus@example.com", "user-plus", "plus", "ws-plus", "different")
	writeJSON(filepath.Join(s.Cfg.CodexHome, "auth.json"), a)
	_, conflicts := AdoptLives(s, s.LoadClients())
	if len(conflicts) == 0 {
		t.Fatal("same timestamp and different refresh tokens accepted without conflict")
	}
}

func TestReviewFailedOperationStatus(t *testing.T) {
	s := testEnv(t)
	s.SaveOperation(Operation{ID: "review-op", Account: "missing", Status: "pending"})
	code := Run([]string{"--store", s.Root, "operation", "apply", "review-op", "--json"}, nilReader{}, discard{}, discard{})
	op, _ := s.GetOperation("review-op")
	if code != 0 && op.Status == "applied" {
		t.Fatal("failed operation marked applied")
	}
}

func TestReviewForceCLI(t *testing.T) {
	s := testEnv(t)
	t.Setenv("GPA_FAKE_CLI", "running")
	r := UseAccount(s, "biz1", "cli", true, false, false, false)
	if r.Status == "completed" {
		t.Fatal("running CLI credentials overwritten and reported completed without stopping CLI")
	}
}

func TestReviewMenuRunningApp(t *testing.T) {
	s := testEnv(t)
	t.Setenv("GPA_CHATGPT", "auto")
	t.Setenv("GPA_FAKE_APP", "running")
	t.Setenv("GPA_FAKE_CLI", "running")
	r := UseAccount(s, "biz1", "all", false, false, false, true)
	if r.Status == "blocked" {
		t.Fatal("menu Enter always blocked for running app; no in-menu completion action")
	}
}

func TestLockForeignSideNotStolen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gpa.lock.d")
	os.MkdirAll(dir, 0o700)
	writeJSON(filepath.Join(dir, "owner.json"), map[string]any{
		"side": "windows",
		"pid":  4242,
		"at":   time.Now().UTC().Format(time.RFC3339),
	})
	t.Setenv("GPA_LOCK_REMOTE", "alive")
	if currentSide() == "windows" {
		t.Skip("running on windows")
	}
	if staleLock(dir) {
		t.Fatal("live Windows lock treated as stale from WSL")
	}
}

func TestOtherDistroAuthPathIsNotLocal(t *testing.T) {
	cur := currentWSLDistro()
	if cur == "" {
		cur = "Ubuntu-24.04"
	}
	otherName := "Debian"
	if strings.EqualFold(cur, otherName) {
		otherName = "Ubuntu-24.04"
	}
	local := Client{ID: "wsl:" + cur, Distro: cur, WSLPath: "/home/demo/.codex/auth.json", Storage: "posix-" + cur}
	other := Client{
		ID: "wsl:" + otherName, Distro: otherName, Storage: "posix-" + otherName,
		WSLPath: "/home/demo/.codex/auth.json",
		WinPath: `\\wsl.localhost\` + otherName + `\home\demo\.codex\auth.json`,
	}
	if inWSL() {
		if local.needsWSLBridge() {
			t.Fatal("current distro should use local files")
		}
		if !other.needsWSLBridge() {
			t.Fatal("other distro should use the distro adapter")
		}
	}
	if inWSL() || onWindows() {
		if local.AuthPath() == other.AuthPath() {
			t.Fatalf("collapsed paths: %s", local.AuthPath())
		}
	}
}

func TestQueryFailureIsUnknown(t *testing.T) {
	s := testEnv(t)
	t.Setenv("GPA_CHATGPT", "auto")
	t.Setenv("GPA_IGNORE_CLI", "")
	t.Setenv("GPA_PROC_QUERY", "fail")
	var app Client
	for _, c := range s.LoadClients() {
		if c.Kind == "app" {
			app = c
			break
		}
	}
	if app.ID == "" {
		t.Fatal("no app client")
	}
	st := inspectClient(app)
	if st.Process != ProcUnknown {
		t.Fatalf("query failure became %s", st.Process)
	}
	r := UseAccount(s, "biz1", "app", false, false, false, false)
	if r.Status == "completed" {
		t.Fatal("query failure allowed overwrite")
	}
}

func TestSameInstallFile(t *testing.T) {
	if !sameInstallFile(`C:\Users\zheng\AppData\Local\gpa\bin\gpa.exe`, `/mnt/c/Users/zheng/AppData/Local/gpa/bin/gpa.exe`) {
		t.Fatal("windows and wsl dest should match")
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "gpa")
	os.WriteFile(src, []byte("x"), 0o755)
	if !sameInstallFile(src, src) {
		t.Fatal("identical path")
	}
	if sameInstallFile(src, filepath.Join(tmp, "other")) {
		t.Fatal("different files")
	}
}

func TestDiscoverWindowsAddsWSLWhenNotIsolated(t *testing.T) {
	// Production discovery (no GPA_STORE) should mention WSL distros when interop exists.
	t.Setenv("GPA_STORE", "")
	t.Setenv("GPA_CODEX_HOME", "")
	t.Setenv("GPA_WINDOWS_CODEX", "")
	cfg := LoadConfig("")
	clients := DiscoverClients(cfg)
	if onWindows() || inWSL() {
		found := false
		for _, c := range clients {
			if c.Kind == "cli" && (c.ID == "win-cli" || len(c.Distro) > 0 || c.ID == "linux") {
				found = true
			}
		}
		if !found {
			t.Fatal(clients)
		}
	}
	_ = cfg
}
