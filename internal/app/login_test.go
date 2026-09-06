package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/soren-labs/gpt-account/internal/gpa"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginProcessHelper(t *testing.T) {
	if os.Getenv("GPA_TEST_LOGIN_PROCESS") != "1" {
		return
	}
	fmt.Println("https://auth.openai.com/codex/device\nABCD-EFGHI")
	time.Sleep(250 * time.Millisecond)
	a := gpa.FakeAuth("alice@example.com", "user-biz1", "team", "ws-team", "new-authorization")
	raw, _ := json.Marshal(a)
	if err := os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json"), raw, 0600); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
func productionFixture(t *testing.T) *Service {
	s := demoSvc(t)
	s.Demo = false
	t.Setenv("GPA_TEST_LOGIN_PROCESS", "1")
	s.LoginCommand = func(ctx context.Context, args []string) *exec.Cmd {
		if strings.Join(args, " ") != `login --device-auth -c cli_auth_credentials_store="file"` {
			t.Errorf("unsafe login arguments: %v", args)
		}
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestLoginProcessHelper", "--")
	}
	return s
}
func awaitLogin(t *testing.T, s *Service, id string, terminal bool) LoginRecord {
	t.Helper()
	end := time.Now().Add(5 * time.Second)
	for time.Now().Before(end) {
		r, e := s.LoginStatus(id)
		if e != nil {
			t.Fatal(e)
		}
		done := r.Status == "succeeded" || r.Status == "failed" || r.Status == "cancelled"
		if terminal && done || !terminal && r.UserCode != "" {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("login timeout")
	return LoginRecord{}
}
func TestProductionLoginCapturesAndUpdatesExisting(t *testing.T) {
	s := productionFixture(t)
	live := filepath.Join(s.Store.Root, "demo-win", "auth.json")
	before, _ := os.ReadFile(live)
	r, e := s.StartLogin("新备注", "")
	if e != nil {
		t.Fatal(e)
	}
	waiting := awaitLogin(t, s, r.ID, false)
	if waiting.VerificationURL == "" {
		t.Fatal("device URL missing")
	}
	persisted, _ := os.ReadFile(filepath.Join(s.loginsDir(), r.ID+".json"))
	if strings.Contains(string(persisted), "ABCD-EFGHI") {
		t.Fatal("device code persisted")
	}
	done := awaitLogin(t, s, r.ID, true)
	if done.Status != "succeeded" || !done.UpdatedExisting || done.Slot != "biz1" {
		t.Fatalf("%+v", done)
	}
	if len(s.Store.Names()) != 3 {
		t.Fatal("duplicate account")
	}
	after, _ := os.ReadFile(live)
	if string(before) != string(after) {
		t.Fatal("live credentials changed during login")
	}
	a, _ := s.AccountDetail("biz1")
	if a.DisplayName != "新备注" {
		t.Fatal(a)
	}
}
func TestCancelLoginCannotLaterCommit(t *testing.T) {
	s := productionFixture(t)
	before, _ := s.Store.Get("biz1")
	r, e := s.StartLogin("cancel", "")
	if e != nil {
		t.Fatal(e)
	}
	awaitLogin(t, s, r.ID, false)
	if _, e = s.CancelLogin(r.ID); e != nil {
		t.Fatal(e)
	}
	time.Sleep(350 * time.Millisecond)
	done, _ := s.LoginStatus(r.ID)
	after, _ := s.Store.Get("biz1")
	if done.Status != "cancelled" || refreshOf(before.Auth) != refreshOf(after.Auth) {
		t.Fatal("cancelled login committed")
	}
}
func TestUpdateDifferentIdentityDoesNotReplace(t *testing.T) {
	s := productionFixture(t)
	before, _ := s.Store.Get("plus")
	r, e := s.StartLogin("wrong", "plus")
	if e != nil {
		t.Fatal(e)
	}
	done := awaitLogin(t, s, r.ID, true)
	after, _ := s.Store.Get("plus")
	if !done.IdentityMismatch || refreshOf(before.Auth) != refreshOf(after.Auth) {
		t.Fatal(done)
	}
}
