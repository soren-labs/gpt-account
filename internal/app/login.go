package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

type asyncLogin struct {
	rec    LoginRecord
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *Service) StartLogin(displayName, updateAccount string) (LoginRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, task := range s.logins {
		if task.rec.Status == "starting" || task.rec.Status == "waiting_authorization" {
			return LoginRecord{}, errf("已有登录正在进行，请完成或取消后再添加")
		}
	}
	rec := LoginRecord{ID: newID("login_"), Status: "starting", DisplayName: strings.TrimSpace(displayName), CreatedAt: nowISO()}
	if updateAccount != "" {
		a, err := s.Store.ResolveAccount(updateAccount)
		if err != nil {
			return rec, err
		}
		rec.Slot = a.Name
		rec.AccountID = s.Store.AccountID(a)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	task := &asyncLogin{rec: rec, ctx: ctx, cancel: cancel}
	if err := s.saveLogin(rec); err != nil {
		cancel()
		return rec, err
	}
	s.logins[rec.ID] = task
	go s.runLogin(task)
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
	defer s.mu.Unlock()
	if t := s.logins[id]; t != nil {
		if t.rec.Status == "starting" || t.rec.Status == "waiting_authorization" {
			t.cancel()
			t.rec.Status = "cancelled"
			t.rec.UserCode = ""
			t.rec.VerificationURL = ""
			t.rec.Message = "已取消本次登录"
			if err := s.saveLogin(t.rec); err != nil {
				return t.rec, err
			}
		}
		return t.rec, nil
	}
	return s.loadLogin(id)
}
func (s *Service) runLogin(task *asyncLogin) {
	defer task.cancel()
	s.mu.Lock()
	rec := task.rec
	s.mu.Unlock()
	var auth map[string]any
	var err error
	if s.Demo {
		select {
		case <-task.ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
		user := newID("demo_")
		if rec.Slot != "" {
			a, e := s.Store.Get(rec.Slot)
			if e != nil {
				err = e
			} else {
				auth = a.Auth
			}
		}
		if auth == nil {
			auth = gpa.FakeAuth(user+"@example.invalid", user, "plus", user, "demo-refresh")
		}
	} else {
		auth, err = s.captureLogin(task)
	}
	if err != nil {
		s.finishLogin(task, "failed", err.Error())
		return
	}
	release, err := s.lockMutation()
	if err != nil {
		s.finishLogin(task, "failed", err.Error())
		return
	}
	defer release()
	// Cancellation and credential commit are serialized: a cancelled task cannot
	// later publish a successful account or overwrite the cancelled status.
	s.mu.Lock()
	defer s.mu.Unlock()
	if task.ctx.Err() != nil || task.rec.Status == "cancelled" {
		return
	}
	id := gpa.InspectAuth(auth)
	if !gpa.IsChatGPTBundle(id) {
		task.rec.Status = "failed"
		task.rec.Message = "登录未产生可用的订阅凭据"
		_ = s.saveLogin(task.rec)
		return
	}
	slot := s.Store.FindByIdentity(id)
	updated := slot != ""
	if rec.Slot != "" {
		old, e := s.Store.Get(rec.Slot)
		if e != nil || !gpa.SameSeat(old.Identity(), id) {
			task.rec.Status = "failed"
			task.rec.IdentityMismatch = true
			task.rec.Message = "登录身份与原账号不同，原账号未修改；请通过添加账号保存其他身份"
			_ = s.saveLogin(task.rec)
			return
		}
		slot = rec.Slot
		updated = true
	}
	if slot == "" {
		slot = newID("account-")
	}
	_, err = s.Store.Put(slot, auth, "isolated-login", true)
	if err == nil && rec.DisplayName != "" {
		a, e := s.Store.Get(slot)
		if e == nil {
			a.Meta["display_name"] = rec.DisplayName
			err = writeJSON(filepath.Join(mustSlot(s.Store, slot), "meta.json"), a.Meta)
		} else {
			err = e
		}
	}
	task.rec.UserCode = ""
	task.rec.VerificationURL = ""
	if err != nil {
		task.rec.Status = "failed"
		task.rec.Message = "保存登录失败: " + err.Error()
	} else {
		a, _ := s.Store.Get(slot)
		task.rec.Status = "succeeded"
		task.rec.Slot = slot
		task.rec.AccountID = s.Store.AccountID(a)
		task.rec.EmailHint = gpa.MaskEmail(id.Email)
		task.rec.Plan = id.PlanLabel()
		task.rec.UpdatedExisting = updated
	}
	_ = s.saveLogin(task.rec)
}
func (s *Service) finishLogin(task *asyncLogin, status, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if task.rec.Status == "cancelled" {
		return
	}
	if task.ctx.Err() == context.DeadlineExceeded {
		status = "expired"
		message = "授权超时，请重新开始"
	}
	task.rec.Status = status
	task.rec.Message = message
	task.rec.UserCode = ""
	task.rec.VerificationURL = ""
	_ = s.saveLogin(task.rec)
}
func (s *Service) captureLogin(task *asyncLogin) (map[string]any, error) {
	root := filepath.Join(s.Store.Root, "_inbox")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	inbox, err := os.MkdirTemp(root, "web-login-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(inbox)
	argv := []string{"login", "--device-auth", "-c", `cli_auth_credentials_store="file"`}
	var cmd *exec.Cmd
	if s.LoginCommand != nil {
		cmd = s.LoginCommand(task.ctx, argv)
	} else {
		cmd, err = newLoginCommand(task.ctx, argv)
		if err != nil {
			return nil, err
		}
	}
	var env []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch strings.ToUpper(key) {
		case "CODEX_HOME", "OPENAI_API_KEY", "CODEX_ACCESS_TOKEN":
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "CODEX_HOME="+inbox)
	cmd.Dir = inbox
	cmd.Stdin = nil
	cmd.WaitDelay = 2 * time.Second
	configureLoginProcess(cmd)
	output := &loginOutput{emit: func(code, link string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if task.rec.Status == "cancelled" {
			return
		}
		task.rec.Status = "waiting_authorization"
		if code != "" {
			task.rec.UserCode = code
		}
		if link != "" {
			task.rec.VerificationURL = link
		}
		_ = s.saveLogin(task.rec)
	}}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("官方登录未完成，请检查设备码权限或重试（%v）", err)
	}
	raw, err := os.ReadFile(filepath.Join(inbox, "auth.json"))
	if err != nil {
		return nil, fmt.Errorf("未取得隔离认证文件: %w", err)
	}
	var auth map[string]any
	if err = json.Unmarshal(raw, &auth); err != nil {
		return nil, err
	}
	return auth, nil
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
var deviceCode = regexp.MustCompile(`\b[A-Z0-9]{4,6}-[A-Z0-9]{4,6}\b`)
var authURL = regexp.MustCompile(`https://auth\.openai\.com/[A-Za-z0-9/_-]+`)

type loginOutput struct {
	mu   sync.Mutex
	buf  string
	emit func(string, string)
}

func (w *loginOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf += string(p)
	if len(w.buf) > 16384 {
		w.buf = w.buf[len(w.buf)-16384:]
	}
	clean := ansi.ReplaceAllString(w.buf, "")
	code := deviceCode.FindString(clean)
	link := authURL.FindString(clean)
	if link != "" {
		u, e := url.Parse(link)
		if e != nil || u.Scheme != "https" || u.Host != "auth.openai.com" {
			link = ""
		}
	}
	if code != "" || link != "" {
		w.emit(code, link)
	}
	return len(p), nil
}
func (s *Service) loginsDir() string { return filepath.Join(s.Store.Root, "logins") }
func (s *Service) saveLogin(rec LoginRecord) error {
	rec.UserCode = ""
	rec.VerificationURL = ""
	if err := os.MkdirAll(s.loginsDir(), 0700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.loginsDir(), rec.ID+".json"), rec)
}
func (s *Service) loadLogin(id string) (LoginRecord, error) {
	if !gpa.ValidSlotName(id) {
		return LoginRecord{}, errf("invalid login id")
	}
	raw, err := os.ReadFile(filepath.Join(s.loginsDir(), id+".json"))
	if err != nil {
		return LoginRecord{}, errf("no login " + id)
	}
	var rec LoginRecord
	err = json.Unmarshal(raw, &rec)
	return rec, err
}
