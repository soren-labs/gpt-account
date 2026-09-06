package gpa

import (
	"os"
	"path/filepath"
	"strings"
)

type Client struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // app | cli
	Label   string `json:"label"`
	Storage string `json:"storage"`
	Distro  string `json:"distro,omitempty"`
	WinPath string `json:"windows_path"`
	WSLPath string `json:"wsl_path"`
}


type ClientFile struct {
	Version int      `json:"version"`
	Clients []Client `json:"clients"`
}

func (s *Store) LoadClients() []Client {
	saved := readSavedClients(s.ClientsPath())
	return mergeClients(saved, DiscoverClients(s.Cfg))
}

func readSavedClients(path string) []Client {
	data, err := readJSON(path)
	if err != nil || data == nil {
		return nil
	}
	raw, _ := data["clients"].([]any)
	var out []Client
	for _, item := range raw {
		m := asMap(item)
		if m == nil || asString(m["id"]) == "" {
			continue
		}
		out = append(out, Client{
			ID:      asString(m["id"]),
			Kind:    asString(m["kind"]),
			Label:   asString(m["label"]),
			Storage: asString(m["storage"]),
			Distro:  asString(m["distro"]),
			WinPath: asString(m["windows_path"]),
			WSLPath: asString(m["wsl_path"]),
		})
	}
	return out
}

