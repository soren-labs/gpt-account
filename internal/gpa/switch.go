package gpa

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

func loadAuthFile(path string) map[string]any {
	if !existsQuiet(path) {
		return nil
	}
	auth, err := readJSON(path)
	if err != nil {
		return nil
	}
	if !IsChatGPTBundle(InspectAuth(auth)) {
		return nil
	}
	return auth
}

func freshness(auth map[string]any) (t time.Time, ok bool) {
	id := InspectAuth(auth)
	if id.LastRefresh == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, id.LastRefresh)
	if err != nil {
		parsed, err = time.Parse("2006-01-02T15:04:05Z", id.LastRefresh)
	}
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func liveAuths(clients []Client) []map[string]any {
	seen := map[string]bool{}
	var out []map[string]any
	for _, c := range clients {
		key := c.ID + "|" + c.AuthPath()
		if seen[key] {
			continue
		}
		seen[key] = true
		if auth := loadClientAuth(c); auth != nil {
			out = append(out, auth)
		}
	}
	return out
}

type authCand struct {
	auth map[string]any
	t    time.Time
	ok   bool
}

func AdoptLives(store *Store, clients []Client) (adopted []string, conflicts []string) {
	liveUpdates, conflicts := LatestLives(store, clients)
	for name, auth := range liveUpdates {
		if _, err := store.Put(name, auth, "live", true); err == nil {
			adopted = append(adopted, name)
		}
	}
	sort.Strings(adopted)
	sort.Strings(conflicts)
	return adopted, conflicts
}

// LatestLives only observes the clients. It does not mutate the account store,
// so a switch can commit refreshed credentials together with its other writes.
func LatestLives(store *Store, clients []Client) (updates map[string]map[string]any, conflicts []string) {
	updates = map[string]map[string]any{}
	lives := map[string][]authCand{}
	for _, auth := range liveAuths(clients) {
		id := InspectAuth(auth)
		name := store.FindByIdentity(id)
		if name == "" {
			continue
		}
		t, ok := freshness(auth)
		lives[name] = append(lives[name], authCand{auth: auth, t: t, ok: ok})
	}
	for _, name := range store.Names() {
		acct, err := store.Get(name)
		if err != nil {
			continue
		}
		stored := authCand{auth: acct.Auth}
		stored.t, stored.ok = freshness(acct.Auth)
		cands := append([]authCand{stored}, lives[name]...)
		if conflictingAuths(cands) {
			conflicts = append(conflicts, name)
			continue
		}
		best := newestAuth(cands)
		if best == nil || !tokensDiffer(best.auth, stored.auth) {
			continue
		}
		if !best.ok || (stored.ok && !best.t.After(stored.t)) {
			conflicts = append(conflicts, name)
			continue
		}
		updates[name] = best.auth
	}
	sort.Strings(conflicts)
	return updates, conflicts
}

func conflictingAuths(cands []authCand) bool {
	for i := 0; i < len(cands); i++ {
		for j := i + 1; j < len(cands); j++ {
			a, b := cands[i], cands[j]
			if !tokensDiffer(a.auth, b.auth) {
				continue
			}
			if !a.ok || !b.ok {
				return true
			}
			if a.t.Equal(b.t) {
				return true
			}
		}
	}
	return false
}

func newestAuth(cands []authCand) *authCand {
	var best *authCand
	for i := range cands {
		c := &cands[i]
		if !c.ok {
			continue
		}
		if best == nil || c.t.After(best.t) {
			best = c
		}
	}
	return best
}

func tokensDiffer(a, b map[string]any) bool {
	ta, tb := asMap(a["tokens"]), asMap(b["tokens"])
	return asString(ta["refresh_token"]) != asString(tb["refresh_token"])
}

func writeAuth(path string, auth map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeJSON(path, auth)
}

func UseAccount(store *Store, name, target string, force, open, dryRun, interactive bool) Result {
	return useAccount(store, name, target, force, open, dryRun, interactive, "")
}

