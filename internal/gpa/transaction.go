package gpa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type authSnapshot struct {
	Client Client `json:"client"`
	Old    []byte `json:"old"`
	Had    bool   `json:"had"`
}
type authTransaction struct {
	Files []authSnapshot `json:"files"`
	Next  []byte         `json:"next"`
	State map[string]any `json:"state"`
}

func journalPath(s *Store) string { return filepath.Join(s.Root, "switch-journal.json") }
func prepareTransaction(s *Store, clients []Client, auth map[string]any) (*authTransaction, error) {
	if exists(journalPath(s)) {
		return nil, fmt.Errorf("unfinished recovery requires attention")
	}
	raw, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return nil, err
	}
	tx := &authTransaction{Next: append(raw, '\n'), State: s.State()}
	seen := map[string]bool{}
	for _, c := range clients {
		if seen[c.Storage] {
			continue
		}
		seen[c.Storage] = true
		old, err := clientReadBytes(c)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot snapshot %s: %w", c.Label, err)
		}
		tx.Files = append(tx.Files, authSnapshot{Client: c, Old: old, Had: err == nil})
	}
	if err := writeJSON(journalPath(s), tx); err != nil {
		return nil, err
	}
	return tx, nil
}
func (tx *authTransaction) rollback(s *Store) error {
	// Preflight every file before restoring any: never downgrade credentials that
	// a client refreshed after our write, including after a process restart.
	for _, b := range tx.Files {
		current, err := clientReadBytes(b.Client)
		if err != nil && !(os.IsNotExist(err) && !b.Had) {
			return fmt.Errorf("cannot inspect recovery for %s: %w", b.Client.Label, err)
		}
		if err == nil && !bytes.Equal(current, b.Old) && !bytes.Equal(current, tx.Next) {
			return fmt.Errorf("%s changed after the switch; recovery retained for manual resolution", b.Client.Label)
		}
	}
	for i := len(tx.Files) - 1; i >= 0; i-- {
		b := tx.Files[i]
		current, readErr := clientReadBytes(b.Client)
		if (readErr == nil && b.Had && bytes.Equal(current, b.Old)) || (os.IsNotExist(readErr) && !b.Had) {
			continue
		}
		var err error
		if b.Had {
			err = clientWriteBytes(b.Client, b.Old)
		} else {
			err = clientRemove(b.Client)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			return err
		}
	}
	if err := s.WriteState(tx.State); err != nil {
		return err
	}
	return os.Remove(journalPath(s))
}
func RecoverTransaction(s *Store) error {
	raw, err := os.ReadFile(journalPath(s))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var tx authTransaction
	if err = json.Unmarshal(raw, &tx); err != nil {
		return err
	}
	for _, b := range tx.Files {
		st := InspectClient(b.Client)
		if st.Process != ProcNone {
			return fmt.Errorf("recovery deferred: %s is running or unknown", b.Client.Label)
		}
	}
	return tx.rollback(s)
}
