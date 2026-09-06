package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

type asyncLogin struct {
	rec    LoginRecord
	cancel chan struct{}
}

func (s *Service) StartLogin(displayName, updateAccount string) (LoginRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logins == nil {
		s.logins = map[string]*asyncLogin{}
	}
	rec := LoginRecord{
		ID:          newID("login_"),
		Status:      "starting",
		DisplayName: strings.TrimSpace(displayName),
		CreatedAt:   nowISO(),
	}
	if updateAccount != "" {
		acct, err := s.Store.ResolveAccount(updateAccount)
		if err != nil {
			return LoginRecord{}, err
		}
		rec.Slot = acct.Name
		rec.AccountID = s.Store.AccountID(acct)
	}
	task := &asyncLogin{rec: rec, cancel: make(chan struct{})}
	s.logins[rec.ID] = task
	_ = s.saveLogin(rec)
	go s.runLogin(rec.ID)
	return rec, nil
}

func (s *Service) LoginStatus(id string) (LoginRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.logins[id]; t != nil {
		return t.rec, nil
	}
	return s.loadLogin(id)
}

func (s *Service) CancelLogin(id string) (LoginRecord, error) {
	s.mu.Lock()
	t := s.logins[id]
	s.mu.Unlock()
	if t != nil {
		select {
		case <-t.cancel:
		default:
			close(t.cancel)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t == nil {
		rec, err := s.loadLogin(id)
		if err != nil {
			return LoginRecord{}, err
		}
		rec.Status = "cancelled"
		_ = s.saveLogin(rec)
		return rec, nil
	}
	t.rec.Status = "cancelled"
	t.rec.Message = "已取消本次登录"
	_ = s.saveLogin(t.rec)
	return t.rec, nil
}

func (s *Service) runLogin(id string) {
	s.mu.Lock()
	task := s.logins[id]
	s.mu.Unlock()
	if task == nil {
		return
	}
	s.setLogin(id, func(r *LoginRecord) {
		r.Status = "waiting_authorization"
		r.VerificationURL = "https://auth.openai.com/otp/code"
		r.UserCode = "DEMO-CODE"
	})
	if s.Login == nil && !s.Demo {
		s.setLogin(id, func(r *LoginRecord) {
			r.Status = "failed"
			r.Message = "本机没有可用的隔离登录适配器。安装 Codex CLI 后重试。"
		})
		return
	}
	name := task.rec.DisplayName
	if name == "" {
		name = "account"
	}
	if s.Demo || s.Login != nil {
		select {
		case <-task.cancel:
			return
		case <-time.After(30 * time.Millisecond):
		}
		if s.Login != nil {
			meta, err := s.Login(s.Store, sanitizeSlot(name))
			if err != nil {
				s.setLogin(id, func(r *LoginRecord) {
					r.Status = "failed"
					r.Message = err.Error()
				})
				return
			}
			s.setLogin(id, func(r *LoginRecord) {
				r.Status = "succeeded"
				r.Slot = asStr(meta["slot"])
				r.EmailHint = gpa.MaskEmail(asStr(meta["email"]))
				r.Plan = asStr(meta["plan"])
				r.UpdatedExisting = asStr(meta["updated"]) == "true"
			})
			return
		}
		// demo: create a fake account if store is isolated
		s.setLogin(id, func(r *LoginRecord) {
			r.Status = "succeeded"
			r.EmailHint = "d***@example.com"
			r.Plan = "Plus"
			r.Message = "演示登录完成，未写入真实凭据"
		})
	}
}

func (s *Service) setLogin(id string, fn func(*LoginRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.logins[id]
	if t == nil {
		return
	}
	fn(&t.rec)
	_ = s.saveLogin(t.rec)
}

func sanitizeSlot(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "account"
	}
	return out
}

func (s *Service) loginsDir() string { return filepath.Join(s.Store.Root, "logins") }

func (s *Service) saveLogin(rec LoginRecord) error {
	_ = os.MkdirAll(s.loginsDir(), 0o700)
	return writeJSON(filepath.Join(s.loginsDir(), rec.ID+".json"), rec)
}

func (s *Service) loadLogin(id string) (LoginRecord, error) {
	raw, err := os.ReadFile(filepath.Join(s.loginsDir(), id+".json"))
	if err != nil {
		return LoginRecord{}, errf("no login " + id)
	}
	var rec LoginRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return LoginRecord{}, err
	}
	return rec, nil
}

func asStr(v any) string {
	s, _ := v.(string)
	return s
}

