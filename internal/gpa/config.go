package gpa

import (
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	Store        string
	CodexHome    string
	WindowsCodex string
	ChatGPTMode  string
}

func envOr(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func defaultSharedStore() string {
	if p := envOr("GPA_STORE"); p != "" {
		return p
	}
	if onWindows() || inWSL() {
		if app := windowsLocalAppData(); app != "" {
			return nativePath(winJoin(app, "gpa"))
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "gpa")
}

func defaultCodexHome() string {
	if p := envOr("GPA_CODEX_HOME", "CODEX_HOME"); p != "" {
		return p
	}
	if onWindows() {
		if home := windowsUserProfile(); home != "" {
			return filepath.Join(home, ".codex")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func defaultWindowsCodex() string {
	if p := envOr("GPA_WINDOWS_CODEX", "GPT_ACCOUNT_WINDOWS_CODEX"); p != "" {
		return nativePath(p)
	}
	if home := windowsUserProfile(); home != "" {
		return nativePath(winJoin(home, ".codex"))
	}
	return ""
}

func LoadConfig(storeOverride string) Config {
	store := storeOverride
	if store == "" {
		store = defaultSharedStore()
	}
	mode := envOr("GPA_CHATGPT")
	if mode == "" {
		mode = "auto"
	}
	win := defaultWindowsCodex()
	codex := defaultCodexHome()
	if runtime.GOOS == "windows" && win != "" {
		codex = win
	}
	return Config{
		Store:        store,
		CodexHome:    codex,
		WindowsCodex: win,
		ChatGPTMode:  mode,
	}
}

func OpenStore(cfg Config) *Store {
	return &Store{Root: cfg.Store, Cfg: cfg}
}

func legacyStores() []string {
	home, _ := os.UserHomeDir()
	var out []string
	for _, p := range []string{
		filepath.Join(home, ".local", "share", "gpa"),
		filepath.Join(home, ".local", "share", "gpt-accounts"),
	} {
		if exists(filepath.Join(p, "accounts")) {
			out = append(out, p)
		}
	}
	return out
}
