package gpa

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func testEnv(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GPA_STORE", filepath.Join(root, "gpa"))
	t.Setenv("GPA_CODEX_HOME", filepath.Join(root, "wsl-codex"))
	t.Setenv("GPA_WINDOWS_CODEX", filepath.Join(root, "win-codex"))
	t.Setenv("GPA_CHATGPT", "off")
	t.Setenv("GPA_IGNORE_CLI", "1")
	t.Setenv("GPA_CALLER", "human")
	os.MkdirAll(filepath.Join(root, "wsl-codex"), 0o700)
	os.MkdirAll(filepath.Join(root, "win-codex"), 0o700)
	cfg := LoadConfig("")
	store := OpenStore(cfg)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	plus := FakeAuth("plus@example.com", "user-plus", "plus", "ws-plus", "plus-refresh")
	biz1 := FakeAuth("biz1@example.com", "user-biz1", "team", "ws-team", "biz1-refresh")
	biz2 := FakeAuth("biz2@example.com", "user-biz2", "team", "ws-team", "biz2-refresh")
	store.Put("plus", plus, "test", true)
	store.Put("biz1", biz1, "test", true)
	store.Put("biz2", biz2, "test", true)
	store.SetCurrent("plus")
	raw, _ := json.Marshal(plus)
	os.WriteFile(filepath.Join(root, "wsl-codex", "auth.json"), raw, 0o600)
	os.WriteFile(filepath.Join(root, "win-codex", "auth.json"), raw, 0o600)
	_ = store.SaveClients(DiscoverClients(cfg))
	return store
}

func TestUseWritesSharedAndSeparate(t *testing.T) {
	store := testEnv(t)
	res := UseAccount(store, "biz1", "all", false, false, false, true)
	if res.Status != "completed" {
		t.Fatalf("%+v", res)
	}
	for _, c := range store.LoadClients() {
		auth := loadAuthFile(c.AuthPath())
		if auth == nil || InspectAuth(auth).UserID != "user-biz1" {
			t.Fatalf("%s not switched: %v", c.ID, auth)
		}
	}
	if store.Current() != "biz1" {
		t.Fatal(store.Current())
	}
}

func TestUseTargetAppDoesNotWriteOtherStorage(t *testing.T) {
	store := testEnv(t)
	// identify storages
	var appStore, other string
	for _, c := range store.LoadClients() {
		if c.Kind == "app" {
			appStore = c.Storage
		}
	}
	for _, c := range store.LoadClients() {
		if c.Storage != appStore {
			other = c.AuthPath()
		}
	}
	before := []byte("keep")
	if other != "" {
		before, _ = os.ReadFile(other)
	}
	res := UseAccount(store, "biz2", "app", false, false, false, true)
	if res.Status != "completed" {
		t.Fatalf("%+v", res)
	}
	if other != "" {
		after, _ := os.ReadFile(other)
		if string(after) != string(before) {
			t.Fatal("unrelated storage changed")
		}
	}
}

