package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/soren-labs/gpt-account/internal/app"
	"github.com/soren-labs/gpt-account/web"
)

type Server struct {
	Svc             *app.Service
	Bootstrap       string
	AgentToken      string
	TestSession     string
	OpenBrowser     func(string)
	BaseURL         string
	bootstrapTokens map[string]time.Time
	InstanceID      string

	mu       sync.Mutex
	sessions map[string]session
	usedBoot bool
}

type session struct {
	csrf   string
	expiry time.Time
}

func New(svc *app.Service) *Server {
	return &Server{
		Svc:             svc,
		Bootstrap:       tokenHex(16),
		AgentToken:      tokenHex(16),
		InstanceID:      tokenHex(8),
		sessions:        map[string]session{},
		bootstrapTokens: map[string]time.Time{},
	}
}

func tokenHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/session", s.session)
	mux.HandleFunc("/api/v1/status", s.auth(s.status))
	mux.HandleFunc("/api/v1/accounts", s.auth(s.accounts))
	mux.HandleFunc("/api/v1/accounts/", s.auth(s.accountItem))
	mux.HandleFunc("/api/v1/imports/preview", s.auth(s.importPreview))
	mux.HandleFunc("/api/v1/imports", s.auth(s.importApply))
	mux.HandleFunc("/api/v1/logins", s.auth(s.logins))
	mux.HandleFunc("/api/v1/logins/", s.auth(s.loginItem))
	mux.HandleFunc("/api/v1/switch-plans", s.auth(s.plans))
	mux.HandleFunc("/api/v1/operations", s.auth(s.operations))
	mux.HandleFunc("/api/v1/operations/", s.auth(s.operationItem))
	mux.HandleFunc("/api/v1/diagnostics", s.auth(s.diagnostics))
	mux.HandleFunc("/api/v1/health", s.health)
	mux.HandleFunc("/api/v1/open-ui", s.auth(s.openUI))
	mux.HandleFunc("/", s.static)
	return s.guard(mux)
}

func (s *Server) Listen(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/api/") && !strings.Contains(r.URL.Path, ".") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if host != "127.0.0.1" && host != "localhost" && host != "::1" {
				http.Error(w, `{"error":"bad host"}`, http.StatusForbidden)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				origin := r.Header.Get("Origin")
				if origin != "" && (!localOrigin(origin) || origin != "http://"+r.Host) {
					http.Error(w, `{"error":"bad origin"}`, http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func localOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "http" && u.User == nil && u.Path == "" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")
}
func (s *Server) BrowserURL(operation string) string {
	token := tokenHex(24)
	s.mu.Lock()
	for k, t := range s.bootstrapTokens {
		if time.Now().After(t) {
			delete(s.bootstrapTokens, k)
		}
	}
	s.bootstrapTokens[token] = time.Now().Add(2 * time.Minute)
	s.mu.Unlock()
	link := s.BaseURL + "/#bootstrap=" + token
	if operation != "" {
		link += "&operation=" + url.QueryEscape(operation)
	}
	return link
}
func (s *Server) openUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	var body struct {
		OperationID string `json:"operation_id"`
	}
	if err := decodeStrict(r, &body); err != nil {
		fail(w, err)
		return
	}
	if body.OperationID != "" {
		if _, err := s.Svc.GetOp(body.OperationID); err != nil {
			fail(w, err)
			return
		}
	}
	if s.OpenBrowser == nil {
		fail(w, fmt.Errorf("browser launcher unavailable"))
		return
	}
	link := s.BrowserURL(body.OperationID)
	s.OpenBrowser(link)
	writeJSON(w, 200, map[string]any{"status": "ok", "url": link, "operation_id": body.OperationID})
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		data, err := web.FS.ReadFile("index.html")
		if err != nil {
			http.Error(w, "missing page", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
		return
	}
	http.FileServer(http.FS(web.FS)).ServeHTTP(w, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "instance": s.InstanceID, "demo": s.Svc.Demo})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if s.actorOf(r) != "ui" {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		c, _ := r.Cookie("gpa_session")
		s.mu.Lock()
		sess := s.sessions[c.Value]
		s.mu.Unlock()
		writeJSON(w, 200, map[string]any{"csrf": sess.csrf, "demo": s.Svc.Demo})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method"}`, 405)
		return
	}
	var body struct {
		Bootstrap string `json:"bootstrap"`
	}
	if err := decodeStrict(r, &body); err != nil {
		fail(w, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, found := s.bootstrapTokens[body.Bootstrap]
	ok := found && time.Now().Before(expiry)
	if body.Bootstrap != "" && body.Bootstrap == s.Bootstrap && !s.usedBoot {
		ok = true
		s.usedBoot = true
	}
	if s.TestSession != "" && body.Bootstrap == s.TestSession {
		ok = true
	}
	if !ok {
		http.Error(w, `{"error":"invalid bootstrap"}`, 401)
		return
	}
	delete(s.bootstrapTokens, body.Bootstrap)
	sid, csrf := tokenHex(16), tokenHex(16)
	s.sessions[sid] = session{csrf: csrf, expiry: time.Now().Add(12 * time.Hour)}
	http.SetCookie(w, &http.Cookie{Name: "gpa_session", Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]any{"csrf": csrf, "demo": s.Svc.Demo})
}

type ctxKey int

func (s *Server) auth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := s.actorOf(r)
		if actor == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && actor == "ui" {
			csrf := r.Header.Get("X-CSRF-Token")
			if !s.validCSRF(r, csrf) {
				http.Error(w, `{"error":"csrf"}`, http.StatusForbidden)
				return
			}
		}
		r.Header.Set("X-GPA-Actor", actor)
		fn(w, r)
	}
}

func (s *Server) actorOf(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if strings.TrimPrefix(auth, "Bearer ") == s.AgentToken {
			return "agent"
		}
	}
	c, err := r.Cookie("gpa_session")
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	if !ok || time.Now().After(sess.expiry) {
		return ""
	}
	return "ui"
}

