package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

type Service struct {
	Store        *gpa.Store
	Demo         bool
	Probe        ProbeFunc
	Switch       SwitchFunc
	Login        LoginFunc
	LoginCommand func(context.Context, []string) *exec.Cmd

	opMu   sync.Mutex
	mu     sync.Mutex
	logins map[string]*asyncLogin
}

func New(store *gpa.Store) *Service {
	return &Service{Store: store, logins: map[string]*asyncLogin{}}
}

func (s *Service) inspect(c gpa.Client) gpa.ClientState {
	if s.Probe != nil {
		return s.Probe(c)
	}
	return gpa.InspectClient(c)
}

func (s *Service) switchAccount(name, target string, restart bool) gpa.Result {
	if s.Switch != nil {
		return s.Switch(s.Store, name, target, restart)
	}
	return gpa.UseAccount(s.Store, name, target, restart, false, false, false)
}

func newID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func (s *Service) Status(selected string) (StatusView, error) {

	clients := s.Store.LoadClients()
	targets := s.targets(clients)
	if selected == "" {
		selected = s.preferredTarget(targets)
	}
	tv, err := s.targetByID(targets, selected)
	if err != nil {
		// An empty installation has no target yet, but the web manager must
		// still open so the user can import accounts or inspect setup state.
		if selected != "" {
			return StatusView{}, err
		}
		tv = TargetView{}
	}
	lives := s.liveByStorage(clients)
	accounts := s.listAccounts(false, clients, lives)
	mixed := false
	current := ""
	if tv.ID != "" {
		seats := map[string]string{}
		for _, id := range tv.Members {
			c := clientByID(clients, id)
			if c.ID == "" {
				continue
			}
			if name := lives[c.Storage]; name != "" {
				seats[c.Storage] = name
			}
		}
		uniq := uniqueValues(seats)
		if len(uniq) > 1 {
			mixed = true
			current = "各客户端使用不同账号"
		} else if len(uniq) == 1 {
			if acct, err := s.Store.Get(uniq[0]); err == nil {
				current = s.Store.DisplayName(acct)
			} else {
				current = uniq[0]
			}
		}
	}
	return StatusView{
		Demo:           s.Demo,
		Connected:      true,
		Store:          s.Store.Root,
		SelectedTarget: selected,
		MixedCurrent:   mixed,
		Accounts:       accounts,
		Targets:        targets,
		CurrentLabel:   current,
		Operations:     s.ListOps(20),
	}, nil
}

func (s *Service) listAccounts(includeArchived bool, clients []gpa.Client, lives map[string]string) []AccountView {
	var out []AccountView
	for _, name := range s.Store.Names() {
		acct, err := s.Store.Get(name)
		if err != nil {
			continue
		}
		if s.Store.Archived(acct) && !includeArchived {
			continue
		}
		id := acct.Identity()
		on := []string{}
		for _, c := range clients {
			if lives[c.Storage] == name {
				on = append(on, c.ID)
			}
		}
		out = append(out, AccountView{
			ID:                 s.Store.AccountID(acct),
			Slot:               name,
			DisplayName:        s.Store.DisplayName(acct),
			LegacyAliases:      gpaAliases(acct),
			EmailHint:          gpa.MaskEmail(id.Email),
			Plan:               id.PlanLabel(),
			Archived:           s.Store.Archived(acct),
			CredentialRevision: credVer(acct),
			ConfiguredOn:       on,
			CurrentOn:          on,
			Verification:       map[string]any{"local_credentials": "stored", "online": "not_checked"},
		})
	}
	return out
}

func credVer(acct gpa.Account) int {
	return intFrom(acct.Meta["cred_version"])
}

func intFrom(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	}
	return 0
}

func gpaAliases(acct gpa.Account) []string {
	out := []string{acct.Name}
	for _, a := range asStrings(acct.Meta["legacy_aliases"]) {
		if a != acct.Name {
			out = append(out, a)
		}
	}
	return out
}

func asStrings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func (s *Service) AccountDetail(ref string) (AccountView, error) {
	acct, err := s.Store.ResolveAccount(ref)
	if err != nil {
		return AccountView{}, err
	}
	clients := s.Store.LoadClients()
	lives := s.liveByStorage(clients)
	for _, v := range s.listAccounts(true, clients, lives) {
		if v.ID == s.Store.AccountID(acct) || v.Slot == acct.Name {
			v.Email = acct.Identity().Email
			return v, nil
		}
	}
	return AccountView{}, errf("account not found")
}

func (s *Service) PatchAccount(ref, displayName string, archived *bool) (AccountView, error) {
	release, err := s.lockMutation()
	if err != nil {
		return AccountView{}, err
	}
	defer release()
	acct, err := s.Store.ResolveAccount(ref)
	if err != nil {
		return AccountView{}, err
	}
	if displayName != "" {
		acct.Meta["display_name"] = strings.TrimSpace(displayName)
	}
	if archived != nil {
		acct.Meta["archived"] = *archived
	}
	if err := writeJSON(filepath.Join(mustSlot(s.Store, acct.Name), "meta.json"), acct.Meta); err != nil {
		return AccountView{}, err
	}
	return s.AccountDetail(acct.Name)
}

func mustSlot(store *gpa.Store, name string) string {
	p, _ := store.SlotDir(name)
	return p
}