func TestSharedTargetDisclosesApp(t *testing.T) {
	store := testEnv(t)
	_, notes, err := ResolveTargets(store.LoadClients(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) == 0 {
		// app and win-cli share windows-codex
		affected, _, _ := ResolveTargets(store.LoadClients(), "app")
		ids := map[string]bool{}
		for _, c := range affected {
			ids[c.ID] = true
		}
		if !ids["app"] {
			t.Fatal(affected)
		}
	}
}

func TestPendingWhenAppRunningNonInteractive(t *testing.T) {
	store := testEnv(t)
	t.Setenv("GPA_FAKE_APP", "running")
	t.Setenv("GPA_CHATGPT", "auto")
	t.Setenv("GPA_CALLER", "app")
	before, _ := os.ReadFile(filepath.Join(store.Cfg.CodexHome, "auth.json"))
	res := UseAccount(store, "biz1", "all", false, false, false, false)
	if res.Status != "pending" {
		t.Fatalf("want pending got %+v", res)
	}
	if res.OperationID == "" {
		t.Fatal("missing operation id")
	}
	after, _ := os.ReadFile(filepath.Join(store.Cfg.CodexHome, "auth.json"))
	if string(after) != string(before) {
		t.Fatal("wrote while pending")
	}
	if store.Current() != "plus" {
		t.Fatal("current changed")
	}
}

func TestForceWritesWhileRunning(t *testing.T) {
	store := testEnv(t)
	t.Setenv("GPA_FAKE_APP", "running")
	t.Setenv("GPA_CHATGPT", "off") // don't actually start/stop
	res := UseAccount(store, "biz1", "all", true, false, false, false)
	if res.Status != "completed" {
		t.Fatalf("%+v", res)
	}
}

func TestDryRunDoesNotWrite(t *testing.T) {
	store := testEnv(t)
	before, _ := os.ReadFile(filepath.Join(store.Cfg.CodexHome, "auth.json"))
	res := UseAccount(store, "biz1", "all", false, false, true, true)
	if res.Status != "completed" {
		t.Fatalf("%+v", res)
	}
	after, _ := os.ReadFile(filepath.Join(store.Cfg.CodexHome, "auth.json"))
	if string(after) != string(before) {
		t.Fatal("dry-run wrote")
	}
}

func TestAdoptKeepsNewerRefresh(t *testing.T) {
	store := testEnv(t)
	newer := FakeAuth("plus@example.com", "user-plus", "plus", "ws-plus", "plus-new")
	newer["last_refresh"] = "2026-09-05T12:00:00Z"
	older := FakeAuth("plus@example.com", "user-plus", "plus", "ws-plus", "plus-old")
	older["last_refresh"] = "2026-09-01T00:00:00Z"
	nw, _ := json.Marshal(newer)
	ow, _ := json.Marshal(older)
	os.WriteFile(filepath.Join(store.Cfg.CodexHome, "auth.json"), nw, 0o600)
	os.WriteFile(filepath.Join(store.Cfg.WindowsCodex, "auth.json"), ow, 0o600)
	AdoptLives(store, store.LoadClients())
	if asString(asMap(storeMust(t, store, "plus").Auth["tokens"])["refresh_token"]) != "plus-new" {
		t.Fatal(storeMust(t, store, "plus").Auth)
	}
}

func storeMust(t *testing.T, store *Store, name string) Account {
	t.Helper()
	a, err := store.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRollbackOnSecondWriteFailure(t *testing.T) {
	store := testEnv(t)
	// make second unique storage unwritable if it exists
	clients := store.LoadClients()
	storages := uniqueStorages(clients)
	if len(storages) < 2 {
		t.Skip("need two storages")
	}
	second := clientsForStorage(clients, storages[1])[0].AuthPath()
	os.Remove(second)
	if err := os.MkdirAll(second, 0o700); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(clientsForStorage(clients, storages[0])[0].AuthPath())
	res := UseAccount(store, "biz1", "all", false, false, false, true)
	if res.Status != "failed" {
		t.Fatalf("want failed %+v", res)
	}
	after, _ := os.ReadFile(clientsForStorage(clients, storages[0])[0].AuthPath())
	if string(after) != string(before) {
		t.Fatal("did not roll back")
	}
	if store.Current() != "plus" {
		t.Fatal(store.Current())
	}
}

func TestLoginRejectsPathName(t *testing.T) {
	store := testEnv(t)
	victim := filepath.Join(t.TempDir(), "keep-me")
	os.MkdirAll(victim, 0o700)
	os.WriteFile(filepath.Join(victim, "marker"), []byte("safe"), 0o600)
	_, err := LoginAccount(store, victim, false, func(argv []string, env []string, cwd string) int {
		t.Fatal("runner should not run")
		return 0
	})
	if err == nil {
		t.Fatal("expected error")
	}
	raw, _ := os.ReadFile(filepath.Join(victim, "marker"))
	if string(raw) != "safe" {
		t.Fatal("touched victim")
	}
}

func TestLoginIsolated(t *testing.T) {
	store := testEnv(t)
	fresh := FakeAuth("biz3@example.com", "user-biz3", "team", "ws-team", "biz3-refresh")
	res, err := LoginAccount(store, "biz3", false, func(argv []string, env []string, cwd string) int {
		writeJSON(filepath.Join(cwd, "auth.json"), fresh)
		return 0
	})
	if err != nil {
		t.Fatal(err)
	}
	if asString(res["email"]) != "biz3@example.com" {
		t.Fatal(res)
	}
	if storeMust(t, store, "plus").Identity().Email != "plus@example.com" {
		t.Fatal("live slot clobbered")
	}
}

func TestMigrate(t *testing.T) {
	store := testEnv(t)
	legacy := filepath.Join(t.TempDir(), "legacy")
	os.MkdirAll(filepath.Join(legacy, "accounts", "extra"), 0o700)
	writeJSON(filepath.Join(legacy, "accounts", "extra", "auth.json"), FakeAuth("e@x.com", "u-e", "plus", "ws-e", "e"))
	writeJSON(filepath.Join(legacy, "state.json"), map[string]any{"current": "plus"})
	res, err := MigrateFrom(store, legacy, false)
	if err != nil {
		t.Fatal(err)
	}
	imps := asStringSlice(res["imported"])
	if len(imps) != 1 || imps[0] != "extra" {
		t.Fatal(res)
	}
}

func TestCLIListAndJSONUse(t *testing.T) {
	store := testEnv(t)
	code := Run([]string{"--store", store.Root, "list"}, os.Stdin, os.Stdout, os.Stderr)
	if code != 0 {
		t.Fatal(code)
	}
	code = Run([]string{"--store", store.Root, "--json", "use", "biz2"}, nilReader{}, discard{}, discard{})
	if code != 0 {
		t.Fatal(code)
	}
	if OpenStore(store.Cfg).Current() != "biz2" {
		t.Fatal(OpenStore(store.Cfg).Current())
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

type nilReader struct{}

func (nilReader) Read(p []byte) (int, error) { return 0, os.ErrClosed }

func TestNoTTYDoesNotCancel(t *testing.T) {
	store := testEnv(t)
	code := Run([]string{"--store", store.Root}, nilReader{}, discard{}, discard{})
	if code != 0 {
		t.Fatal(code)
	}
	if OpenStore(store.Cfg).Current() != "plus" {
		t.Fatal("empty input switched")
	}
}

func TestHostPathRoundTrip(t *testing.T) {
	got := toWSLPath(`C:\Users\alex\.codex`)
	if got != "/mnt/c/Users/alex/.codex" {
		t.Fatal(got)
	}
	back := toWindowsPath(got)
	if back != `C:\Users\alex\.codex` {
		t.Fatal(back)
	}
	if winJoin(`C:\Users\alex\AppData\Local`, "gpa", "bin", "gpa.exe") != `C:\Users\alex\AppData\Local\gpa\bin\gpa.exe` {
		t.Fatal(winJoin(`C:\Users\alex\AppData\Local`, "gpa", "bin", "gpa.exe"))
	}
	blob := "'\\\\wsl.localhost\\Ubuntu\\home\\x'\nUNC paths are not supported.\nC:\\Users\\zheng\\AppData\\Local\n"
	if lastWinPath(blob) != `C:\Users\zheng\AppData\Local` {
		t.Fatal(lastWinPath(blob))
	}
}
