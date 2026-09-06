package gpa

import (
	"bytes"
	"os"
	"testing"
)

func TestRecoveryPreservesRefreshedCredentials(t *testing.T) {
	s := testEnv(t)
	a, _ := s.Get("biz1")
	clients := s.LoadClients()
	tx, err := prepareTransaction(s, clients, a.Auth)
	if err != nil {
		t.Fatal(err)
	}
	c := tx.Files[0].Client
	changed := append([]byte(" "), tx.Next...)
	if err := clientWriteBytes(c, changed); err != nil {
		t.Fatal(err)
	}
	if err := tx.rollback(s); err == nil {
		t.Fatal("overwrote newer client content")
	}
	got, _ := clientReadBytes(c)
	if !bytes.Equal(got, changed) {
		t.Fatal("refreshed content lost")
	}
	if _, err := os.Stat(journalPath(s)); err != nil {
		t.Fatal("recovery journal removed")
	}
}
func TestRecoveryAfterInterruptedFirstWrite(t *testing.T) {
	s := testEnv(t)
	a, _ := s.Get("biz1")
	tx, err := prepareTransaction(s, s.LoadClients(), a.Auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := clientWriteBytes(tx.Files[0].Client, tx.Next); err != nil {
		t.Fatal(err)
	}
	if err := RecoverTransaction(s); err != nil {
		t.Fatal(err)
	}
	for _, b := range tx.Files {
		got, _ := clientReadBytes(b.Client)
		if !bytes.Equal(got, b.Old) {
			t.Fatal("not restored", b.Client.ID)
		}
	}
	if _, err := os.Stat(journalPath(s)); !os.IsNotExist(err) {
		t.Fatal("journal remains")
	}
}
