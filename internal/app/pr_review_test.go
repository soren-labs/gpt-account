package app

import (
	"github.com/soren-labs/gpt-account/internal/gpa"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReviewWebRespectsStoreLock(t *testing.T) {
	s := demoSvc(t)
	p, err := s.Preview("biz1", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := gpa.AcquireLock(s.Store.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	e, err := s.Submit(p.ID, "lock-req", "lock-idem", "agent")
	if err == nil && e.Status == "succeeded" {
		t.Fatal("web changed credentials while another writer holds account-store lock")
	}
}

func TestReviewRealProbePreservesAppRestartReason(t *testing.T) {
	s := demoSvc(t)
	s.Demo = false
	s.Probe = nil
	t.Setenv("GPA_CHATGPT", "auto")
	t.Setenv("GPA_FAKE_APP", "running")
	t.Setenv("GPA_IGNORE_CLI", "1")
	plan, err := s.Preview("biz1", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "waiting_user" || plan.ReasonCode != "APP_RESTART_REQUIRED" {
		t.Fatalf("real probe lost restart reason: %+v", plan)
	}
}

func TestReviewNoopWhenAlreadyCurrentEvenIfAppRunning(t *testing.T) {
	s := demoSvc(t)
	s.Demo = false
	s.Probe = nil
	t.Setenv("GPA_CHATGPT", "auto")
	t.Setenv("GPA_FAKE_APP", "running")
	t.Setenv("GPA_IGNORE_CLI", "1")
	// The demo seed writes the plus credentials to both client files, so
	// switching desktop to plus must be a no-op and must not ask for a restart.
	plan, err := s.Preview("plus", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "noop" || plan.ReasonCode != "" || !plan.AlreadyCurrent {
		t.Fatalf("already-current seat asked for restart: %+v", plan)
	}
	env, err := s.Submit(plan.ID, "noop-req", "noop-idem", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if env.Status != "succeeded" {
		t.Fatalf("noop submit should succeed without touching the App: %+v", env)
	}
}

func TestReviewConcurrentIdempotency(t *testing.T) {
	s := demoSvc(t)
	p, err := s.Preview("biz1", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	s.Switch = func(*gpa.Store, string, string, bool) gpa.Result {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return gpa.Result{Status: "completed"}
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.Submit(p.ID, "same-request", "same-key", "agent") }()
	}
	<-entered
	select {
	case <-entered:
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("same request executed %d times", calls.Load())
	}
}

func TestReviewRestartMustNotOverwriteConflict(t *testing.T) {
	s := demoSvc(t)
	acct, err := s.Store.Get("biz1")
	if err != nil {
		t.Fatal(err)
	}
	// Same identity and timestamp; a different refresh credential must block.
	acct.Auth["tokens"].(map[string]any)["refresh_token"] = "conflicting-live-token"
	if err := writeJSON(filepath.Join(s.Store.Root, "demo-win", "auth.json"), acct.Auth); err != nil {
		t.Fatal(err)
	}
	s.Probe = func(c gpa.Client) gpa.ClientState {
		if c.Kind == "app" {
			return gpa.ClientState{Client: c, Process: gpa.ProcUnknown, Presence: gpa.PresenceRunning, ReasonCode: "APP_RUNNING"}
		}
		return gpa.ClientState{Client: c, Process: gpa.ProcNone, Presence: gpa.PresenceNone}
	}
	p, err := s.Preview("biz1", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.Submit(p.ID, "restart-req", "restart-key", "ui")
	if err != nil {
		t.Fatal(err)
	}
	e, err = s.Confirm(e.OperationID, "confirm-req", "ui")
	if err == nil && e.Status == "succeeded" {
		t.Fatal("restart confirmation bypassed conflicting refresh-token protection")
	}
}

func TestReviewRetryExpiredPending(t *testing.T) {
	s := demoSvc(t)
	s.Probe = func(c gpa.Client) gpa.ClientState {
		return gpa.ClientState{Client: c, Process: gpa.ProcRunning, Presence: gpa.PresenceRunning, ReasonCode: "CLI_RUNNING"}
	}
	p, err := s.Preview("biz1", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.Submit(p.ID, "wait-req", "wait-key", "agent")
	if err != nil {
		t.Fatal(err)
	}
	p.ExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339)
	s.savePlan(p)
	s.Probe = func(c gpa.Client) gpa.ClientState {
		return gpa.ClientState{Client: c, Process: gpa.ProcNone, Presence: gpa.PresenceNone}
	}
	retry, err := s.Retry(e.OperationID, "retry-req", "retry-key", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if retry.ReasonCode == "PLAN_STALE" {
		t.Fatal("retry cannot resume pending operation after its five-minute preview expires")
	}
}

func TestReviewFailedOperationPersistenceMustPreventWrite(t *testing.T) {
	s := demoSvc(t)
	p, err := s.Preview("biz1", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(s.opsDir())
	if err := os.WriteFile(s.opsDir(), []byte("block-directory"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	s.Switch = func(*gpa.Store, string, string, bool) gpa.Result {
		called = true
		return gpa.Result{Status: "completed"}
	}
	e, err := s.Submit(p.ID, "disk-req", "disk-key", "agent")
	if called {
		t.Fatalf("executed switch without durable operation record: status=%s err=%v", e.Status, err)
	}
}