func (s *Service) liveByStorage(clients []gpa.Client) map[string]string {
	out := map[string]string{}
	seen := map[string]bool{}
	for _, c := range clients {
		if seen[c.Storage] {
			continue
		}
		seen[c.Storage] = true
		auth := gpa.LoadClientAuth(c)
		if auth == nil {
			continue
		}
		id := gpa.InspectAuth(auth)
		if name := s.Store.FindByIdentity(id); name != "" {
			out[c.Storage] = name
		}
	}
	return out
}

func (s *Service) targets(clients []gpa.Client) []TargetView {
	var out []TargetView
	app := clientByKind(clients, "app")
	if app.ID != "" {
		members := []string{app.ID}
		note := ""
		for _, c := range clients {
			if c.ID != app.ID && c.Storage == app.Storage {
				members = append(members, c.ID)
				note = "Windows App 和 Windows CLI 共用登录，会一起更新"
			}
		}
		st := s.inspect(app)
		out = append(out, TargetView{
			ID: "desktop", Label: "桌面端", Members: members, SharedNote: note,
			Available: true, Presence: string(presenceOf(st)), ReasonCode: st.ReasonCode, Detail: st.Detail,
		})
	}
	for _, c := range clients {
		if c.Kind != "cli" || c.ID == "win-cli" {
			continue
		}
		st := s.inspect(c)
		out = append(out, TargetView{
			ID: c.ID, Label: c.Label, Members: []string{c.ID},
			Available: true, Presence: string(presenceOf(st)), ReasonCode: st.ReasonCode, Detail: st.Detail,
		})
	}
	if len(clients) > 0 {
		var all []string
		for _, c := range clients {
			all = append(all, c.ID)
		}
		out = append(out, TargetView{ID: "all", Label: "本机全部", Members: all, Available: true, Presence: "none"})
	}
	return out
}

func (s *Service) preferredTarget(targets []TargetView) string {
	for _, t := range targets {
		if t.ID == "desktop" {
			return t.ID
		}
	}
	if len(targets) > 0 {
		return targets[0].ID
	}
	return ""
}

func (s *Service) targetByID(targets []TargetView, id string) (TargetView, error) {
	for _, t := range targets {
		if t.ID == id {
			return t, nil
		}
	}
	return TargetView{}, errf("unknown target " + id)
}

func clientByID(clients []gpa.Client, id string) gpa.Client {
	for _, c := range clients {
		if c.ID == id {
			return c
		}
	}
	return gpa.Client{}
}

func clientByKind(clients []gpa.Client, kind string) gpa.Client {
	for _, c := range clients {
		if c.Kind == kind {
			return c
		}
	}
	return gpa.Client{}
}

func presenceOf(st gpa.ClientState) gpa.Presence {
	if st.Presence != "" {
		return st.Presence
	}
	if st.Process == gpa.ProcNone || st.Process == gpa.ProcIdle {
		return gpa.PresenceNone
	}
	if st.ReasonCode == "QUERY_FAILED" {
		return gpa.PresenceUnknown
	}
	return gpa.PresenceRunning
}

func uniqueValues(m map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range m {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func writeJSON(path string, obj any) error {
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return gpa.AtomicWriteFile(path, raw)
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

type appError struct{ msg string }

func (e *appError) Error() string { return e.msg }

func errf(msg string) error { return &appError{msg: msg} }

func (s *Service) lockMutation() (func(), error) {
	s.opMu.Lock()
	lock, err := gpa.AcquireLock(s.Store.LockPath())
	if err != nil {
		s.opMu.Unlock()
		return nil, err
	}
	return func() { lock.Release(); s.opMu.Unlock() }, nil
}

// Recover is called once by the sole host before accepting requests.
func (s *Service) Recover() error {
	release, err := s.lockMutation()
	if err != nil {
		return err
	}
	defer release()
	_, journalErr := os.Stat(filepath.Join(s.Store.Root, "switch-journal.json"))
	recoveryErr := gpa.RecoverTransaction(s.Store)
	for _, op := range s.ListOps(0) {
		if op.Status != "running" && op.Status != "queued" {
			continue
		}
		op.Status = "failed"
		op.ReasonCode = "INTERRUPTED"
		op.Message = "后台中断，请检查恢复结果后重试"
		status := "unknown"
		if journalErr == nil {
			status = "restored"
		}
		if recoveryErr != nil {
			status = "failed"
			op.Message = recoveryErr.Error()
		}
		op.Recovery = map[string]any{"status": status}
		op.UpdatedAt = nowISO()
		if err := s.saveOp(op); err != nil {
			return err
		}
	}
	entries, _ := os.ReadDir(s.loginsDir())
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		r, err := s.loadLogin(id)
		if err != nil {
			continue
		}
		if r.Status == "starting" || r.Status == "waiting_authorization" {
			r.Status = "expired"
			r.Message = "后台已重启，请重新开始授权"
			r.UserCode = ""
			r.VerificationURL = ""
			if err := s.saveLogin(r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) ArchivedAccounts() []AccountView {
	clients := s.Store.LoadClients()
	all := s.listAccounts(true, clients, s.liveByStorage(clients))
	out := []AccountView{}
	for _, a := range all {
		if a.Archived {
			out = append(out, a)
		}
	}
	return out
}
