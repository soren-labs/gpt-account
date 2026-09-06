package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

func (s *Service) Preview(accountRef, targetID string) (Plan, error) {
	if strings.TrimSpace(targetID) == "" {
		return Plan{}, errf("target is required")
	}
	acct, err := s.Store.ResolveAccount(accountRef)
	if err != nil {
		return Plan{}, err
	}
	clients := s.Store.LoadClients()
	targets := s.targets(clients)
	tv, err := s.targetByID(targets, targetID)
	if err != nil {
		return Plan{}, err
	}
	cliSpec := targetToCLI(targetID)
	affected, _, err := gpa.ResolveTargets(clients, cliSpec)
	if err != nil {
		return Plan{}, err
	}
	members := tv.Members
	presence := map[string]string{}
	reason := ""
	detail := ""
	decision := "ready"
	already := true
	id := acct.Identity()
	for _, c := range affected {
		st := s.inspect(c)
		p := string(presenceOf(st))
		presence[c.ID] = p
		if st.ReasonCode == "QUERY_FAILED" || p == string(gpa.PresenceUnknown) {
			decision = "blocked"
			reason = "QUERY_FAILED"
			detail = "无法确认运行状态，不能写入凭据"
		} else if st.ReasonCode == "CLI_RUNNING" && decision != "blocked" {
			decision = "waiting_user"
			reason = "CLI_IN_USE"
			detail = c.Label + " 正在使用，关闭后再重试"
		} else if st.ReasonCode == "APP_RUNNING" && decision == "ready" {
			decision = "waiting_user"
			reason = "APP_RESTART_REQUIRED"
			detail = "桌面应用正在运行，需要确认重启"
		}
		live := gpa.LoadClientAuth(c)
		if live == nil || !gpa.SameSeat(gpa.InspectAuth(live), id) {
			already = false
		}
	}
	if already && decision == "ready" {
		if conflict, msg := s.credentialConflict(acct, affected); conflict {
			decision = "blocked"
			reason = "CREDENTIAL_CONFLICT"
			detail = msg
			already = false
		} else {
			decision = "noop"
			detail = "所选范围已经是这个账号"
		}
	} else if decision == "ready" {
		if conflict, msg := s.credentialConflict(acct, affected); conflict {
			decision = "blocked"
			reason = "CREDENTIAL_CONFLICT"
			detail = msg
		}
	}
	plan := Plan{
		ID:             newID("plan_"),
		Account:        acct.Name,
		AccountID:      s.Store.AccountID(acct),
		Target:         targetID,
		Members:        members,
		Revision:       credVer(acct),
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339),
		Decision:       decision,
		ReasonCode:     reason,
		Message:        detail,
		SharedNote:     tv.SharedNote,
		Version:        1,
		Presence:       presence,
		AlreadyCurrent: already && decision == "noop",
	}
	if err := s.savePlan(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (s *Service) credentialConflict(acct gpa.Account, affected []gpa.Client) (bool, string) {
	stored := acct.Auth
	for _, c := range affected {
		live := gpa.LoadClientAuth(c)
		if live == nil {
			continue
		}
		if !gpa.SameSeat(gpa.InspectAuth(live), acct.Identity()) {
			continue
		}
		if refreshOf(live) != refreshOf(stored) && freshnessEqual(live, stored) {
			return true, c.Label + " 与账号库凭据版本冲突"
		}
	}
	return false, ""
}

func refreshOf(auth map[string]any) string {
	tok := map[string]any{}
	if m, ok := auth["tokens"].(map[string]any); ok {
		tok = m
	}
	s, _ := tok["refresh_token"].(string)
	return s
}

func freshnessEqual(a, b map[string]any) bool {
	as, _ := a["last_refresh"].(string)
	bs, _ := b["last_refresh"].(string)
	return as != "" && as == bs
}

func targetToCLI(id string) string {
	switch id {
	case "desktop":
		return "app"
	case "all":
		return "all"
	default:
		return id
	}
}

func (s *Service) Submit(planID, requestID, idem string, actor string) (Envelope, error) {
	return s.submit(planID, requestID, idem, actor, false)
}

func (s *Service) submit(planID, requestID, idem, actor string, confirmRestart bool) (Envelope, error) {
	if requestID == "" {
		requestID = newID("req_")
	}
	if idem == "" {
		idem = requestID
	}
	if prev, ok := s.lookupIdem(idem); ok {
		if prev.PlanID != planID {
			return Envelope{}, errf("idempotency key reused with a different request")
		}
		return s.envelope(prev, requestID), nil
	}
	plan, err := s.loadPlan(planID)
	if err != nil {
		return Envelope{}, err
	}
	if expired(plan.ExpiresAt) {
		return Envelope{Status: "blocked", ReasonCode: "PLAN_STALE", Message: "预览已过期，请重新检查", RequestID: requestID}, nil
	}
	fresh, err := s.Preview(plan.Account, plan.Target)
	if err != nil {
		return Envelope{}, err
	}
	if scopeGrew(plan, fresh) || fresh.Revision != plan.Revision {
		return Envelope{Status: "blocked", ReasonCode: "PLAN_STALE", Message: "账号或范围已变化，请查看新的预览", RequestID: requestID}, nil
	}
	op := OpRecord{
		ID:             newID("op_"),
		CreatedAt:      nowISO(),
		UpdatedAt:      nowISO(),
		Kind:           "switch",
		Account:        plan.Account,
		AccountID:      plan.AccountID,
		Target:         plan.Target,
		Targets:        []string{plan.Target},
		RequestID:      requestID,
		PlanID:         plan.ID,
		IdempotencyKey: idem,
		Actor:          actor,
		Attempt:        1,
		Phase:          "checking",
		Recovery:       map[string]any{"status": "not_needed"},
	}
	if fresh.Decision == "noop" {
		op.Status = "succeeded"
		op.Phase = "verifying"
		op.Message = fresh.Message
		_ = s.saveOp(op)
		s.saveIdem(idem, op)
		return s.envelope(op, requestID), nil
	}
	if fresh.Decision == "blocked" {
		op.Status = "blocked"
		op.ReasonCode = fresh.ReasonCode
		op.Message = fresh.Message
		_ = s.saveOp(op)
		s.saveIdem(idem, op)
		return s.envelope(op, requestID), nil
	}
	if fresh.Decision == "waiting_user" && !confirmRestart {
		op.Status = "waiting_user"
		op.ReasonCode = fresh.ReasonCode
		op.Message = fresh.Message
		_ = s.saveOp(op)
		s.saveIdem(idem, op)
		return s.envelope(op, requestID), nil
	}
	if fresh.ReasonCode == "APP_RESTART_REQUIRED" && actor != "ui" && !confirmRestart {
		op.Status = "waiting_user"
		op.ReasonCode = "APP_RESTART_REQUIRED"
		op.Message = "桌面应用正在运行，需要在管理页面确认重启。"
		_ = s.saveOp(op)
		s.saveIdem(idem, op)
		return s.envelope(op, requestID), nil
	}
	if fresh.ReasonCode == "CLI_IN_USE" {
		op.Status = "waiting_user"
		op.ReasonCode = "CLI_IN_USE"
		op.Message = fresh.Message
		_ = s.saveOp(op)
		s.saveIdem(idem, op)
		return s.envelope(op, requestID), nil
	}
	op.Status = "running"
	op.Phase = "writing"
	_ = s.saveOp(op)
	res := s.switchAccount(plan.Account, targetToCLI(plan.Target), confirmRestart || fresh.ReasonCode == "APP_RESTART_REQUIRED" && actor == "ui")
	op.UpdatedAt = nowISO()
	switch res.Status {
	case "completed":
		op.Status = "succeeded"
		op.Phase = "verifying"
		op.Message = "已切换本地凭据"
		if confirmRestart {
			op.Message = "已切换本地凭据 · 应用已重新打开"
		}
		op.Recovery = map[string]any{"status": "not_needed"}
	case "pending":
		op.Status = "waiting_user"
		op.Message = first(res.Todo, res.Error)
		if strings.Contains(op.Message, "CLI") || strings.Contains(op.Message, "Codex") {
			op.ReasonCode = "CLI_IN_USE"
		} else {
			op.ReasonCode = "APP_RESTART_REQUIRED"
		}
	case "blocked":
		op.Status = "blocked"
		op.ReasonCode = "BLOCKED"
		op.Message = res.Error
	default:
		op.Status = "failed"
		op.Phase = "restoring"
		op.Message = res.Error
		if res.Error == "" {
			op.Message = "切换失败"
		}
		op.Recovery = map[string]any{"status": "restored"}
	}
	op.Result = map[string]any{"legacy_status": res.Status, "written": res.Written}
	_ = s.saveOp(op)
	s.saveIdem(idem, op)
	return s.envelope(op, requestID), nil
}

func (s *Service) Confirm(id, requestID, actor string) (Envelope, error) {
	if actor != "ui" {
		return Envelope{}, errf("restart confirmation requires the web session")
	}
	op, err := s.GetOp(id)
	if err != nil {
		return Envelope{}, err
	}
	if op.Status == "succeeded" || op.Status == "cancelled" {
		return s.envelope(op, requestID), nil
	}
	if op.PlanID == "" {
		return Envelope{}, errf("operation has no plan")
	}
	env, err := s.submit(op.PlanID, or(requestID, op.RequestID), op.IdempotencyKey+"-confirm", actor, true)
	if err != nil {
		return Envelope{}, err
	}
	if env.OperationID != "" && env.OperationID != op.ID {
		fresh, e2 := s.GetOp(env.OperationID)
		if e2 == nil {
			_ = os.Remove(s.opPath(env.OperationID))
			fresh.ID = op.ID
			fresh.Attempt = op.Attempt + 1
			_ = s.saveOp(fresh)
			env.OperationID = op.ID
		}
	}
	return env, nil
}

func (s *Service) Retry(id, requestID, idem, actor string) (Envelope, error) {
	op, err := s.GetOp(id)
	if err != nil {
		return Envelope{}, err
	}
	if rec, _ := op.Recovery["status"].(string); rec == "failed" {
		return Envelope{Status: "blocked", ReasonCode: "RECOVERY_FAILED", Message: "先处理恢复失败", OperationID: op.ID, RequestID: requestID}, nil
	}
	if op.PlanID == "" {
		return Envelope{}, errf("operation has no plan")
	}
	if idem == "" {
		idem = or(requestID, newID("req_"))
	}
	env, err := s.submit(op.PlanID, or(requestID, op.RequestID), idem, actor, false)
	if err != nil {
		return Envelope{}, err
	}
	if env.OperationID != "" && env.OperationID != op.ID {
		// keep original id for retries of waiting/blocked ops
		if op.Status == "waiting_user" || op.Status == "blocked" || op.Status == "failed" {
			fresh, _ := s.GetOp(env.OperationID)
			fresh.ID = op.ID
			fresh.Attempt = op.Attempt + 1
			_ = os.Remove(s.opPath(env.OperationID))
			_ = s.saveOp(fresh)
			env.OperationID = op.ID
		}
	}
	return env, nil
}

func (s *Service) SaveCancelled(op OpRecord) error { return s.saveOp(op) }

func (s *Service) GetOp(id string) (OpRecord, error) {
	raw, err := os.ReadFile(s.opPath(id))
	if err != nil {
		op, e2 := s.Store.GetOperation(id)
		if e2 != nil {
			return OpRecord{}, errf("no operation " + id)
		}
		return legacyOp(op), nil
	}
	var op OpRecord
	if err := json.Unmarshal(raw, &op); err != nil {
		return OpRecord{}, err
	}
	if op.Status == "applied" {
		op.Status = "succeeded"
	}
	return op, nil
}

func (s *Service) ListOps(limit int) []OpRecord {
	entries, err := os.ReadDir(s.opsDir())
	if err != nil {
		return nil
	}
	var out []OpRecord
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		op, err := s.GetOp(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, op)
	}
	sortOps(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortOps(ops []OpRecord) {
	for i := 0; i < len(ops); i++ {
		for j := i + 1; j < len(ops); j++ {
			if ops[j].CreatedAt > ops[i].CreatedAt {
				ops[i], ops[j] = ops[j], ops[i]
			}
		}
	}
}

func legacyOp(op gpa.Operation) OpRecord {
	st := op.Status
	if st == "applied" || st == "completed" {
		st = "succeeded"
	}
	if st == "pending" {
		st = "waiting_user"
	}
	return OpRecord{ID: op.ID, CreatedAt: op.CreatedAt, Kind: op.Kind, Account: op.Account, Target: op.Target, Status: st, Message: op.Reason}
}

func (s *Service) envelope(op OpRecord, requestID string) Envelope {
	env := Envelope{
		SchemaVersion: 1,
		RequestID:     or(requestID, op.RequestID),
		Status:        op.Status,
		OperationID:   op.ID,
		AccountID:     op.AccountID,
		Targets:       op.Targets,
		ReasonCode:    op.ReasonCode,
		Message:       op.Message,
		Verification:  map[string]any{"local_credentials": "not_checked", "app_process": "not_checked", "online": "not_checked"},
		Demo:          s.Demo,
	}
	if op.Status == "succeeded" {
		env.Verification["local_credentials"] = "matched"
	}
	if op.Status == "waiting_user" && op.ReasonCode == "APP_RESTART_REQUIRED" {
		env.NextAction = &NextAction{Type: "open_ui", OperationID: op.ID}
	}
	return env
}

func (s *Service) plansDir() string { return filepath.Join(s.Store.Root, "plans") }
func (s *Service) opsDir() string   { return filepath.Join(s.Store.Root, "operations") }
func (s *Service) opPath(id string) string {
	return filepath.Join(s.opsDir(), id+".json")
}
func (s *Service) idemPath() string { return filepath.Join(s.Store.Root, "idempotency.json") }

func (s *Service) savePlan(p Plan) error {
	_ = os.MkdirAll(s.plansDir(), 0o700)
	return writeJSON(filepath.Join(s.plansDir(), p.ID+".json"), p)
}

func (s *Service) loadPlan(id string) (Plan, error) {
	raw, err := os.ReadFile(filepath.Join(s.plansDir(), id+".json"))
	if err != nil {
		return Plan{}, errf("unknown plan")
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return Plan{}, err
	}
	return p, nil
}

func (s *Service) saveOp(op OpRecord) error {
	_ = os.MkdirAll(s.opsDir(), 0o700)
	return writeJSON(s.opPath(op.ID), op)
}

func (s *Service) lookupIdem(key string) (OpRecord, bool) {
	data := map[string]string{}
	raw, err := os.ReadFile(s.idemPath())
	if err == nil {
		_ = json.Unmarshal(raw, &data)
	}
	id := data[key]
	if id == "" {
		return OpRecord{}, false
	}
	op, err := s.GetOp(id)
	if err != nil {
		return OpRecord{}, false
	}
	return op, true
}

func (s *Service) saveIdem(key string, op OpRecord) {
	data := map[string]string{}
	raw, err := os.ReadFile(s.idemPath())
	if err == nil {
		_ = json.Unmarshal(raw, &data)
	}
	data[key] = op.ID
	_ = writeJSON(s.idemPath(), data)
}

func scopeGrew(old, fresh Plan) bool {
	if fresh.Target != old.Target {
		return true
	}
	have := map[string]bool{}
	for _, m := range old.Members {
		have[m] = true
	}
	for _, m := range fresh.Members {
		if !have[m] {
			return true
		}
	}
	return false
}

func expired(iso string) bool {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return true
	}
	return time.Now().After(t)
}

func first(ss []string, fallback string) string {
	if len(ss) > 0 && ss[0] != "" {
		return ss[0]
	}
	return fallback
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
