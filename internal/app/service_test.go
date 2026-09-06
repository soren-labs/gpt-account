package app

import (
	"path/filepath"
	"testing"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

func demoSvc(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GPA_STORE", root)
	t.Setenv("GPA_CODEX_HOME", filepath.Join(root, "demo-wsl"))
	t.Setenv("GPA_WINDOWS_CODEX", filepath.Join(root, "demo-win"))
	t.Setenv("GPA_CHATGPT", "off")
	t.Setenv("GPA_IGNORE_CLI", "1")
	cfg := gpa.LoadConfig(root)
	store := gpa.OpenStore(cfg)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := SeedDemo(store); err != nil {
		t.Fatal(err)
	}
	svc := New(store)
	svc.Demo = true
	svc.Probe = func(c gpa.Client) gpa.ClientState {
		return gpa.ClientState{Client: c, Process: gpa.ProcNone, Presence: gpa.PresenceNone}
	}
	return svc
}

func TestStatusListsThreeSeats(t *testing.T) {
	svc := demoSvc(t)
	st, err := svc.Status("desktop")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Accounts) != 3 {
		t.Fatalf("accounts=%d", len(st.Accounts))
	}
	if st.SelectedTarget != "desktop" {
		t.Fatal(st.SelectedTarget)
	}
	ids := map[string]bool{}
	for _, a := range st.Accounts {
		ids[a.ID] = true
		if a.Email != "" {
			t.Fatal("list leaked full email")
		}
	}
	if len(ids) != 3 {
		t.Fatal("stable ids collapsed")
	}
}

func TestStatusOpensForEmptyInstallation(t *testing.T) {
	root := t.TempDir()
	store := gpa.OpenStore(gpa.LoadConfig(root))
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	view, err := New(store).Status("")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Accounts) != 0 {
		t.Fatalf("unexpected empty status: %+v", view)
	}
}

func TestPreviewAndIdempotentSwitch(t *testing.T) {
	svc := demoSvc(t)
	wrote := 0
	svc.Switch = func(store *gpa.Store, name, target string, restart bool) gpa.Result {
		wrote++
		return gpa.Result{Status: "completed", Written: []string{"demo"}}
	}
	plan, err := svc.Preview("biz1", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "ready" {
		t.Fatal(plan)
	}
	a, err := svc.Submit(plan.ID, "req-1", "idem-1", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "succeeded" {
		t.Fatal(a)
	}
	b, err := svc.Submit(plan.ID, "req-1", "idem-1", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if b.OperationID != a.OperationID || wrote != 1 {
		t.Fatalf("not idempotent: %+v wrote=%d", b, wrote)
	}
}

func TestAgentCannotConfirmRestart(t *testing.T) {
	svc := demoSvc(t)
	svc.Probe = func(c gpa.Client) gpa.ClientState {
		if c.Kind == "app" {
			return gpa.ClientState{Client: c, Process: gpa.ProcUnknown, Presence: gpa.PresenceRunning, ReasonCode: "APP_RUNNING"}
		}
		return gpa.ClientState{Client: c, Process: gpa.ProcNone, Presence: gpa.PresenceNone}
	}
	plan, err := svc.Preview("plus", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	env, err := svc.Submit(plan.ID, "req-app", "idem-app", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if env.Status != "waiting_user" || env.ReasonCode != "APP_RESTART_REQUIRED" {
		t.Fatal(env)
	}
	_, err = svc.Confirm(env.OperationID, "req-app", "agent")
	if err == nil {
		t.Fatal("agent confirmed restart")
	}
}

func TestQueryFailureBlocks(t *testing.T) {
	svc := demoSvc(t)
	svc.Probe = func(c gpa.Client) gpa.ClientState {
		return gpa.ClientState{Client: c, Process: gpa.ProcUnknown, Presence: gpa.PresenceUnknown, ReasonCode: "QUERY_FAILED"}
	}
	plan, err := svc.Preview("plus", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "blocked" {
		t.Fatal(plan)
	}
}

func TestRenameDoesNotChangeID(t *testing.T) {
	svc := demoSvc(t)
	before, err := svc.AccountDetail("biz1")
	if err != nil {
		t.Fatal(err)
	}
	after, err := svc.PatchAccount("biz1", "工作席位", nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID || after.Slot != "biz1" {
		t.Fatal(after)
	}
}

func TestStableIDsSurviveRemigrate(t *testing.T) {
	svc := demoSvc(t)
	a, _ := svc.AccountDetail("plus")
	if err := svc.Store.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	b, _ := svc.AccountDetail("plus")
	if a.ID == "" || a.ID != b.ID {
		t.Fatalf("%s vs %s", a.ID, b.ID)
	}
}
