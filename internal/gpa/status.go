package gpa

import (
	"os"
)

func StatusPayload(store *Store) map[string]any {
	clients := store.LoadClients()
	var lives []map[string]any
	seen := map[string]bool{}
	var liveIDs []Identity
	for _, c := range clients {
		path := c.AuthPath()
		key := c.ID + "|" + path
		if seen[key] {
			continue
		}
		seen[key] = true
		auth := loadClientAuth(c)
		if auth == nil {
			lives = append(lives, map[string]any{
				"id":    c.ID,
				"label": c.Label,
				"path":  path,
				"email": "",
				"plan":  "",
			})
			continue
		}
		id := InspectAuth(auth)
		liveIDs = append(liveIDs, id)
		lives = append(lives, map[string]any{
			"id":           c.ID,
			"label":        c.Label,
			"path":         path,
			"email":        id.Email,
			"plan":         id.Plan,
			"user_id":      id.UserID,
			"workspace_id": id.WorkspaceID,
		})
	}
	current := store.Current()
	var accounts []map[string]any
	for _, name := range store.Names() {
		acct, err := store.Get(name)
		if err != nil {
			continue
		}
		id := acct.Identity()
		var marks []string
		if name == current {
			marks = append(marks, "current")
		}
		for _, live := range liveIDs {
			if SameSeat(id, live) {
				marks = append(marks, "live")
				break
			}
		}
		where := whereUsed(clients, id)
		accounts = append(accounts, map[string]any{
			"name":         name,
			"email":        id.Email,
			"plan":         id.Plan,
			"plan_label":   id.PlanLabel(),
			"user_id":      id.UserID,
			"workspace_id": id.WorkspaceID,
			"cred_version": id.CredVersion,
			"marks":        marks,
			"used_on":      where,
		})
	}
	return map[string]any{
		"store":     store.Root,
		"current":   current,
		"lives":     lives,
		"accounts":  accounts,
		"clients":   clients,
		"legacy":    DiscoverLegacy(),
	}
}

func whereUsed(clients []Client, id Identity) []string {
	var out []string
	for _, c := range clients {
		auth := loadClientAuth(c)
		if auth == nil {
			continue
		}
		if SameSeat(InspectAuth(auth), id) {
			out = append(out, c.Label)
		}
	}
	return uniqueStrings(out)
}

func DoctorPayload(store *Store) map[string]any {
	status := StatusPayload(store)
	clients := store.LoadClients()
	var rows []map[string]any
	var suggestions []string
	byStore := map[string][]Client{}
	for _, c := range clients {
		byStore[c.Storage] = append(byStore[c.Storage], c)
		st := inspectClient(c)
		auth := loadClientAuth(c)
		email := ""
		if auth != nil {
			email = InspectAuth(auth).Email
		}
		shared := []string{}
		for _, o := range byStore[c.Storage] {
			if o.ID != c.ID {
				shared = append(shared, o.Label)
			}
		}
		// recompute shared fully
		shared = nil
		for _, o := range clients {
			if o.Storage == c.Storage && o.ID != c.ID {
				shared = append(shared, o.Label)
			}
		}
		if st.Process == ProcUnknown {
			suggestions = append(suggestions, c.Label+" 运行状态未知：不要在任务进行中强制切换")
		}
		if !clientExists(c) {
			suggestions = append(suggestions, c.Label+" 还没有 auth.json："+c.AuthPath())
		}
		rows = append(rows, map[string]any{
			"id":         c.ID,
			"label":      c.Label,
			"kind":       c.Kind,
			"path":       c.AuthPath(),
			"storage":    c.Storage,
			"process":    st.Process,
			"detail":     st.Detail,
			"email":      email,
			"shared":     shared,
		})
	}
	if len(store.Names()) == 0 {
		if legacy := DiscoverLegacy(); len(legacy) > 0 {
			suggestions = append(suggestions, "发现旧账号库，运行 gpa migrate")
		} else {
			suggestions = append(suggestions, "还没有账号，运行 gpa login NAME")
		}
	}
	status["client_states"] = rows
	status["suggestions"] = uniqueStrings(suggestions)
	status["caller"] = parentProcessName()
	status["pid"] = os.Getpid()
	return status
}