func useAccount(store *Store, name, target string, force, open, dryRun, interactive bool, operationID string) Result {
	acct, err := store.Get(name)
	if err != nil {
		return Result{Status: "failed", Account: name, Error: err.Error()}
	}
	id := acct.Identity()
	if !IsChatGPTBundle(id) {
		return Result{Status: "failed", Account: name, Error: "slot " + name + " has no refresh token"}
	}
	clients := store.LoadClients()
	affected, shared, err := ResolveTargets(clients, target)
	if err != nil {
		return Result{Status: "failed", Account: name, Error: err.Error()}
	}

	res := Result{
		Account:     name,
		Email:       id.Email,
		Plan:        id.Plan,
		WorkspaceID: id.WorkspaceID,
		UserID:      id.UserID,
		Shared:      shared,
	}

	states := map[string]ClientState{}
	for _, c := range affected {
		states[c.ID] = inspectClient(c, true)
		if states[c.ID].ReasonCode == "QUERY_FAILED" {
			return Result{Status: "blocked", Account: name, Error: "cannot query client state: " + c.Label}
		}
	}

	selfRestart := false
	for _, c := range affected {
		if c.Kind == "app" && states[c.ID].Process != ProcNone && parentLooksLikeApp() {
			selfRestart = true
		}
	}

	busyCLI := map[string]bool{}
	busyApp := false
	for _, c := range affected {
		st := states[c.ID]
		if st.Process == ProcNone || st.Process == ProcIdle {
			continue
		}
		if c.Kind == "app" {
			busyApp = true
		}
		if c.Kind == "cli" {
			busyCLI[c.Storage] = true
		}
	}

	// --force may restart App. It never silently overwrites a running CLI.
	holdWrites := map[string]bool{}
	for _, storage := range uniqueStorages(affected) {
		if busyCLI[storage] {
			holdWrites[storage] = true
			continue
		}
		if !force && storageHasBusyApp(affected, states, storage) {
			holdWrites[storage] = true
		}
	}

	if len(holdWrites) > 0 {
		op := pendingOp(name, target, force, open, selfRestart, busyCLI)
		if operationID != "" {
			op.ID = operationID
		}
		if !dryRun {
			_ = store.SaveOperation(op)
			store.AppendLog("pending", map[string]any{"id": op.ID, "slot": name, "reason": op.Reason})
		}
		res.Status = "pending"
		res.OperationID = op.ID
		res.Todo = []string{op.Reason}
		res.Next = pendingNext(op, selfRestart, interactive)
		res.Clients = clientResults(affected, states, "pending", id)
		return res
	}

	if dryRun {
		res.Status = "completed"
		res.Done = []string{"dry-run: would write " + name + " to selected clients"}
		res.Clients = clientResults(affected, states, "completed", id)
		return res
	}

	needAppRestart := false
	for _, c := range affected {
		if c.Kind == "app" && (force && states[c.ID].Process != ProcNone || open) {
			needAppRestart = true
		}
	}
	if force && busyApp {
		stopChatGPT(true)
		if os.Getenv("GPA_CHATGPT") != "off" && chatgptRunning() {
			res.Status = "failed"
			res.Error = "could not stop ChatGPT.exe"
			return res
		}
	}

	liveUpdates, conflicts := LatestLives(store, store.LoadClients())
	if len(conflicts) > 0 {
		res.Status = "blocked"
		res.Error = "凭据新旧无法判断: " + joinComma(conflicts)
		res.Todo = []string{"gpa doctor", "gpa use " + name + " --force"}
		return res
	}
	var kept []string
	for n := range liveUpdates {
		if n != name {
			kept = append(kept, n)
		}
	}
	sort.Strings(kept)
	acct, _ = store.Get(name)
	if latest, ok := liveUpdates[name]; ok {
		acct.Auth = latest
	}
	id = acct.Identity()
	res.Email = id.Email
	res.Plan = id.Plan
	res.Adopted = kept

	writable := []Client{}
	skipped := []string{}
	for _, c := range affected {
		if holdWrites[c.Storage] {
			skipped = append(skipped, c.Label)
		} else {
			writable = append(writable, c)
		}
	}
	tx, err := prepareTransaction(store, writable, acct.Auth)
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		res.RecoveryStatus = "not_needed"
		return res
	}
	rollback := func() {
		if err := tx.rollback(store); err != nil {
			res.RecoveryStatus = "failed"
			res.Error += "; recovery failed: " + err.Error()
		} else {
			res.RecoveryStatus = "restored"
		}
	}
	written := []string{}
	for _, b := range tx.Files {
		if err := clientWriteBytes(b.Client, tx.Next); err != nil {
			res.Status = "failed"
			res.Error = "write " + b.Client.Label + ": " + err.Error()
			rollback()
			return res
		}
		written = append(written, b.Client.AuthPath())
	}
	if err := store.SetCurrent(name); err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		rollback()
		return res
	}

	if needAppRestart || (open && !chatgptRunning()) {
		if !startChatGPT() {
			res.Status = "failed"
			res.Error = "could not start ChatGPT.exe"
			rollback()
			return res
		}
	}

	for _, storage := range uniqueStorages(affected) {
		if holdWrites[storage] {
			continue
		}
		c := clientsForStorage(affected, storage)[0]
		got := loadClientAuth(c)
		if got == nil || !SameSeat(InspectAuth(got), id) {
			res.Status = "failed"
			res.Error = "live auth mismatch after switch: " + c.AuthPath()
			rollback()
			return res
		}
	}

	// Save refreshed live credentials only after all client files have passed
	// verification. Keep a memory snapshot so a metadata write failure can
	// restore the account library along with the client transaction.
	type accountSnapshot struct {
		name             string
		auth, meta       []byte
		authHad, metaHad bool
	}
	var accountBackups []accountSnapshot
	for n := range liveUpdates {
		dest, err := store.SlotDir(n)
		if err != nil {
			res.Status = "failed"
			res.Error = err.Error()
			rollback()
			return res
		}
		var snap accountSnapshot
		snap.name = n
		snap.auth, err = os.ReadFile(filepath.Join(dest, "auth.json"))
		if err == nil {
			snap.authHad = true
		} else if !os.IsNotExist(err) {
			res.Status = "failed"
			res.Error = err.Error()
			rollback()
			return res
		}
		snap.meta, err = os.ReadFile(filepath.Join(dest, "meta.json"))
		if err == nil {
			snap.metaHad = true
		} else if !os.IsNotExist(err) {
			res.Status = "failed"
			res.Error = err.Error()
			rollback()
			return res
		}
		accountBackups = append(accountBackups, snap)
	}
	restoreAccounts := func() {
		for _, snap := range accountBackups {
			dest, err := store.SlotDir(snap.name)
			if err != nil {
				continue
			}
			if snap.authHad {
				_ = atomicWrite(filepath.Join(dest, "auth.json"), snap.auth, 0o600)
			} else {
				_ = os.Remove(filepath.Join(dest, "auth.json"))
			}
			if snap.metaHad {
				_ = atomicWrite(filepath.Join(dest, "meta.json"), snap.meta, 0o600)
			} else {
				_ = os.Remove(filepath.Join(dest, "meta.json"))
			}
		}
	}
	for n, latest := range liveUpdates {
		if _, err := store.Put(n, latest, "live", true); err != nil {
			res.Status = "failed"
			res.Error = "saving refreshed credentials for " + n + ": " + err.Error()
			rollback()
			restoreAccounts()
			return res
		}
	}

	if err := os.Remove(journalPath(store)); err != nil {
		res.Status = "failed"
		res.Error = "could not finalize switch journal: " + err.Error()
		rollback()
		restoreAccounts()
		return res
	}
	res.RecoveryStatus = "not_needed"
	store.AppendLog("use", map[string]any{
		"slot": name, "email": id.Email, "plan": id.Plan,
		"written": written, "adopted": kept, "target": target, "skipped": skipped,
	})
	res.Written = written
	res.Done = []string{"已写入 " + name}
	if len(skipped) > 0 {
		op := pendingOp(name, target, force, open, selfRestart, busyCLI)
		if operationID != "" {
			op.ID = operationID
		}
		op.Reason = "未托管的 Codex 仍在运行，未覆盖: " + joinComma(uniqueStrings(skipped))
		_ = store.SaveOperation(op)
		res.Status = "pending"
		res.OperationID = op.ID
		res.Todo = []string{op.Reason}
		res.Next = "关闭对应 Codex 后: gpa operation apply " + op.ID
		res.Clients = clientResultsByStorage(affected, states, holdWrites, id)
		return res
	}
	res.Status = "completed"
	res.Clients = clientResults(affected, states, "completed", id)
	return res
}

