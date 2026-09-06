package gpa

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type Account struct {
	Name string
	Meta map[string]any
	Auth map[string]any
}

func (a Account) Identity() Identity {
	id := InspectAuth(a.Auth)
	id.CredVersion = credVersion(a.Meta)
	return id
}

func credVersion(meta map[string]any) int {
	switch v := meta["cred_version"].(type) {
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	case int:
		return v
	}
	return 0
}

type Store struct {
	Root string
	Cfg  Config
}

func (s *Store) AccountsDir() string { return filepath.Join(s.Root, "accounts") }
func (s *Store) StatePath() string   { return filepath.Join(s.Root, "state.json") }
func (s *Store) LockPath() string    { return filepath.Join(s.Root, "gpa.lock") }
func (s *Store) LogPath() string     { return filepath.Join(s.Root, "logs", "ops.jsonl") }
func (s *Store) ClientsPath() string { return filepath.Join(s.Root, "clients.json") }
func (s *Store) OpsDir() string      { return filepath.Join(s.Root, "operations") }

func (s *Store) Ensure() error {
	if err := safePath(s.Root); err != nil {
		return err
	}
	for _, dir := range []string{s.Root, s.AccountsDir(), filepath.Join(s.Root, "logs"), s.OpsDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	_ = os.Chmod(s.Root, 0o700)
	return s.EnsureSchema()
}

func (s *Store) SlotDir(name string) (string, error) {
	if !ValidSlotName(name) {
		return "", fail("invalid name " + strconv.Quote(name) + "; use letters, digits, . _ -")
	}
	return filepath.Join(s.AccountsDir(), name), nil
}

func (s *Store) Names() []string {
	entries, err := os.ReadDir(s.AccountsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && exists(filepath.Join(s.AccountsDir(), e.Name(), "auth.json")) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func (s *Store) Get(name string) (Account, error) {
	dest, err := s.SlotDir(name)
	if err != nil {
		return Account{}, err
	}
	authPath := filepath.Join(dest, "auth.json")
	if !exists(authPath) {
		return Account{}, fail("no slot " + name + "; gpa list")
	}
	auth, err := readJSON(authPath)
	if err != nil {
		return Account{}, fail("cannot read " + authPath + ": " + err.Error())
	}
	meta, err := readJSON(filepath.Join(dest, "meta.json"))
	if err != nil {
		meta = map[string]any{}
	}
	return Account{Name: name, Meta: meta, Auth: auth}, nil
}

func (s *Store) State() map[string]any {
	data, err := readJSON(s.StatePath())
	if err != nil || data == nil {
		data = map[string]any{}
	}
	if _, ok := data["version"]; !ok {
		data["version"] = 1
	}
	if _, ok := data["current"]; !ok {
		data["current"] = nil
	}
	return data
}

func (s *Store) WriteState(state map[string]any) error {
	return writeJSON(s.StatePath(), state)
}

func (s *Store) Current() string {
	v := s.State()["current"]
	if v == nil {
		return ""
	}
	return asString(v)
}

func (s *Store) SetCurrent(name string) error {
	state := s.State()
	if name == "" {
		state["current"] = nil
	} else {
		state["current"] = name
	}
	return s.WriteState(state)
}

func (s *Store) Put(name string, auth map[string]any, source string, overwrite bool) (map[string]any, error) {
	id := InspectAuth(auth)
	if !IsChatGPTBundle(id) {
		return nil, fail("not a ChatGPT token bundle (need auth_mode=chatgpt and refresh_token)")
	}
	dest, err := s.SlotDir(name)
	if err != nil {
		return nil, err
	}
	authPath := filepath.Join(dest, "auth.json")
	if exists(authPath) && !overwrite {
		old, _ := readJSON(authPath)
		oldID := InspectAuth(old)
		if (oldID.UserID != "" || oldID.Email != "") && (id.UserID != "" || id.Email != "") && !SameSeat(oldID, id) {
			who := oldID.Email
			if who == "" && len(oldID.UserID) >= 12 {
				who = oldID.UserID[:12]
			} else if who == "" {
				who = oldID.UserID
			}
			return nil, fail("slot " + name + " already holds " + who + "; use another name or --force")
		}
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return nil, err
	}
	existing, _ := readJSON(filepath.Join(dest, "meta.json"))
	version := credVersion(existing) + 1
	if version < 1 {
		version = 1
	}
	meta := map[string]any{
		"name":         name,
		"email":        id.Email,
		"plan":         id.Plan,
		"orgs":         id.Orgs,
		"workspace_id": id.WorkspaceID,
		"account_id":   id.WorkspaceID,
		"user_id":      id.UserID,
		"sub":          id.Sub,
		"cred_version": version,
		"updated_at":   time.Now().UTC().Format(time.RFC3339),
		"source":       source,
	}
	preserveAccountMeta(existing, meta, id, name)
	if err := writeJSON(authPath, auth); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(dest, "meta.json"), meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *Store) FindByIdentity(id Identity) string {
	for _, name := range s.Names() {
		acct, err := s.Get(name)
		if err != nil {
			continue
		}
		if SameSeat(acct.Identity(), id) {
			return name
		}
	}
	return ""
}

func (s *Store) AppendLog(event string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["event"] = event
	fields["at"] = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.Marshal(fields)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.LogPath()), 0o700)
	f, err := os.OpenFile(s.LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
}