func mergeClients(saved, discovered []Client) []Client {
	byID := map[string]Client{}
	var order []string
	for _, c := range saved {
		if _, ok := byID[c.ID]; !ok {
			order = append(order, c.ID)
		}
		byID[c.ID] = c
	}
	for _, c := range discovered {
		if _, ok := byID[c.ID]; ok {
			continue
		}
		byID[c.ID] = c
		order = append(order, c.ID)
	}
	var out []Client
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

func (s *Store) SaveClients(clients []Client) error {
	return writeJSON(s.ClientsPath(), ClientFile{Version: 1, Clients: clients})
}

func DiscoverClients(cfg Config) []Client {
	var out []Client
	winHome := cfg.WindowsCodex
	if winHome == "" {
		winHome = defaultWindowsCodex()
	}
	if winHome != "" {
		authWSL := toWSLPath(filepath.Join(winHome, "auth.json"))
		authWin := toWindowsPath(filepath.Join(winHome, "auth.json"))
		out = append(out,
			Client{
				ID:      "app",
				Kind:    "app",
				Label:   "ChatGPT App",
				Storage: "windows-codex",
				WinPath: authWin,
				WSLPath: authWSL,
			},
			Client{
				ID:      "win-cli",
				Kind:    "cli",
				Label:   "Windows CLI",
				Storage: "windows-codex",
				WinPath: authWin,
				WSLPath: authWSL,
			},
		)
	}
	out = append(out, discoverPOSIXClients(cfg, winHome)...)
	seen := map[string]bool{}
	var uniq []Client
	for _, c := range out {
		if c.ID == "" || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		uniq = append(uniq, c)
	}
	return uniq
}

func isolatedDiscover() bool {
	return os.Getenv("GPA_STORE") != "" || os.Getenv("GPA_CODEX_HOME") != "" || os.Getenv("GPA_WINDOWS_CODEX") != ""
}

func discoverPOSIXClients(cfg Config, winHome string) []Client {
	if isolatedDiscover() {
		if onWindows() {
			return nil
		}
		return []Client{posixClient(cfg, winHome, currentWSLDistro())}
	}
	var out []Client
	seen := map[string]bool{}
	distros := listWSLDistros()
	if d := currentWSLDistro(); d != "" {
		distros = append([]string{d}, distros...)
	}
	for _, distro := range uniqueStrings(distros) {
		if distro == "" || seen[distro] {
			continue
		}
		seen[distro] = true
		home := wslHome(distro)
		if home == "" {
			continue
		}
		out = append(out, wslClient(distro, home, winHome))
	}
	if !onWindows() && !inWSL() {
		out = append(out, posixClient(cfg, winHome, "linux"))
	}
	return out
}

func posixClient(cfg Config, winHome, distro string) Client {
	posixHome := cfg.CodexHome
	if posixHome == "" {
		home, _ := os.UserHomeDir()
		posixHome = filepath.Join(home, ".codex")
	}
	if distro == "" {
		if inWSL() {
			distro = currentWSLDistro()
		} else {
			distro = "linux"
		}
	}
	auth := filepath.Join(posixHome, "auth.json")
	id := "wsl:" + distro
	if !inWSL() && distro == "linux" {
		id = "linux"
	}
	c := Client{
		ID:      id,
		Kind:    "cli",
		Label:   distro + " CLI",
		Storage: "posix-" + distro,
		Distro:  distro,
		WSLPath: auth,
	}
	if inWSL() && distro != "" && distro != "linux" {
		c.WinPath = wslUNC(distro, auth)
	}
	if winHome != "" && localPathBelongsToDistro(distro) && sameFile(auth, filepath.Join(toWSLPath(winHome), "auth.json")) {
		c.Storage = "windows-codex"
		c.WinPath = toWindowsPath(filepath.Join(winHome, "auth.json"))
		c.WSLPath = toWSLPath(filepath.Join(winHome, "auth.json"))
	}
	return c
}

func wslClient(distro, home, winHome string) Client {
	auth := home + "/.codex/auth.json"
	if !strings.HasPrefix(home, "/") && !strings.Contains(home, `:\`) {
		auth = filepath.Join(home, ".codex", "auth.json")
	} else if strings.Contains(home, `:\`) {
		auth = winJoin(home, ".codex", "auth.json")
	}
	posix := auth
	if !strings.HasPrefix(auth, "/") {
		posix = toWSLPath(auth)
	}
	c := Client{
		ID:      "wsl:" + distro,
		Kind:    "cli",
		Label:   distro + " CLI",
		Storage: "posix-" + distro,
		Distro:  distro,
		WSLPath: posix,
		WinPath: wslUNC(distro, posix),
	}
	if winHome != "" && localPathBelongsToDistro(distro) && sameFile(posix, filepath.Join(toWSLPath(winHome), "auth.json")) {
		c.Storage = "windows-codex"
		c.WinPath = toWindowsPath(filepath.Join(winHome, "auth.json"))
		c.WSLPath = toWSLPath(filepath.Join(winHome, "auth.json"))
	}
	return c
}

func localPathBelongsToDistro(distro string) bool {
	if distro == "" || distro == "linux" {
		return true
	}
	if !inWSL() {
		return false
	}
	return distro == currentWSLDistro()
}

func sameFile(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil {
		ra = filepath.Clean(a)
	}
	if err2 != nil {
		rb = filepath.Clean(b)
	}
	return ra == rb
}

func ResolveTargets(clients []Client, spec string) ([]Client, []string, error) {
	if spec == "" || spec == "all" {
		return clients, sharedNotes(clients, clients), nil
	}
	var picked []Client
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch part {
		case "app":
			picked = append(picked, filterKind(clients, "app")...)
		case "cli":
			picked = append(picked, filterKind(clients, "cli")...)
		default:
			found := false
			for _, c := range clients {
				if c.ID == part || strings.EqualFold(c.ID, part) || strings.EqualFold(c.Label, part) {
					picked = append(picked, c)
					found = true
				}
			}
			if !found {
				return nil, nil, fail("unknown target " + part)
			}
		}
	}
	// Expand to every client that shares storage with a picked one, for disclosure.
	storages := map[string]bool{}
	for _, c := range picked {
		storages[c.Storage] = true
	}
	var affected []Client
	seen := map[string]bool{}
	for _, c := range clients {
		if storages[c.Storage] && !seen[c.ID] {
			affected = append(affected, c)
			seen[c.ID] = true
		}
	}
	return affected, sharedNotes(picked, affected), nil
}

func filterKind(clients []Client, kind string) []Client {
	var out []Client
	for _, c := range clients {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

func sharedNotes(requested, affected []Client) []string {
	req := map[string]bool{}
	for _, c := range requested {
		req[c.ID] = true
	}
	var notes []string
	for _, c := range affected {
		if !req[c.ID] {
			notes = append(notes, c.Label+" 与所选目标共用凭据文件，会一起更新")
		}
	}
	// also note when app + win-cli share even if both requested
	byStore := map[string][]Client{}
	for _, c := range affected {
		byStore[c.Storage] = append(byStore[c.Storage], c)
	}
	for _, group := range byStore {
		if len(group) > 1 {
			var labels []string
			for _, c := range group {
				labels = append(labels, c.Label)
			}
			notes = append(notes, strings.Join(labels, " · ")+" 共用同一份 auth.json")
		}
	}
	return uniqueStrings(notes)
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func uniqueStorages(clients []Client) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range clients {
		if !seen[c.Storage] {
			seen[c.Storage] = true
			out = append(out, c.Storage)
		}
	}
	return out
}

func clientsForStorage(clients []Client, storage string) []Client {
	var out []Client
	for _, c := range clients {
		if c.Storage == storage {
			out = append(out, c)
		}
	}
	return out
}