func storageHasBusyApp(affected []Client, states map[string]ClientState, storage string) bool {
	for _, c := range clientsForStorage(affected, storage) {
		if c.Kind != "app" {
			continue
		}
		st := states[c.ID]
		if st.Process != ProcNone && st.Process != ProcIdle {
			return true
		}
	}
	return false
}

func pendingOp(name, target string, force, open, selfRestart bool, busyCLI map[string]bool) Operation {
	op := Operation{
		ID:        newOpID(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Kind:      "use",
		Account:   name,
		Target:    target,
		Force:     force,
		Open:      open,
		Status:    "pending",
	}
	if selfRestart {
		op.Reason = "切换会结束当前 ChatGPT 会话；完成后在独立 GPA 窗口继续，或按 Y 确认重启"
	} else if len(busyCLI) > 0 {
		op.Reason = "有未托管的 Codex 正在运行，默认不覆盖它正在用的凭据"
	} else {
		op.Reason = "目标客户端正在运行或状态未知，默认不覆盖凭据"
	}
	return op
}

func pendingNext(op Operation, selfRestart, interactive bool) string {
	if interactive && !selfRestart {
		return "在菜单按 Y 确认重启，或 gpa operation apply " + op.ID + " --force"
	}
	if selfRestart {
		return "gpa ui --new-window   # 或 gpa operation apply " + op.ID + " --force"
	}
	return "gpa operation apply " + op.ID + " --force"
}

func clientResultsByStorage(clients []Client, states map[string]ClientState, held map[string]bool, id Identity) []ClientResult {
	out := clientResults(clients, states, "completed", id)
	for i := range out {
		if held[out[i].Storage] {
			out[i].Status = "pending"
		}
	}
	return out
}

func clientResults(clients []Client, states map[string]ClientState, status string, id Identity) []ClientResult {
	byStore := map[string][]string{}
	for _, c := range clients {
		byStore[c.Storage] = append(byStore[c.Storage], c.Label)
	}
	var out []ClientResult
	for _, c := range clients {
		st := states[c.ID]
		shared := []string{}
		for _, label := range byStore[c.Storage] {
			if label != c.Label {
				shared = append(shared, label)
			}
		}
		out = append(out, ClientResult{
			ID:         c.ID,
			Label:      c.Label,
			Kind:       c.Kind,
			Status:     status,
			Path:       c.AuthPath(),
			Storage:    c.Storage,
			Process:    st.Process,
			SharedWith: shared,
			Email:      id.Email,
			Detail:     st.Detail,
		})
	}
	return out
}

func joinComma(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}

func SaveLive(store *Store, name string, force bool) (map[string]any, error) {
	clients := store.LoadClients()
	var best map[string]any
	var bestT time.Time
	var bestOK bool
	for _, auth := range liveAuths(clients) {
		t, ok := freshness(auth)
		if best == nil || (ok && (!bestOK || t.After(bestT))) {
			best = auth
			bestT = t
			bestOK = ok
		}
	}
	if best == nil {
		return nil, fail("no ChatGPT login in live clients; stay logged in and retry")
	}
	id := InspectAuth(best)
	taken := map[string]bool{}
	for _, n := range store.Names() {
		taken[n] = true
	}
	slot := name
	if slot == "" {
		slot = DefaultSlotName(id, taken)
	}
	dest, err := store.SlotDir(slot)
	if err != nil {
		return nil, err
	}
	if exists(filepath.Join(dest, "auth.json")) && !force {
		old, _ := readJSON(filepath.Join(dest, "auth.json"))
		oldID := InspectAuth(old)
		if !SameSeat(oldID, id) {
			who := oldID.Email
			if who == "" {
				who = oldID.UserID
			}
			return nil, fail("slot " + slot + " already holds " + who + "; use another name or --force")
		}
	}
	meta, err := store.Put(slot, best, "live", true)
	if err != nil {
		return nil, err
	}
	_ = store.SetCurrent(slot)
	store.AppendLog("save", map[string]any{"slot": slot, "email": id.Email, "plan": id.Plan})
	return meta, nil
}

// CredentialConflicts compares saved and live bundles without mutating either.
func CredentialConflicts(store *Store, clients []Client) []string {
	var conflicts []string
	lives := liveAuths(clients)
	for _, name := range store.Names() {
		acct, err := store.Get(name)
		if err != nil {
			continue
		}
		t, ok := freshness(acct.Auth)
		cands := []authCand{{auth: acct.Auth, t: t, ok: ok}}
		for _, auth := range lives {
			if SameSeat(InspectAuth(auth), acct.Identity()) {
				t, ok := freshness(auth)
				cands = append(cands, authCand{auth: auth, t: t, ok: ok})
			}
		}
		if conflictingAuths(cands) {
			conflicts = append(conflicts, name)
		}
	}
	return conflicts
}