func (s *Server) validCSRF(r *http.Request, csrf string) bool {
	c, err := r.Cookie("gpa_session")
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	return ok && csrf != "" && csrf == sess.csrf
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	selected := r.URL.Query().Get("target")
	view, err := s.Svc.Status(selected)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
		return
	}
	view, err := s.Svc.Status("")
	if err != nil {
		fail(w, err)
		return
	}
	if r.URL.Query().Get("archived") == "true" {
		view.Accounts = s.Svc.ArchivedAccounts()
	}
	writeJSON(w, 200, map[string]any{"accounts": view.Accounts})
}

func (s *Server) accountItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/accounts/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		view, err := s.Svc.AccountDetail(id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, view)
	case http.MethodPatch:
		var body struct {
			DisplayName *string `json:"display_name"`
			Archived    *bool   `json:"archived"`
		}
		if err := decodeStrict(r, &body); err != nil {
			fail(w, err)
			return
		}
		name := ""
		if body.DisplayName != nil {
			name = *body.DisplayName
		}
		view, err := s.Svc.PatchAccount(id, name, body.Archived)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, view)
	default:
		http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) importPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
		return
	}
	prev, err := s.Svc.ImportPreview()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, prev)
}

func (s *Server) importApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
		return
	}
	res, err := s.Svc.ImportApply()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) logins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		Account     string `json:"account"`
	}
	if err := decodeStrict(r, &body); err != nil {
		fail(w, err)
		return
	}
	rec, err := s.Svc.StartLogin(body.DisplayName, body.Account)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 202, rec)
}

func (s *Server) loginItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/logins/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		rec, err := s.Svc.CancelLogin(id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, rec)
		return
	}
	if r.Method == http.MethodGet {
		rec, err := s.Svc.LoginStatus(id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, rec)
		return
	}
	http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
}

func (s *Server) plans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Account string `json:"account"`
		Target  string `json:"target"`
	}
	if err := decodeStrict(r, &body); err != nil {
		fail(w, err)
		return
	}
	if body.Account == "" || body.Target == "" {
		http.Error(w, `{"error":"account and target are required"}`, http.StatusBadRequest)
		return
	}
	plan, err := s.Svc.Preview(body.Account, body.Target)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, plan)
}

func (s *Server) operations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{"operations": s.Svc.ListOps(20)})
	case http.MethodPost:
		var body struct {
			PlanID         string `json:"plan_id"`
			RequestID      string `json:"request_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := decodeStrict(r, &body); err != nil {
			fail(w, err)
			return
		}
		if body.PlanID == "" {
			http.Error(w, `{"error":"plan_id is required"}`, http.StatusBadRequest)
			return
		}
		env, err := s.Svc.Submit(body.PlanID, body.RequestID, body.IdempotencyKey, r.Header.Get("X-GPA-Actor"))
		if err != nil {
			fail(w, err)
			return
		}
		code := 200
		if env.Status == "waiting_user" || env.Status == "running" || env.Status == "queued" {
			code = 202
		}
		writeJSON(w, code, env)
	default:
		http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) operationItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet && len(parts) == 1 {
		op, err := s.Svc.GetOp(id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, op)
		return
	}
	if r.Method == http.MethodPost && len(parts) == 2 {
		var body struct {
			RequestID      string `json:"request_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := decodeStrict(r, &body); err != nil {
			fail(w, err)
			return
		}
		var env app.Envelope
		var err error
		switch parts[1] {
		case "confirm":
			env, err = s.Svc.Confirm(id, body.RequestID, r.Header.Get("X-GPA-Actor"))
		case "retry":
			env, err = s.Svc.Retry(id, body.RequestID, body.IdempotencyKey, r.Header.Get("X-GPA-Actor"))
		case "cancel":
			op, e2 := s.Svc.Cancel(id)
			if e2 != nil {
				fail(w, e2)
				return
			}
			writeJSON(w, 200, op)
			return

		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, env)
		return
	}
	http.Error(w, `{"error":"method"}`, http.StatusMethodNotAllowed)
}

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Svc.Diagnostics())
}

func fail(w http.ResponseWriter, err error) {
	writeJSON(w, 400, map[string]any{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func decodeStrict(r *http.Request, dest any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("Content-Type must be application/json")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}
