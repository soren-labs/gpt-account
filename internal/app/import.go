package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

func (s *Service) ImportPreview() (ImportPreview, error) {
	prev := ImportPreview{}
	for _, src := range legacySources() {
		if filepath.Clean(src) == filepath.Clean(s.Store.Root) {
			continue
		}
		accounts := filepath.Join(src, "accounts")
		if !dirOK(accounts) {
			continue
		}
		prev.Sources = append(prev.Sources, src)
		entries, _ := os.ReadDir(accounts)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			auth, err := readMap(filepath.Join(accounts, e.Name(), "auth.json"))
			if err != nil || !gpa.IsChatGPTBundle(gpa.InspectAuth(auth)) {
				continue
			}
			id := gpa.InspectAuth(auth)
			existing := s.Store.FindByIdentity(id)
			if existing == "" {
				if _, err := s.Store.Get(e.Name()); err == nil {
					prev.Conflicts = append(prev.Conflicts, e.Name())
				} else {
					prev.New = append(prev.New, e.Name())
				}
				continue
			}
			acct, _ := s.Store.Get(existing)
			if refreshOf(acct.Auth) == refreshOf(auth) {
				prev.Mergeable = append(prev.Mergeable, e.Name())
			} else {
				prev.Conflicts = append(prev.Conflicts, e.Name())
			}
		}
	}
	return prev, nil
}

func (s *Service) ImportApply() (map[string]int, error) {
	release, err := s.lockMutation()
	if err != nil {
		return nil, err
	}
	defer release()
	prev, err := s.ImportPreview()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{"added": 0, "skipped": 0, "conflicts": 0}
	for _, src := range prev.Sources {
		entries, err := os.ReadDir(filepath.Join(src, "accounts"))
		if err != nil {
			return counts, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			auth, err := readMap(filepath.Join(src, "accounts", e.Name(), "auth.json"))
			if err != nil || !gpa.IsChatGPTBundle(gpa.InspectAuth(auth)) {
				continue
			}
			existing := s.Store.FindByIdentity(gpa.InspectAuth(auth))
			if existing != "" {
				a, _ := s.Store.Get(existing)
				if refreshOf(a.Auth) == refreshOf(auth) {
					counts["skipped"]++
				} else {
					counts["conflicts"]++
				}
				continue
			}
			if _, err := s.Store.Get(e.Name()); err == nil {
				counts["conflicts"]++
				continue
			}
			if _, err := s.Store.Put(e.Name(), auth, "legacy-import", false); err != nil {
				return counts, err
			}
			counts["added"]++
		}
	}
	return counts, nil
}

func (s *Service) Diagnostics() map[string]any {
	clients := s.Store.LoadClients()
	var rows []map[string]any
	for _, c := range clients {
		st := s.inspect(c)
		writable := "unknown"
		if p := c.AuthPath(); p != "" {
			if _, err := os.Stat(filepath.Dir(p)); err == nil {
				writable = "dir_ok"
			} else {
				writable = "missing"
			}
		}
		rows = append(rows, map[string]any{
			"id": c.ID, "label": c.Label, "presence": presenceOf(st),
			"reason": st.ReasonCode, "path_ok": writable,
		})
	}
	return map[string]any{
		"version":  "0.3.0",
		"store":    s.Store.Root,
		"demo":     s.Demo,
		"clients":  rows,
		"accounts": len(s.Store.Names()),
	}
}

func legacySources() []string {
	home, _ := os.UserHomeDir()
	var out []string
	for _, p := range []string{
		filepath.Join(home, ".local", "share", "gpa"),
		filepath.Join(home, ".local", "share", "gpt-accounts"),
	} {
		if dirOK(filepath.Join(p, "accounts")) {
			out = append(out, p)
		}
	}
	return out
}

func dirOK(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func readMap(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}
