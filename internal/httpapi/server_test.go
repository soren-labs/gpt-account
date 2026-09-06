package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soren-labs/gpt-account/internal/app"
	"github.com/soren-labs/gpt-account/internal/gpa"
)

func testServer(t *testing.T) (*Server, *http.Client, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GPA_STORE", root)
	t.Setenv("GPA_CODEX_HOME", filepath.Join(root, "demo-wsl"))
	t.Setenv("GPA_WINDOWS_CODEX", filepath.Join(root, "demo-win"))
	t.Setenv("GPA_CHATGPT", "off")
	t.Setenv("GPA_IGNORE_CLI", "1")
	store := gpa.OpenStore(gpa.LoadConfig(root))
	_ = store.Ensure()
	_ = app.SeedDemo(store)
	svc := app.New(store)
	svc.Demo = true
	svc.Probe = func(c gpa.Client) gpa.ClientState {
		return gpa.ClientState{Client: c, Process: gpa.ProcNone, Presence: gpa.PresenceNone}
	}
	svc.Switch = func(store *gpa.Store, name, target string, restart bool) gpa.Result {
		return gpa.Result{Status: "completed"}
	}
	s := New(svc)
	s.TestSession = "demo-boot"
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts.Client(), ts.URL
}

func openSession(t *testing.T, client *http.Client, base string) (cookie, csrf string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"bootstrap": "demo-boot"})
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1"
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	csrf, _ = out["csrf"].(string)
	for _, c := range res.Cookies() {
		if c.Name == "gpa_session" {
			cookie = c.Value
		}
	}
	if cookie == "" || csrf == "" {
		t.Fatalf("session %+v cookie=%s", out, cookie)
	}
	return cookie, csrf
}

func TestRejectsBadOrigin(t *testing.T) {
	_, client, base := testServer(t)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/switch-plans", strings.NewReader(`{"account":"plus","target":"desktop"}`))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatal(res.StatusCode)
	}
}

func TestWebSwitchAndAgentCannotConfirm(t *testing.T) {
	s, client, base := testServer(t)
	cookie, csrf := openSession(t, client, base)
	body, _ := json.Marshal(map[string]string{"account": "biz2", "target": "desktop"})
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/switch-plans", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Cookie", "gpa_session="+cookie)
	req.Host = "127.0.0.1"
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	var plan map[string]any
	_ = json.NewDecoder(res.Body).Decode(&plan)
	res.Body.Close()
	opBody, _ := json.Marshal(map[string]string{"plan_id": plan["id"].(string), "request_id": "r1", "idempotency_key": "k1"})
	req, _ = http.NewRequest(http.MethodPost, base+"/api/v1/operations", bytes.NewReader(opBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Cookie", "gpa_session="+cookie)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode >= 300 {
		t.Fatal(res.Status)
	}
	var env map[string]any
	_ = json.NewDecoder(res.Body).Decode(&env)
	res.Body.Close()
	if env["status"] != "succeeded" {
		t.Fatal(env)
	}
	req, _ = http.NewRequest(http.MethodPost, base+"/api/v1/operations/nope/confirm", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+s.AgentToken)
	req.Header.Set("Content-Type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode == 200 {
		t.Fatal("agent confirm accepted")
	}
}

func TestIndexHTML(t *testing.T) {
	_, client, base := testServer(t)
	res, err := client.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	raw.ReadFrom(res.Body)
	if res.StatusCode != 200 || !strings.Contains(raw.String(), "GPA 账号管理") {
		t.Fatalf("%d %s", res.StatusCode, raw.String())
	}
}

func TestAccountsOmitTokens(t *testing.T) {
	_, client, base := testServer(t)
	cookie, csrf := openSession(t, client, base)
	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/accounts", nil)
	req.Header.Set("Cookie", "gpa_session="+cookie)
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	raw.ReadFrom(res.Body)
	if strings.Contains(raw.String(), "refresh_token") || strings.Contains(raw.String(), "plus-refresh") {
		t.Fatal(raw.String())
	}
}

func TestOpenUIReturnsOneTimeBrowserURL(t *testing.T) {
	s, client, base := testServer(t)
	var opened string
	s.OpenBrowser = func(url string) { opened = url }
	cookie, csrf := openSession(t, client, base)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/open-ui", strings.NewReader(`{"operation_id":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "gpa_session="+cookie)
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || opened == "" || out["url"] != opened || !strings.Contains(opened, "#bootstrap=") {
		t.Fatalf("status=%d opened=%q response=%v", res.StatusCode, opened, out)
	}
}

func TestSessionSurvivesReload(t *testing.T) {
	_, client, base := testServer(t)
	cookie, csrf := openSession(t, client, base)
	req, _ := http.NewRequest("GET", base+"/api/v1/session", nil)
	req.Header.Set("Cookie", "gpa_session="+cookie)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != 200 || out["csrf"] != csrf {
		t.Fatal(res.Status, out)
	}
}
func TestRejectsLoopbackPrefixAndWrongPort(t *testing.T) {
	_, client, base := testServer(t)
	for _, origin := range []string{"http://localhost.evil.example", "http://127.0.0.1.evil.example", "http://127.0.0.1:1"} {
		req, _ := http.NewRequest("POST", base+"/api/v1/session", strings.NewReader(`{"bootstrap":"demo-boot"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal(origin, res.StatusCode)
		}
	}
}
func TestMalformedConfirmRejectedBeforeAction(t *testing.T) {
	s, client, base := testServer(t)
	req, _ := http.NewRequest("POST", base+"/api/v1/operations/id/confirm", strings.NewReader(`{"force":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.AgentToken)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal("error is not JSON", err)
	}
	if res.StatusCode != 400 {
		t.Fatal(res.Status)
	}
}
