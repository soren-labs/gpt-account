package main

import (
	"bytes"
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
	"time"

	"github.com/soren-labs/gpt-account/internal/app"
	"github.com/soren-labs/gpt-account/internal/gpa"
	"github.com/soren-labs/gpt-account/internal/httpapi"
)

func main() { os.Exit(run(os.Args[1:])) }
func run(args []string) int {
	fs := flag.NewFlagSet("gpa-manager", flag.ContinueOnError)
	root := fs.String("store", "", "account store")
	listen := fs.String("listen", "127.0.0.1:0", "loopback listen address")
	noBrowser := fs.Bool("no-browser", false, "do not open browser")
	background := fs.Bool("background", false, "run background host")
	demo := fs.Bool("demo", false, "isolated demo")
	stdio := fs.Bool("agent-stdio", false, "forward one JSON request to host")
	testSession := fs.String("test-session", "", "isolated demo bootstrap")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) != 0 {
		return 2
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || host != "127.0.0.1" {
		fmt.Fprintln(os.Stderr, "only 127.0.0.1 listeners are allowed")
		return 2
	}
	if *demo && *root == "" {
		*root = filepath.Join(os.TempDir(), "gpa-demo")
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(*root, "/mnt/") && len(*root) > 7 {
		*root = strings.ToUpper((*root)[5:6]) + ":" + strings.ReplaceAll((*root)[6:], "/", `\`)
	}
	if runtime.GOOS != "windows" && len(*root) > 3 && (*root)[1] == ':' && ((*root)[2] == '\\' || (*root)[2] == '/') {
		// A Windows-style store path handed to the Linux/WSL binary must map to
		// /mnt/<drive>/..., never be created literally under the current directory.
		*root = "/mnt/" + strings.ToLower((*root)[0:1]) + strings.ReplaceAll((*root)[2:], `\`, "/")
	}
	cfg := gpa.LoadConfig(*root)
	if *stdio {
		return runStdio(cfg.Store)
	}
	if *testSession != "" && !*demo {
		fmt.Fprintln(os.Stderr, "--test-session requires an isolated demo")
		return 2
	}
	if err := os.MkdirAll(cfg.Store, 0700); err != nil {
		return 1
	}
	lock, err := gpa.AcquireLock(filepath.Join(cfg.Store, "manager.lock"))
	if err != nil {
		rt, e := waitRuntime(cfg.Store, 5*time.Second)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		if !*background && !*noBrowser {
			_, _, e = callHost(cfg.Store, rt, "POST", "/api/v1/open-ui", `{}`)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 1
			}
		}
		return 0
	}
	defer lock.Release()
	if *demo {
		marker := filepath.Join(cfg.Store, ".gpa-demo")
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			if entries, _ := os.ReadDir(filepath.Join(cfg.Store, "accounts")); len(entries) > 0 {
				fmt.Fprintln(os.Stderr, "refusing demo over an existing account store")
				return 2
			}
			if err := os.WriteFile(marker, []byte("demo\n"), 0600); err != nil {
				return 1
			}
		}
		os.Setenv("GPA_STORE", cfg.Store)
		os.Setenv("GPA_CODEX_HOME", filepath.Join(cfg.Store, "demo-wsl"))
		os.Setenv("GPA_WINDOWS_CODEX", filepath.Join(cfg.Store, "demo-win"))
		os.Setenv("GPA_CHATGPT", "off")
		os.Setenv("GPA_IGNORE_CLI", "1")
		cfg = gpa.LoadConfig(cfg.Store)
	} else {
		for _, key := range []string{"GPA_FAKE_APP", "GPA_FAKE_CLI", "GPA_IGNORE_CLI", "GPA_PROC_QUERY", "GPA_LOCK_REMOTE"} {
			os.Unsetenv(key)
		}
	}
	st := gpa.OpenStore(cfg)
	storeLock, err := gpa.AcquireLock(st.LockPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = st.Ensure()
	if err == nil && *demo && len(st.Names()) == 0 {
		err = app.SeedDemo(st)
	}
	storeLock.Release()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	svc := app.New(st)
	svc.Demo = *demo
	if *demo {
		svc.Probe = func(c gpa.Client) gpa.ClientState {
			return gpa.ClientState{Client: c, Process: gpa.ProcNone, Presence: gpa.PresenceNone}
		}
	}
	if err := svc.Recover(); err != nil {
		fmt.Fprintln(os.Stderr, "recovery:", err)
		return 1
	}
	srv := httpapi.New(svc)
	srv.TestSession = *testSession
	srv.OpenBrowser = openBrowser
	srv.AgentToken, err = loadOrCreate(filepath.Join(st.Root, "agent.token"), srv.AgentToken)
	if err != nil {
		return 1
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	srv.BaseURL = "http://127.0.0.1:" + port
	exe, _ := os.Executable()
	rt := runtimeRecord{Instance: srv.InstanceID, Port: port, PID: os.Getpid(), Exe: exe, URL: srv.BaseURL + "/"}
	if err := writeRuntime(st.Root, rt); err != nil {
		return 1
	}
	httpServer := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if !*background && !*noBrowser {
		go openBrowser(srv.BrowserURL(""))
	}
	fmt.Fprintln(os.Stderr, "GPA Manager", srv.BaseURL)
	err = httpServer.Serve(ln)
	if err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

type runtimeRecord struct {
	Instance string `json:"instance"`
	Port     string `json:"port"`
	PID      int    `json:"pid"`
	Exe      string `json:"exe"`
	URL      string `json:"url"`
}

func loadRuntime(root string) (runtimeRecord, error) {
	var rt runtimeRecord
	raw, err := os.ReadFile(filepath.Join(root, "runtime.json"))
	if err != nil {
		return rt, err
	}
	err = json.Unmarshal(raw, &rt)
	if err != nil {
		return rt, err
	}
	c := &http.Client{Timeout: time.Second}
	res, err := c.Get("http://127.0.0.1:" + rt.Port + "/api/v1/health")
	if err != nil {
		return rt, err
	}
	defer res.Body.Close()
	var health struct {
		Instance string `json:"instance"`
	}
	err = json.NewDecoder(res.Body).Decode(&health)
	if err != nil || rt.Instance == "" || health.Instance != rt.Instance {
		return rt, fmt.Errorf("GPA host identity mismatch")
	}
	return rt, nil
}
func waitRuntime(root string, timeout time.Duration) (runtimeRecord, error) {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if rt, err := loadRuntime(root); err == nil {
			return rt, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return runtimeRecord{}, fmt.Errorf("GPA host unavailable")
}
func callHost(root string, rt runtimeRecord, method, path, body string) (int, string, error) {
	if !strings.HasPrefix(path, "/api/v1/") || strings.ContainsAny(path, "\r\n") {
		return 0, "", fmt.Errorf("invalid API path")
	}
	token, err := os.ReadFile(filepath.Join(root, "agent.token"))
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequest(method, "http://127.0.0.1:"+rt.Port+path, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	return res.StatusCode, string(raw), err
}
func runStdio(root string) int {
	enc := json.NewEncoder(os.Stdout)
	var req struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   string `json:"body"`
	}
	dec := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		enc.Encode(map[string]any{"error": err.Error()})
		return 5
	}
	rt, err := loadRuntime(root)
	if err != nil {
		self, _ := os.Executable()
		cmd := exec.Command(self, "--background", "--store", root)
		detach(cmd)
		if err = cmd.Start(); err == nil {
			cmd.Process.Release()
			rt, err = waitRuntime(root, 10*time.Second)
		}
	}
	if err != nil {
		enc.Encode(map[string]any{"error": "TRANSPORT_UNAVAILABLE: " + err.Error()})
		return 5
	}
	code, body, err := callHost(root, rt, req.Method, req.Path, req.Body)
	if err != nil {
		enc.Encode(map[string]any{"error": "TRANSPORT_UNAVAILABLE: " + err.Error()})
		return 5
	}
	enc.Encode(map[string]any{"status": code, "body": json.RawMessage(body)})
	return 0
}
func loadOrCreate(path, fallback string) (string, error) {
	if raw, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		return string(bytes.TrimSpace(raw)), nil
	}
	err := gpa.AtomicWriteFile(path, []byte(fallback+"\n"))
	return fallback, err
}
func writeRuntime(root string, data any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return gpa.AtomicWriteFile(filepath.Join(root, "runtime.json"), append(raw, '\n'))
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
