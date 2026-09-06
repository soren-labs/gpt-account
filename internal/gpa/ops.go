package gpa

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Operation struct {
	ID        string         `json:"id"`
	CreatedAt string         `json:"created_at"`
	Kind      string         `json:"kind"`
	Account   string         `json:"account"`
	Target    string         `json:"target"`
	Reason    string         `json:"reason"`
	Force     bool           `json:"force"`
	Open      bool           `json:"open"`
	Status    string         `json:"status"`
	Result    map[string]any `json:"result,omitempty"`
}

func newOpID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:])
}

func (s *Store) SaveOperation(op Operation) error {
	if err := os.MkdirAll(s.OpsDir(), 0o700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.OpsDir(), op.ID+".json"), op)
}

func (s *Store) GetOperation(id string) (Operation, error) {
	if !ValidSlotName(id) && !opIDOK(id) {
		return Operation{}, fail("invalid operation id")
	}
	data, err := readJSON(filepath.Join(s.OpsDir(), id+".json"))
	if err != nil {
		return Operation{}, fail("no operation " + id)
	}
	if asString(data["id"]) == "" {
		return Operation{}, fail("no operation " + id)
	}
	raw, _ := json.Marshal(data)
	var op Operation
	if err := json.Unmarshal(raw, &op); err != nil {
		return Operation{}, fail("corrupt operation " + id)
	}
	return op, nil
}

func opIDOK(id string) bool {
	for _, r := range id {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == 'T' {
			continue
		}
		return false
	}
	return id != ""
}

func (s *Store) ListOperations() []Operation {
	entries, err := os.ReadDir(s.OpsDir())
	if err != nil {
		return nil
	}
	var out []Operation
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		op, err := s.GetOperation(e.Name()[:len(e.Name())-5])
		if err != nil {
			continue
		}
		out = append(out, op)
	}
	return out
}

// ApplyOperation keeps retries and completion attached to the original operation.
func ApplyOperation(store *Store, id string, force, interactive bool) Result {
	lock, err := AcquireLock(store.LockPath())
	if err != nil {
		return Result{Status: "failed", Error: err.Error()}
	}
	defer lock.Release()
	op, err := store.GetOperation(id)
	if err != nil {
		return Result{Status: "failed", Error: err.Error()}
	}
	if op.Status == "applied" {
		return Result{Status: "completed", Account: op.Account, OperationID: id}
	}
	res := useAccount(store, op.Account, orDefault(op.Target, "all"), force || op.Force, op.Open, false, interactive, id)
	res.OperationID = id
	op.Status = res.Status
	if res.Status == "completed" {
		op.Status = "applied"
	}
	op.Reason = res.Error
	if len(res.Todo) > 0 {
		op.Reason = res.Todo[0]
	}
	if err := store.SaveOperation(op); err != nil {
		res.Status = "failed"
		res.Error = "could not save operation outcome: " + err.Error()
	}
	return res
}
