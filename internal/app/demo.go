package app

import (
	"os"
	"path/filepath"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

func SeedDemo(store *gpa.Store) error {
	if err := store.Ensure(); err != nil {
		return err
	}
	plus := gpa.FakeAuth("plus@example.com", "user-plus", "plus", "ws-plus", "plus-refresh")
	biz1 := gpa.FakeAuth("alice@example.com", "user-biz1", "team", "ws-team", "biz1-refresh")
	biz2 := gpa.FakeAuth("bob@example.com", "user-biz2", "team", "ws-team", "biz2-refresh")
	if _, err := store.Put("plus", plus, "demo", true); err != nil {
		return err
	}
	if _, err := store.Put("biz1", biz1, "demo", true); err != nil {
		return err
	}
	if _, err := store.Put("biz2", biz2, "demo", true); err != nil {
		return err
	}
	_ = patchName(store, "plus", "日常账号")
	_ = patchName(store, "biz1", "工作账号 A")
	_ = patchName(store, "biz2", "工作账号 B")
	win := filepath.Join(store.Root, "demo-win")
	wsl := filepath.Join(store.Root, "demo-wsl")
	_ = os.MkdirAll(win, 0o700)
	_ = os.MkdirAll(wsl, 0o700)
	raw, _ := os.ReadFile(filepath.Join(mustSlot(store, "plus"), "auth.json"))
	_ = os.WriteFile(filepath.Join(win, "auth.json"), raw, 0o600)
	_ = os.WriteFile(filepath.Join(wsl, "auth.json"), raw, 0o600)
	clients := []gpa.Client{
		{ID: "app", Kind: "app", Label: "ChatGPT App", Storage: "windows-codex", WSLPath: filepath.Join(win, "auth.json")},
		{ID: "win-cli", Kind: "cli", Label: "Windows CLI", Storage: "windows-codex", WSLPath: filepath.Join(win, "auth.json")},
		{ID: "wsl:Ubuntu-24.04", Kind: "cli", Label: "Ubuntu-24.04 CLI", Storage: "posix-Ubuntu-24.04", Distro: "Ubuntu-24.04", WSLPath: filepath.Join(wsl, "auth.json")},
	}
	return store.SaveClients(clients)
}

func patchName(store *gpa.Store, slot, display string) error {
	acct, err := store.Get(slot)
	if err != nil {
		return err
	}
	acct.Meta["display_name"] = display
	return writeJSON(filepath.Join(mustSlot(store, slot), "meta.json"), acct.Meta)
}
