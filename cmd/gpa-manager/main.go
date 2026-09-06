package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/soren-labs/gpt-account/internal/app"
	"github.com/soren-labs/gpt-account/internal/gpa"
	"github.com/soren-labs/gpt-account/internal/httpapi"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("gpa-manager", flag.ContinueOnError)
	store := fs.String("store", "", "account store")
	listen := fs.String("listen", "127.0.0.1:0", "listen address")
	noBrowser := fs.Bool("no-browser", false, "do not open a browser")
	demo := fs.Bool("demo", false, "seed isolated demo accounts")
	agentStdio := fs.Bool("agent-stdio", false, "JSON stdio bridge")
	background := fs.Bool("background", false, "start without opening the browser")
	testSession := fs.String("test-session", "", "test-only bootstrap (refuses the default store)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *background {
		*noBrowser = true
	}
	if *demo && *store == "" {
		*store = filepath.Join(os.TempDir(), "gpa-demo")
	}
	cfg := gpa.LoadConfig(*store)
	if *demo {
		os.Setenv("GPA_STORE", cfg.Store)
		os.Setenv("GPA_CODEX_HOME", filepath.Join(cfg.Store, "demo-wsl"))
		os.Setenv("GPA_WINDOWS_CODEX", filepath.Join(cfg.Store, "demo-win"))
		os.Setenv("GPA_CHATGPT", "off")
		os.Setenv("GPA_IGNORE_CLI", "1")
		cfg = gpa.LoadConfig(cfg.Store)
	}
	st := gpa.OpenStore(cfg)
	if err := st.Ensure(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *testSession != "" && samePath(cfg.Store, gpa.LoadConfig("").Store) && !*demo {
		fmt.Fprintln(os.Stderr, "refusing --test-session on the default account store")
		return 2
	}
	svc := app.New(st)
	svc.Demo = *demo
	if *demo {
		if err := app.SeedDemo(st); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		svc.Probe = func(c gpa.Client) gpa.ClientState {
			return gpa.ClientState{Client: c, Process: gpa.ProcNone, Presence: gpa.PresenceNone}
		}
	}
	srv := httpapi.New(svc)
	srv.TestSession = *testSession
	if tok, err := loadOrCreate(filepath.Join(st.Root, "agent.token"), srv.AgentToken); err == nil {
		srv.AgentToken = tok
	}
	if *agentStdio {
		return runStdio(srv)
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	addr := ln.Addr().String()
	host, port, _ := strings.Cut(addr, ":")
	if host == "" {
		host = "127.0.0.1"
	}
	url := fmt.Sprintf("http://127.0.0.1:%s/#bootstrap=%s", port, srv.Bootstrap)
	_ = writeRuntime(st.Root, map[string]any{
		"instance": srv.InstanceID,
		"port":     port,
		"pid":      os.Getpid(),
		"exe":      os.Args[0],
		"url":      "http://127.0.0.1:" + port + "/",
	})
	fmt.Println("GPA Manager", url)
	go http.Serve(ln, srv.Handler())
	if !*noBrowser {
		openBrowser("http://127.0.0.1:" + port + "/#bootstrap=" + srv.Bootstrap)
	}
	select {}
}

func runStdio(srv *httpapi.Server) int {
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		var req map[string]any
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return 0
			}
			enc.Encode(map[string]any{"error": err.Error()})
			return 5
		}
		method, _ := req["method"].(string)
		path, _ := req["path"].(string)
		body, _ := req["body"].(string)
		if method == "" || path == "" {
			enc.Encode(map[string]any{"error": "method and path required"})
			continue
		}
		httpReq, err := http.NewRequest(method, "http://bridge"+path, strings.NewReader(body))
		if err != nil {
			enc.Encode(map[string]any{"error": err.Error()})
			continue
		}
		httpReq.Host = "127.0.0.1"
		httpReq.Header.Set("Authorization", "Bearer "+srv.AgentToken)
		httpReq.Header.Set("Content-Type", "application/json")
		rec := newRecorder()
		srv.Handler().ServeHTTP(rec, httpReq)
		enc.Encode(map[string]any{"status": rec.code, "body": rec.buf.String()})
	}
}

type recorder struct {
	code   int
	header http.Header
	buf    strings.Builder
}

func newRecorder() *recorder { return &recorder{code: 200, header: http.Header{}} }
func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) Write(b []byte) (int, error) { return r.buf.Write(b) }
func (r *recorder) WriteHeader(code int) { r.code = code }

func loadOrCreate(path, fallback string) (string, error) {
	if raw, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(raw))
		if s != "" {
			return s, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fallback, err
	}
	return fallback, os.WriteFile(path, []byte(fallback+"\n"), 0o600)
}

func writeRuntime(root string, data map[string]any) error {
	raw, _ := json.MarshalIndent(data, "", "  ")
	return os.WriteFile(filepath.Join(root, "runtime.json"), append(raw, '\n'), 0o600)
}

func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa != "" && aa == bb
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case os.Getenv("WSL_DISTRO_NAME") != "":
		cmd = exec.Command("explorer.exe", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
