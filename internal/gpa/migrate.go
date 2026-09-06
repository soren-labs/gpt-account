package gpa

import (
	"os"
	"path/filepath"
)

func MigrateFrom(store *Store, source string, force bool) (map[string]any, error) {
	accounts := filepath.Join(source, "accounts")
	info, err := os.Stat(accounts)
	if err != nil || !info.IsDir() {
		return nil, fail("no legacy accounts in " + source)
	}
	entries, err := os.ReadDir(accounts)
	if err != nil {
		return nil, fail(err.Error())
	}
	var imported, skipped []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		authPath := filepath.Join(accounts, name, "auth.json")
		if !exists(authPath) {
			continue
		}
		auth, err := readJSON(authPath)
		if err != nil || !IsChatGPTBundle(InspectAuth(auth)) {
			skipped = append(skipped, name)
			continue
		}
		dest, err := store.SlotDir(name)
		if err != nil {
			skipped = append(skipped, name)
			continue
		}
		if exists(filepath.Join(dest, "auth.json")) && !force {
			skipped = append(skipped, name)
			continue
		}
		if _, err := store.Put(name, auth, authPath, true); err != nil {
			skipped = append(skipped, name)
			continue
		}
		imported = append(imported, name)
	}
	legacy, _ := readJSON(filepath.Join(source, "state.json"))
	current := asString(legacy["current"])
	if current != "" {
		found := false
		for _, n := range store.Names() {
			if n == current {
				found = true
			}
		}
		if found && (force || store.Current() == "") {
			_ = store.SetCurrent(current)
		}
	}
	store.AppendLog("migrate", map[string]any{"source": source, "imported": imported, "skipped": skipped})
	return map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"current":  store.Current(),
		"source":   source,
		"store":    store.Root,
	}, nil
}

func DiscoverLegacy() []map[string]any {
	var out []map[string]any
	for _, src := range legacyStores() {
		names := []string{}
		entries, err := os.ReadDir(filepath.Join(src, "accounts"))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && exists(filepath.Join(src, "accounts", e.Name(), "auth.json")) {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			continue
		}
		out = append(out, map[string]any{"path": src, "accounts": names})
	}
	return out
}
