package gpa

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = 2

func StableAccountID(id Identity, slot string) string {
	key := strings.TrimSpace(id.UserID) + "|" + strings.TrimSpace(id.WorkspaceID)
	if key == "|" {
		key = "slot|" + slot
	}
	sum := sha256.Sum256([]byte(key))
	return "acct_" + hex.EncodeToString(sum[:8])
}

func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return email
	}
	local, domain := email[:at], email[at:]
	if len(local) == 1 {
		return local + "***" + domain
	}
	return string(local[0]) + "***" + domain
}

func preserveAccountMeta(existing, meta map[string]any, id Identity, name string) {
	if existing == nil {
		existing = map[string]any{}
	}
	if v := asString(existing["id"]); v != "" {
		meta["id"] = v
	} else {
		meta["id"] = StableAccountID(id, name)
	}
	if v := asString(existing["display_name"]); v != "" {
		meta["display_name"] = v
	} else {
		meta["display_name"] = name
	}
	if archived, ok := existing["archived"].(bool); ok {
		meta["archived"] = archived
	} else {
		meta["archived"] = false
	}
	if existing["legacy_aliases"] != nil {
		meta["legacy_aliases"] = existing["legacy_aliases"]
	} else {
		meta["legacy_aliases"] = []string{name}
	}
}

func (s *Store) EnsureSchema() error {
	state := s.State()
	cur := 1
	switch v := state["schema_version"].(type) {
	case float64:
		cur = int(v)
	case int:
		cur = v
	}
	if cur >= SchemaVersion {
		return s.ensureAccountIDs()
	}
	backupDir := filepath.Join(s.Root, "backups", time.Now().UTC().Format("20060102T150405")+"-schema")
	_ = os.MkdirAll(backupDir, 0o700)
	if raw, err := os.ReadFile(s.StatePath()); err == nil {
		_ = atomicWrite(filepath.Join(backupDir, "state.json"), raw, 0o600)
	}
	if err := s.ensureAccountIDs(); err != nil {
		return err
	}
	for _, op := range s.ListOperations() {
		if op.Status == "applied" {
			op.Status = "succeeded"
			_ = s.SaveOperation(op)
		}
	}
	state = s.State()
	state["schema_version"] = SchemaVersion
	return s.WriteState(state)
}

func (s *Store) ensureAccountIDs() error {
	for _, name := range s.Names() {
		acct, err := s.Get(name)
		if err != nil {
			continue
		}
		if asString(acct.Meta["id"]) != "" && asString(acct.Meta["display_name"]) != "" {
			continue
		}
		if _, err := s.Put(name, acct.Auth, asString(acct.Meta["source"]), true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AccountID(acct Account) string {
	if v := asString(acct.Meta["id"]); v != "" {
		return v
	}
	return StableAccountID(acct.Identity(), acct.Name)
}

func (s *Store) DisplayName(acct Account) string {
	if v := asString(acct.Meta["display_name"]); v != "" {
		return v
	}
	return acct.Name
}

func (s *Store) Archived(acct Account) bool {
	v, _ := acct.Meta["archived"].(bool)
	return v
}

func (s *Store) ResolveAccount(ref string) (Account, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Account{}, fail("account is required")
	}
	if ValidSlotName(ref) {
		if acct, err := s.Get(ref); err == nil {
			return acct, nil
		}
	}
	var matches []Account
	for _, name := range s.Names() {
		acct, err := s.Get(name)
		if err != nil {
			continue
		}
		if s.AccountID(acct) == ref || strings.EqualFold(s.DisplayName(acct), ref) {
			matches = append(matches, acct)
			continue
		}
		for _, alias := range asStringSlice(acct.Meta["legacy_aliases"]) {
			if alias == ref {
				matches = append(matches, acct)
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return Account{}, fail("account " + ref + " is ambiguous")
	}
	return Account{}, fail("no account " + ref)
}
